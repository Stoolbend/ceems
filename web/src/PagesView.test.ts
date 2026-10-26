import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { expect, test, vi } from 'vitest'
import PagesView from './PagesView.vue'
import { ApiError, createPage, getPreview, listPages, putPage, type Page, type PageSummary } from './api'
import { encode } from './zxnet'

vi.mock('./api', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api')>()), listPages: vi.fn(), createPage: vi.fn(), putPage: vi.fn(), getPreview: vi.fn() }))

const row = (mpp: string): PageSummary => ({ mpp, description: `page ${mpp}`, subpages: 1, transmit: true, updatedAt: '2026-01-01T00:00:00Z', updatedBy: 'cli' })

async function mountView() {
  vi.mocked(listPages).mockResolvedValue([row('100'), row('101'), row('2A0'), row('8FF')])
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: PagesView }, { path: '/pages/:mpp', component: { template: '<i/>' } }] })
  router.push('/')
  await router.isReady()
  const wrapper = mount(PagesView, { global: { plugins: [router] } })
  await flushPromises()
  return { router, wrapper }
}

test('shows magazine counts and all pages', async () => {
  const { wrapper } = await mountView()
  const labels = wrapper.findAll('nav button').map((b) => b.text())
  expect(labels).toEqual(['All (4)', '1 (2)', '2 (1)', '3 (0)', '4 (0)', '5 (0)', '6 (0)', '7 (0)', '8 (1)'])
  expect(wrapper.findAll('tbody tr')).toHaveLength(4)
})

test('magazine button filters the table', async () => {
  const { wrapper } = await mountView()
  await wrapper.findAll('nav button')[2].trigger('click')
  const rows = wrapper.findAll('tbody tr')
  expect(rows).toHaveLength(1)
  expect(rows[0].text()).toContain('2A0')
})

test('page number links to its page route', async () => {
  const { wrapper } = await mountView()
  expect(wrapper.find('tbody a').attributes('href')).toBe('/pages/100')
})

test('jump with a valid number normalises and navigates', async () => {
  const { router, wrapper } = await mountView()
  await wrapper.find('input[name=jump]').setValue('8ff')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(router.currentRoute.value.path).toBe('/pages/8FF')
})

test.each(['900', '0AB', '10', 'xyz'])('jump with %s shows a message and stays', async (bad) => {
  const { router, wrapper } = await mountView()
  await wrapper.find('input[name=jump]').setValue(bad)
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(router.currentRoute.value.path).toBe('/')
  expect(wrapper.find('form [role=alert]').exists()).toBe(true)
})

async function create(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper'], value: string) {
  await wrapper.find('input[name=create]').setValue(value)
  await wrapper.find('form.create').trigger('submit')
  await flushPromises()
}

test('create posts the normalised number and opens the new page', async () => {
  const { router, wrapper } = await mountView()
  vi.mocked(createPage).mockResolvedValue({} as never)
  await create(wrapper, '3a0')
  expect(createPage).toHaveBeenCalledWith('3A0')
  expect(router.currentRoute.value.path).toBe('/pages/3A0')
})

test.each(['900', '0AB', '10', 'xyz'])('create with %s shows a message and sends nothing', async (bad) => {
  vi.mocked(createPage).mockClear()
  const { router, wrapper } = await mountView()
  await create(wrapper, bad)
  expect(createPage).not.toHaveBeenCalled()
  expect(router.currentRoute.value.path).toBe('/')
  expect(wrapper.find('form.create [role=alert]').exists()).toBe(true)
})

test('create at a taken number shows the server message and stays', async () => {
  const { router, wrapper } = await mountView()
  vi.mocked(createPage).mockRejectedValue(new ApiError(409, 'exists', 'page 100 already exists'))
  await create(wrapper, '100')
  expect(wrapper.find('form.create [role=alert]').text()).toBe('page 100 already exists')
  expect(router.currentRoute.value.path).toBe('/')
})

// Create from a zxnet URL (T-026).
const urlRows = Array.from({ length: 25 }, (_, r) => (r === 0 ? 'X'.repeat(40) : `R${r} `.padEnd(40, '.')))
const urlWith = (pn: string) => encode(urlRows, pn || '100', []).replace(/:PN=[^:]*/, pn === '' ? '' : `:PN=${pn}`)
const created = (): Page => ({
  mpp: '3A0', description: '', transmit: false, updatedAt: '', updatedBy: 'u', etag: 'fresh', binding: null, fastextGroup: null, canWrite: true,
  subpages: [{ cycleSeconds: null, cycleMode: null, links: null, rows: ['HEADER', ...Array(24).fill(null)] }],
})

