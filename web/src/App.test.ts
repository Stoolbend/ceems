import { config, mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import App from './App.vue'
import { ApiError, getSite, listPages, login, logout, me, onUnauthorized, request } from './api'
import { authGuard, expireSession, routes } from './router'
import { systemName, user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  me: vi.fn(),
  getSite: vi.fn(),
  listPages: vi.fn().mockResolvedValue([]),
  login: vi.fn(),
  logout: vi.fn(),
}))

const conor = { id: 1, username: 'conor', admin: true }

beforeEach(() => {
  vi.mocked(me).mockReset()
  vi.mocked(getSite).mockReset().mockResolvedValue({ systemName: 'CeeMS' })
  systemName.value = 'CeeMS'
  vi.mocked(login).mockReset()
  vi.mocked(logout).mockReset()
  user.value = null
})

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
  delete document.documentElement.dataset.scheme
})

async function mountAt(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes })
  router.beforeEach(authGuard)
  onUnauthorized(() => expireSession(router))
  router.push(path)
  await router.isReady()
  const wrapper = mount(App, { global: { plugins: [router] } })
  await flushPromises()
  return { router, wrapper }
}

test('shell renders the title and the home route', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  expect(wrapper.find('h1').text()).toBe('CeeMS')
  expect(wrapper.text()).toContain('All (0)')
  expect(wrapper.text()).toContain('conor')
})

test('the h1 and the document title show the public system name, signed out and signed in', async () => {
  vi.mocked(getSite).mockResolvedValue({ systemName: 'SAMPLE TV' })
  vi.mocked(me).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  const { wrapper } = await mountAt('/login')
  expect(wrapper.find('h1').text()).toBe('SAMPLE TV')
  expect(document.title).toBe('SAMPLE TV')
  user.value = conor
  await flushPromises()
  expect(wrapper.find('h1').text()).toBe('SAMPLE TV')
})

test('the h1 and the document title stay CeeMS when the site call fails', async () => {
  vi.mocked(getSite).mockRejectedValue(new Error('down'))
  vi.mocked(me).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  const { wrapper } = await mountAt('/login')
  expect(wrapper.find('h1').text()).toBe('CeeMS')
  expect(document.title).toBe('CeeMS')
})

test('a non-admin has no Settings link, and an anonymous visitor to /settings is sent to login', async () => {
  vi.mocked(me).mockResolvedValue({ id: 2, username: 'guest', admin: false })
  const { wrapper } = await mountAt('/')
  expect(wrapper.findAll('header a').some((a) => a.attributes('href') === '/settings')).toBe(false)
  wrapper.unmount()
  user.value = null
  vi.mocked(me).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  const { router } = await mountAt('/settings')
  expect(router.currentRoute.value.path).toBe('/login')
})

test('anonymous navigation lands on /login with the redirect', async () => {
  vi.mocked(me).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  const { router, wrapper } = await mountAt('/')
  expect(router.currentRoute.value.path).toBe('/login')
  expect(router.currentRoute.value.query.redirect).toBe('/')
  expect(wrapper.text()).not.toContain('All (0)')
  expect(wrapper.find('input[name=username]').exists()).toBe(true)
})

test('successful login goes to the redirect', async () => {
  vi.mocked(me).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  vi.mocked(login).mockResolvedValue(conor)
  const { router, wrapper } = await mountAt('/?from=test')
  expect(router.currentRoute.value.path).toBe('/login')
  expect(router.currentRoute.value.query.redirect).toBe('/?from=test')
  await wrapper.find('input[name=username]').setValue('conor')
  await wrapper.find('input[name=password]').setValue('a password')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(login).toHaveBeenCalledWith('conor', 'a password')
  expect(router.currentRoute.value.fullPath).toBe('/?from=test')
  expect(wrapper.text()).toContain('All (0)')
})

test('failed login shows the server message', async () => {
  vi.mocked(login).mockRejectedValue(new ApiError(401, 'invalid_credentials', 'invalid username or password'))
  const { router, wrapper } = await mountAt('/login')
  await wrapper.find('input[name=username]').setValue('conor')
  await wrapper.find('input[name=password]').setValue('wrong')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(router.currentRoute.value.path).toBe('/login')
  expect(wrapper.find('[role=alert]').text()).toBe('invalid username or password')
  expect(wrapper.find('button[type=submit]').attributes('disabled')).toBeUndefined()
})

test('logout returns to /login', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  vi.mocked(logout).mockResolvedValue()
  const { router, wrapper } = await mountAt('/')
  await wrapper.find('header button').trigger('click')
  await flushPromises()
  expect(logout).toHaveBeenCalledOnce()
  expect(user.value).toBeNull()
  expect(router.currentRoute.value.path).toBe('/login')
})

test('admins see the Templates, Bindings, Fastext, Users and Keys links; others do not', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  let { wrapper } = await mountAt('/')
  const links = (w: typeof wrapper) => w.findAll('header a').map((a) => `${a.text()} ${a.attributes('href')}`)
  expect(links(wrapper)).toEqual(['Pages /', 'Templates /templates', 'Bindings /bindings', 'Fastext /fastext', 'Users /users', 'Keys /keys', 'Settings /settings', '[conor] /password'])
  wrapper.unmount()
  user.value = null
  vi.mocked(me).mockResolvedValue({ id: 2, username: 'guest', admin: false })
  ;({ wrapper } = await mountAt('/'))
  expect(links(wrapper)).toEqual(['Pages /', '[guest] /password'])
})

