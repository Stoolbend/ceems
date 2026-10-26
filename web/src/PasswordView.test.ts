import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import PasswordView from './PasswordView.vue'
import { ApiError, changePassword } from './api'
import { user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  changePassword: vi.fn(),
}))

beforeEach(() => {
  vi.mocked(changePassword).mockReset()
  user.value = { id: 1, username: 'conor', admin: false }
})

async function fill(current: string, next: string, confirm: string) {
  const stub = { template: '<i/>' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: stub }, { path: '/login', component: stub }, { path: '/password', component: PasswordView }],
  })
  router.push('/password')
  await router.isReady()
  const wrapper = mount(PasswordView, { global: { plugins: [router] } })
  await wrapper.find('input[name=current]').setValue(current)
  await wrapper.find('input[name=new]').setValue(next)
  await wrapper.find('input[name=confirm]').setValue(confirm)
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  return { router, wrapper }
}

test('a successful change logs out and goes to /login?changed=1', async () => {
  vi.mocked(changePassword).mockResolvedValue()
  const { router } = await fill('old password!', 'new password!!', 'new password!!')
  expect(changePassword).toHaveBeenCalledWith('old password!', 'new password!!')
  expect(user.value).toBeNull()
  expect(router.currentRoute.value.path).toBe('/login')
  expect(router.currentRoute.value.query.changed).toBe('1')
})

test('a 400 shows the server message and stays', async () => {
  vi.mocked(changePassword).mockRejectedValue(new ApiError(400, 'wrong_password', 'current password is incorrect'))
  const { router, wrapper } = await fill('wrong', 'new password!!', 'new password!!')
  expect(wrapper.find('[role=alert]').text()).toBe('current password is incorrect')
  expect(router.currentRoute.value.path).toBe('/password')
  expect(user.value).not.toBeNull()
})

test('mismatched new passwords are refused before the server', async () => {
  const { wrapper } = await fill('old password!', 'new password!!', 'different!!!!')
  expect(changePassword).not.toHaveBeenCalled()
  expect(wrapper.find('[role=alert]').text()).toBe('The new passwords do not match.')
})