async function createWithUrl(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper'], num: string, url: string) {
  await wrapper.find('input[name=createUrl]').setValue(url)
  await create(wrapper, num)
}

test('create with a number and a URL creates, fills rows 1 to 24 with the fresh etag, and opens the page', async () => {
  vi.mocked(createPage).mockReset().mockResolvedValue(created())
  vi.mocked(putPage).mockReset().mockResolvedValue(created())
  const { router, wrapper } = await mountView()
  await createWithUrl(wrapper, '3a0', urlWith('1A0'))
  expect(createPage).toHaveBeenCalledWith('3A0')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  expect(sent.etag).toBe('fresh')
  expect(sent.transmit).toBe(false)
  expect(sent.subpages[0].rows[0]).toBe('HEADER')
  for (let r = 1; r <= 24; r++) expect(sent.subpages[0].rows[r]).toBe(urlRows[r])
  expect(router.currentRoute.value.path).toBe('/pages/3A0')
})

test('create with the number blank uses the URL PN', async () => {
  vi.mocked(createPage).mockReset().mockResolvedValue(created())
  vi.mocked(putPage).mockReset().mockResolvedValue(created())
  const { router, wrapper } = await mountView()
  await createWithUrl(wrapper, '', urlWith('1A0'))
  expect(createPage).toHaveBeenCalledWith('1A0')
  expect(router.currentRoute.value.path).toBe('/pages/1A0')
})

test('create with the number blank and no valid PN shows the number error and sends nothing', async () => {
  vi.mocked(createPage).mockReset()
  vi.mocked(putPage).mockReset()
  const { router, wrapper } = await mountView()
  await createWithUrl(wrapper, '', urlWith(''))
  expect(wrapper.find('form.create [role=alert]').text()).toContain('Enter three hex digits')
  expect(createPage).not.toHaveBeenCalled()
  expect(putPage).not.toHaveBeenCalled()
  expect(router.currentRoute.value.path).toBe('/')
})

test('create with a refused URL shows the decode message and calls neither createPage nor putPage', async () => {
  vi.mocked(createPage).mockReset()
  vi.mocked(putPage).mockReset()
  const { router, wrapper } = await mountView()
  await createWithUrl(wrapper, '3A0', '#0:ABC')
  expect(wrapper.find('form.create [role=alert]').text()).toBe('page data must be 1120 or 1167 characters, got 3')
  expect(createPage).not.toHaveBeenCalled()
  expect(putPage).not.toHaveBeenCalled()
  expect(router.currentRoute.value.path).toBe('/')
})

test('create where the fill fails says the page was created blank and stays', async () => {
  vi.mocked(createPage).mockReset().mockResolvedValue(created())
  vi.mocked(putPage).mockReset().mockRejectedValue(new ApiError(403, 'forbidden', 'no write access'))
  const { router, wrapper } = await mountView()
  await createWithUrl(wrapper, '3A0', urlWith('3A0'))
  expect(wrapper.find('form.create [role=alert]').text()).toBe('Page 3A0 was created blank; the import failed: no write access')
  expect(router.currentRoute.value.path).toBe('/')
})

test('a page with transmit off is dimmed and marked off air', async () => {
  vi.mocked(listPages).mockResolvedValue([{ ...row('100'), transmit: false }, row('101')])
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: PagesView }, { path: '/pages/:mpp', component: { template: '<i/>' } }] })
  router.push('/')
  await router.isReady()
  const wrapper = mount(PagesView, { global: { plugins: [router] } })
  await flushPromises()
  const [off, on] = wrapper.findAll('tbody tr')
  expect(off.classes()).toContain('offair')
  expect(off.text()).toContain('off air')
  expect(on.classes()).not.toContain('offair')
  expect(on.text()).not.toContain('off air')
})

test('Preview on a row opens the preview dialog for that page', async () => {
  const { wrapper } = await mountView()
  const showModal = vi.fn(); HTMLDialogElement.prototype.showModal = showModal
  vi.mocked(getPreview).mockResolvedValue({ mpp: '2A0', description: '', headerTemplate: '', subpages: [{ rows: Array(25).fill(null), seconds: 8 }] })
  await wrapper.findAll('tbody tr')[2].find('button.preview').trigger('click')
  expect(getPreview).toHaveBeenCalledWith('2A0')
  expect(showModal).toHaveBeenCalled()
})
