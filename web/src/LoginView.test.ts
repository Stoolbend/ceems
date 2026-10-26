import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import LoginView from './LoginView.vue'
import { login } from './api'
import { user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  login: vi.fn(),
}))

beforeEach(() => {
  vi.mocked(login).mockReset().mockResolvedValue({ id: 1, username: 'conor', admin: true })
  user.value = null
})

async function mountAt(path: string) {
  const stub = { template: '<i/>' }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: stub },
      { path: '/pages/:mpp', component: stub },
      { path: '/login', component: LoginView },
      { path: '/:rest(.*)', component: stub },
    ],
  })
  router.push(path)
  await router.isReady()
  const wrapper = mount(LoginView, { global: { plugins: [router] } })
  return { router, wrapper }
}

async function logIn(redirect: string) {
  const { router, wrapper } = await mountAt(`/login?redirect=${encodeURIComponent(redirect)}`)
  expect(router.currentRoute.value.query.redirect).toBe(redirect)
  await wrapper.find('input[name=username]').setValue('conor')
  await wrapper.find('input[name=password]').setValue('a password')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  return router.currentRoute.value.fullPath
}

// KI-001
test('an off-site redirect goes home instead', async () => {
  for (const evil of ['//evil.example', 'http://evil.example', '/\\evil.example', 'evil.example']) {
    expect(await logIn(evil), evil).toBe('/')
  }
})

test('a local redirect is followed', async () => {
  expect(await logIn('/pages/100?x=1')).toBe('/pages/100?x=1')
})

test('after a password change the login page says so', async () => {
  const { wrapper } = await mountAt('/login?changed=1')
  expect(wrapper.find('[role=status]').text()).toBe('Password changed; log in again')
  const plain = (await mountAt('/login')).wrapper
  expect(plain.find('[role=status]').exists()).toBe(false)
})
