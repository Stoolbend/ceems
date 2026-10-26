import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import UsersView from './UsersView.vue'
import { ApiError, createGrant, createUser, deleteGrant, listUsers, resetPassword, setDisabled, type UserDoc } from './api'
import { user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  listUsers: vi.fn(),
  createUser: vi.fn(),
  setDisabled: vi.fn(),
  resetPassword: vi.fn(),
  createGrant: vi.fn(),
  deleteGrant: vi.fn(),
}))

const conor: UserDoc = { id: 1, username: 'conor', admin: true, disabled: false, createdAt: '2026-01-01T00:00:00Z', grants: [] }
const viewer: UserDoc = {
  id: 2, username: 'viewer', admin: false, disabled: false, createdAt: '2026-01-02T00:00:00Z',
  grants: [{ id: 7, lo: '100', hi: '1FF', access: 'write' }],
}
const fakePassword = 'Fake0000000000000000' // a fixture, not a generated password

beforeEach(() => {
  vi.restoreAllMocks()
  for (const f of [listUsers, createUser, setDisabled, resetPassword, createGrant, deleteGrant]) vi.mocked(f).mockReset()
  vi.mocked(listUsers).mockResolvedValue([conor, viewer])
  user.value = { id: 1, username: 'conor', admin: true }
})

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<i/>' } }, { path: '/users', component: UsersView }],
  })
  router.push('/users')
  await router.isReady()
  const wrapper = mount(UsersView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>
const row = (w: Wrapper, name: string) => w.find(`tr[data-user=${name}]`)

test('lists users with their grants; no Disable on your own row', async () => {
  const wrapper = await mountView()
  expect(row(wrapper, 'viewer').text()).toContain('100-1FF write')
  expect(row(wrapper, 'viewer').find('button.toggle').text()).toBe('Disable')
  expect(row(wrapper, 'conor').find('button.toggle').exists()).toBe(false)
})

test('create shows the password once', async () => {
  vi.mocked(createUser).mockResolvedValue({ user: { ...viewer, id: 3, username: 'new' }, password: fakePassword })
  const wrapper = await mountView()
  await wrapper.find('form.create input[name=username]').setValue('new')
  await wrapper.find('form.create input[name=admin]').setValue(true)
  await wrapper.find('form.create').trigger('submit')
  await flushPromises()
  expect(createUser).toHaveBeenCalledWith('new', true)
  expect(wrapper.find('.password code').text()).toBe(fakePassword)
  expect(wrapper.find('.password').text()).toContain('Copy it now')
  // The next action drops it.
  vi.mocked(deleteGrant).mockResolvedValue()
  await row(wrapper, 'viewer').find('button.delete-grant').trigger('click')
  await flushPromises()
  expect(deleteGrant).toHaveBeenCalledWith(7)
  expect(wrapper.find('.password').exists()).toBe(false)
})

test('disable and reset ask first', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  const wrapper = await mountView()
  await row(wrapper, 'viewer').find('button.toggle').trigger('click')
  await row(wrapper, 'viewer').find('button.reset').trigger('click')
  expect(confirm).toHaveBeenCalledTimes(2)
  expect(setDisabled).not.toHaveBeenCalled()
  expect(resetPassword).not.toHaveBeenCalled()

  confirm.mockReturnValue(true)
  vi.mocked(setDisabled).mockResolvedValue({ ...viewer, disabled: true })
  vi.mocked(resetPassword).mockResolvedValue(fakePassword)
  await row(wrapper, 'viewer').find('button.toggle').trigger('click')
  await flushPromises()
  expect(setDisabled).toHaveBeenCalledWith(2, true)
  await row(wrapper, 'viewer').find('button.reset').trigger('click')
  await flushPromises()
  expect(resetPassword).toHaveBeenCalledWith(2)
  expect(wrapper.find('.password code').text()).toBe(fakePassword)
})

test('add grant sends the range upper-cased; a server error is shown', async () => {
  vi.mocked(createGrant).mockRejectedValue(new ApiError(400, 'bad_request', 'lo and hi must be pages 100-8FF with lo <= hi'))
  const wrapper = await mountView()
  const form = row(wrapper, 'viewer').find('form.grant')
  await form.find('input[name=lo]').setValue('2ff')
  await form.find('input[name=hi]').setValue('200')
  await form.find('select').setValue('write')
  await form.trigger('submit')
  await flushPromises()
  expect(createGrant).toHaveBeenCalledWith(2, '2FF', '200', 'write')
  expect(wrapper.find('[role=alert]').text()).toBe('lo and hi must be pages 100-8FF with lo <= hi')
})
