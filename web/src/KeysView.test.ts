import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import KeysView from './KeysView.vue'
import { ApiError, createKey, createKeyGrant, deleteGrant, listKeys, revokeKey, type KeyDoc } from './api'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  listKeys: vi.fn(),
  createKey: vi.fn(),
  revokeKey: vi.fn(),
  createKeyGrant: vi.fn(),
  deleteGrant: vi.fn(),
}))

const sync: KeyDoc = {
  id: '0123456789abcdef', name: 'sync', readOnly: true, createdBy: 'user:1', createdAt: '2026-01-01T00:00:00Z',
  expiresAt: '2027-01-01T00:00:00Z', revokedAt: null, lastUsedAt: '2026-02-01T00:00:00Z',
  grants: [{ id: 7, lo: '100', hi: '1FF', access: 'write' }],
}
const old: KeyDoc = {
  id: 'fedcba9876543210', name: 'old', readOnly: false, createdBy: 'user:1', createdAt: '2026-01-01T00:00:00Z',
  expiresAt: null, revokedAt: '2026-03-01T00:00:00Z', lastUsedAt: null, grants: [],
}
const fakeToken = 'yfk_0000000000000000_fixture-not-a-generated-token' // a fixture, not a generated token

beforeEach(() => {
  vi.restoreAllMocks()
  for (const f of [listKeys, createKey, revokeKey, createKeyGrant, deleteGrant]) vi.mocked(f).mockReset()
  vi.mocked(listKeys).mockResolvedValue([sync, old])
})

async function mountView() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<i/>' } }, { path: '/keys', component: KeysView }],
  })
  router.push('/keys')
  await router.isReady()
  const wrapper = mount(KeysView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>
const row = (w: Wrapper, id: string) => w.find(`tr[data-key="${id}"]`)

test('lists keys; a read-only key with a write grant is labelled; a revoked key has no actions', async () => {
  const wrapper = await mountView()
  const live = row(wrapper, sync.id)
  expect(live.text()).toContain('sync')
  expect(live.text()).toContain('2027-01-01T00:00:00Z')
  expect(live.text()).toContain('100-1FF write (read only: the key cannot write)')
  expect(live.find('button.revoke').exists()).toBe(true)
  const revoked = row(wrapper, old.id)
  expect(revoked.text()).toContain('2026-03-01T00:00:00Z')
  expect(revoked.text()).toContain('never')
  expect(revoked.find('button.revoke').exists()).toBe(false)
  expect(revoked.find('form.grant').exists()).toBe(false)
})

test('create sends the expiry as RFC 3339 and shows the token once', async () => {
  vi.mocked(createKey).mockResolvedValue({ key: { ...old, id: 'aaaaaaaaaaaaaaaa', name: 'new', revokedAt: null }, token: fakeToken })
  const wrapper = await mountView()
  await wrapper.find('form.create input[name=name]').setValue('new')
  await wrapper.find('form.create input[name=readOnly]').setValue(true)
  await wrapper.find('form.create input[name=expiresAt]').setValue('2030-05-06T07:08')
  await wrapper.find('form.create').trigger('submit')
  await flushPromises()
  expect(createKey).toHaveBeenCalledWith('new', true, new Date('2030-05-06T07:08').toISOString())
  expect(wrapper.find('.token code').text()).toBe(fakeToken)
  expect(wrapper.find('.token').text()).toContain('Copy it now')
  // The next action drops it.
  vi.mocked(deleteGrant).mockResolvedValue()
  await row(wrapper, sync.id).find('button.delete-grant').trigger('click')
  await flushPromises()
  expect(deleteGrant).toHaveBeenCalledWith(7)
  expect(wrapper.find('.token').exists()).toBe(false)
})

test('create without an expiry sends none', async () => {
  vi.mocked(createKey).mockResolvedValue({ key: { ...old, revokedAt: null }, token: fakeToken })
  const wrapper = await mountView()
  await wrapper.find('form.create input[name=name]').setValue('n')
  await wrapper.find('form.create').trigger('submit')
  await flushPromises()
  expect(createKey).toHaveBeenCalledWith('n', false, undefined)
})

test('revoke asks first', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  const wrapper = await mountView()
  await row(wrapper, sync.id).find('button.revoke').trigger('click')
  expect(confirm).toHaveBeenCalledOnce()
  expect(revokeKey).not.toHaveBeenCalled()
  confirm.mockReturnValue(true)
  vi.mocked(revokeKey).mockResolvedValue()
  await row(wrapper, sync.id).find('button.revoke').trigger('click')
  await flushPromises()
  expect(revokeKey).toHaveBeenCalledWith(sync.id)
  expect(listKeys).toHaveBeenCalledTimes(2)
})

test('add grant sends the range upper-cased; a server error is shown', async () => {
  vi.mocked(createKeyGrant).mockRejectedValue(new ApiError(409, 'revoked', 'the key is revoked'))
  const wrapper = await mountView()
  const form = row(wrapper, sync.id).find('form.grant')
  await form.find('input[name=lo]').setValue('2ff')
  await form.find('input[name=hi]').setValue('3aa')
  await form.find('select').setValue('write')
  await form.trigger('submit')
  await flushPromises()
  expect(createKeyGrant).toHaveBeenCalledWith(sync.id, '2FF', '3AA', 'write')
  expect(wrapper.find('[role=alert]').text()).toBe('the key is revoked')
})