test('main is full width on the page editor and a centred container elsewhere', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  let { wrapper } = await mountAt('/pages/100')
  expect(wrapper.find('main').classes()).toEqual(['container-fluid'])
  wrapper.unmount()
  ;({ wrapper } = await mountAt('/'))
  expect(wrapper.find('main').classes()).toEqual(['container'])
})

function stub401() {
  vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } }), { status: 401 })))
}

test('a 401 on an API call clears the user and returns to login with the redirect', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { router } = await mountAt('/pages/100')
  expect(vi.mocked(me)).toHaveBeenCalledTimes(1)
  stub401()
  await expect(request('GET', '/api/v1/pages')).rejects.toBeInstanceOf(ApiError)
  await flushPromises()
  expect(user.value).toBeNull()
  expect(router.currentRoute.value.path).toBe('/login')
  expect(router.currentRoute.value.query.redirect).toBe('/pages/100')
  expect(vi.mocked(me)).toHaveBeenCalledTimes(1)
})

test('a second 401 on /login does not stack redirects', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { router } = await mountAt('/pages/100')
  stub401()
  await request('GET', '/api/v1/pages').catch(() => {})
  await flushPromises()
  const first = router.currentRoute.value.fullPath
  await request('GET', '/api/v1/pages').catch(() => {})
  await flushPromises()
  expect(router.currentRoute.value.fullPath).toBe(first)
})

test('logout with the session already gone still ends on /login', async () => {
  config.global.config.errorHandler = () => {} // the rejected logout is rethrown by the click handler; the test only checks where the user ends
  vi.mocked(me).mockResolvedValue(conor)
  vi.mocked(logout).mockRejectedValue(new ApiError(401, 'unauthorized', 'login required'))
  const { router, wrapper } = await mountAt('/')
  await wrapper.find('header button').trigger('click')
  await flushPromises()
  expect(user.value).toBeNull()
  delete config.global.config.errorHandler
  expect(router.currentRoute.value.path).toBe('/login')
})

const darkButton = (w: Awaited<ReturnType<typeof mountAt>>['wrapper']) => w.findAll('footer button').find((b) => b.text() === 'Dark')!

test('Dark sets the attribute, the storage key and aria-pressed; a second click undoes them', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  await darkButton(wrapper).trigger('click')
  expect(document.documentElement.dataset.scheme).toBe('dark')
  expect(localStorage.getItem('ceems.scheme')).toBe('dark')
  expect(darkButton(wrapper).attributes('aria-pressed')).toBe('true')
  await darkButton(wrapper).trigger('click')
  expect(document.documentElement.dataset.scheme).toBeUndefined()
  expect(localStorage.getItem('ceems.scheme')).toBeNull()
  expect(darkButton(wrapper).attributes('aria-pressed')).toBe('false')
})

test('the Dark button starts pressed when the attribute is already set', async () => {
  document.documentElement.dataset.scheme = 'dark'
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  expect(darkButton(wrapper).attributes('aria-pressed')).toBe('true')
})

test('the Dark button still flips the attribute when localStorage throws', async () => {
  const boom = () => { throw new Error('blocked') }
  vi.stubGlobal('localStorage', { getItem: boom, setItem: boom, removeItem: boom, clear: () => {} })
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  await darkButton(wrapper).trigger('click')
  expect(document.documentElement.dataset.scheme).toBe('dark')
})

test('Logout is the last header item, in the same list as the username link', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  const items = wrapper.findAll('header li')
  expect(items[items.length - 1].text()).toBe('Logout')
  const group = wrapper.findAll('header ul').at(-1)!
  expect(group.findAll('li').map((li) => li.text())).toEqual(['[conor]', 'Logout'])
  expect(wrapper.find('header').text()).not.toContain('Dark')
})

test('the footer shows no copyright line, the GitHub link and Dark, also on /login', async () => {
  const { wrapper } = await mountAt('/login')
  const footer = wrapper.find('footer')
  expect(footer.text()).not.toContain('Copyright')
  const a = footer.find('a')
  expect(a.attributes('href')).toBe('https://github.com/Stoolbend/ceems')
  expect(a.attributes('target')).toBe('_blank')
  expect(a.attributes('rel')).toBe('noopener')
  expect(darkButton(wrapper).exists()).toBe(true)
})

test('the footer lists GitHub and Credits; Credits opens the modal dialog', async () => {
  const showModal = vi.fn() // jsdom has no showModal
  HTMLDialogElement.prototype.showModal = showModal
  try {
    const { wrapper } = await mountAt('/login')
    const footer = wrapper.find('footer')
    expect(footer.find('ul').findAll('li').map((li) => li.text())).toEqual(['GitHub', 'Credits'])
    expect(footer.find('a').text()).toBe('GitHub')
    await footer.findAll('button').find((b) => b.text() === 'Credits')!.trigger('click')
    expect(showModal).toHaveBeenCalledOnce()
  } finally {
    delete (HTMLDialogElement.prototype as Partial<HTMLDialogElement>).showModal
  }
})

test('signed in, the footer ends with Help, linking to /help', async () => {
  vi.mocked(me).mockResolvedValue(conor)
  const { wrapper } = await mountAt('/')
  const footer = wrapper.find('footer')
  expect(footer.find('ul').findAll('li').map((li) => li.text())).toEqual(['GitHub', 'Credits', 'Help'])
  expect(footer.findAll('a').find((a) => a.text() === 'Help')!.attributes('href')).toBe('/help')
})

test('the footer shows the version then a separator, then the Dark button', async () => {
  const { wrapper } = await mountAt('/login')
  const items = wrapper.findAll('footer ul')[1].findAll('li')
  expect(items.map((li) => li.text())).toEqual(['CeeMS dev', '|', 'Dark'])
})
