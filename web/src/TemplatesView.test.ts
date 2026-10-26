import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import TemplatesView from './TemplatesView.vue'
import { ApiError, createTemplate, deleteTemplate, getBinding, getTemplate, listBindings, listTemplates, updateTemplate, type Binding, type Template } from './api'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  listTemplates: vi.fn(),
  createTemplate: vi.fn(),
  deleteTemplate: vi.fn(),
  getTemplate: vi.fn(),
  updateTemplate: vi.fn(),
  listBindings: vi.fn(),
  getBinding: vi.fn(),
}))

const created: Template = {
  id: 2, name: 'scores', rowRules: { '17': 'p4name' }, tti: '',
  slots: [
    { name: 'p1name', sub: 0, row: 3, col: 5, width: 9, align: 'left' },
    { name: 'abc', sub: 1, row: 20, col: 0, width: 5, align: 'centre' },
  ],
}

beforeEach(() => {
  vi.mocked(listTemplates).mockReset().mockResolvedValue([{ id: 1, name: 'old', slotCount: 4 }])
  vi.mocked(createTemplate).mockReset()
  vi.mocked(deleteTemplate).mockReset()
  vi.restoreAllMocks()
})

async function mountView() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<i/>' } }, { path: '/templates', component: TemplatesView }] })
  router.push('/templates')
  await router.isReady()
  const wrapper = mount(TemplatesView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>

async function fillForm(wrapper: Wrapper, rules: string) {
  await wrapper.find('input[name=name]').setValue('scores')
  await wrapper.find('textarea').setValue(rules)
  const input = wrapper.find('input[type=file]')
  Object.defineProperty(input.element, 'files', { value: [new File(['PN,10000\r\n'], 'T.tti')] })
  await input.trigger('change')
}

// submit triggers the form and waits for the FileReader and the stubbed API.
async function submit(wrapper: Wrapper) {
  await wrapper.find('form').trigger('submit')
  await vi.waitFor(() => expect(wrapper.find('[role=alert], section').exists()).toBe(true))
  await flushPromises()
}

test('lists templates with slot count and download link', async () => {
  const wrapper = await mountView()
  const row = wrapper.find('tbody tr')
  expect(row.text()).toContain('old')
  expect(row.text()).toContain('4')
  expect(row.find('a[download]').attributes('href')).toBe('/api/v1/templates/1/tti')
})

test('upload sends base64 and the row rules, then shows the parsed slots', async () => {
  vi.mocked(createTemplate).mockResolvedValue(created)
  const wrapper = await mountView()
  await fillForm(wrapper, '{"17": "p4name"}')
  await submit(wrapper)
  expect(createTemplate).toHaveBeenCalledWith('scores', btoa('PN,10000\r\n'), { '17': 'p4name' })
  const slots = wrapper.findAll('table.slots tbody tr').map((r) => r.text())
  expect(slots).toHaveLength(2)
  expect(slots[0]).toContain('p1name')
  expect(slots[1]).toContain('abc')
  expect(slots[1]).toContain('centre')
  expect(listTemplates).toHaveBeenCalledTimes(2)
})

test('bad row rules JSON is refused before upload', async () => {
  const wrapper = await mountView()
  await fillForm(wrapper, '{17:')
  await submit(wrapper)
  expect(createTemplate).not.toHaveBeenCalled()
  expect(wrapper.find('[role=alert]').text()).toContain('JSON')
})

test('a server error is shown', async () => {
  vi.mocked(createTemplate).mockRejectedValue(new ApiError(400, 'invalid_template', 'row rule 17: "x" is not a slot of this template'))
  const wrapper = await mountView()
  await fillForm(wrapper, '{}')
  await submit(wrapper)
  expect(wrapper.find('[role=alert]').text()).toContain('is not a slot')
})

test('delete asks first', async () => {
  vi.mocked(deleteTemplate).mockResolvedValue()
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  const wrapper = await mountView()
  await wrapper.findAll('tbody button').find((b) => b.text() === 'Delete')!.trigger('click')
  expect(confirm).toHaveBeenCalled()
  expect(deleteTemplate).not.toHaveBeenCalled()
  confirm.mockReturnValue(true)
  await wrapper.findAll('tbody button').find((b) => b.text() === 'Delete')!.trigger('click')
  await flushPromises()
  expect(deleteTemplate).toHaveBeenCalledWith(1)
})

test('a closed Help panel links to the full guide', async () => {
  const wrapper = await mountView()
  const help = wrapper.find('details.help')
  expect(help.find('summary').text()).toBe('Help')
  expect(help.attributes('open')).toBeUndefined()
  expect(help.find('a').attributes('href')).toBe('/help')
})

// Replace mode: template 1 "old" has slots a and c, one binding "scores" on page 2F0 uses it.
const stored: Template = {
  id: 1, name: 'old', rowRules: { '17': 'a' }, tti: 'U1RPUkVE',
  slots: [
    { name: 'a', sub: 0, row: 3, col: 0, width: 5, align: 'left' },
    { name: 'c', sub: 0, row: 4, col: 0, width: 5, align: 'left' },
  ],
}
const replaced: Template = { ...stored, name: 'new', slots: [stored.slots[0], { ...stored.slots[1], name: 'b' }] }

async function replaceMode() {
  vi.mocked(getTemplate).mockResolvedValue(stored)
  vi.mocked(listBindings).mockResolvedValue({
    sheetsConfigured: true,
    bindings: [
      { id: 7, name: 'scores', templateId: 1, templateName: 'old', mpp: '2F0', lastRun: null },
      { id: 8, name: 'other', templateId: 9, templateName: 'x', mpp: '2F1', lastRun: null },
    ],
  })
  const wrapper = await mountView()
  await wrapper.findAll('tbody button').find((b) => b.text() === 'Replace')!.trigger('click')
  await flushPromises()
  return wrapper
}

test('Replace prefills the name and rules and lists the bindings that use the template', async () => {
  const wrapper = await replaceMode()
  expect((wrapper.find('input[name=name]').element as HTMLInputElement).value).toBe('old')
  expect(JSON.parse((wrapper.find('textarea').element as HTMLTextAreaElement).value)).toEqual({ '17': 'a' })
  const used = wrapper.find('.used-by').text()
  expect(used).toContain('Used by: scores (page 2F0)')
  expect(used).not.toContain('other')
  expect(used).toContain('next hourly run, or when you press Run now')
  expect(wrapper.find('form button[type=submit]').text()).toBe('Replace')
})

test('Replace says so when no binding uses the template', async () => {
  vi.mocked(getTemplate).mockResolvedValue(stored)
  vi.mocked(listBindings).mockResolvedValue({ sheetsConfigured: true, bindings: [] })
  const wrapper = await mountView()
  await wrapper.findAll('tbody button')[0].trigger('click')
  await flushPromises()
  expect(wrapper.find('.used-by').text()).toBe('No binding uses this template.')
})

test('Replace with no file sends the stored tti by PUT, then shows Added, Removed and stale fixed values', async () => {
  vi.mocked(updateTemplate).mockResolvedValue(replaced)
  vi.mocked(getBinding).mockResolvedValue({ id: 7, static: { c: 'x', a: 'y', b: 'z' } } as unknown as Binding)
  const wrapper = await replaceMode()
  await wrapper.find('input[name=name]').setValue('new')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(updateTemplate).toHaveBeenCalledWith(1, 'new', 'U1RPUkVE', { '17': 'a' })
  expect(wrapper.find('.changes').text()).toBe('Added: b. Removed: c.')
  expect(wrapper.findAll('table.slots tbody tr')).toHaveLength(2)
  expect(wrapper.find('.stale').text()).toBe('scores (page 2F0) still has fixed values for: c. They are ignored. Open the binding and save it to drop them.')
  expect(wrapper.find('form button[type=submit]').text()).toBe('Upload')
})

test('a binding with no stale key adds no stale line, and an unchanged slot set reads none', async () => {
  vi.mocked(updateTemplate).mockResolvedValue(stored)
  vi.mocked(getBinding).mockResolvedValue({ id: 7, static: { a: 'y' } } as unknown as Binding)
  const wrapper = await replaceMode()
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(wrapper.find('.changes').text()).toBe('Added: none. Removed: none.')
  expect(wrapper.find('.stale').exists()).toBe(false)
})

test('a failed fixed-value check after a saved Replace still reloads the list and says the template was saved', async () => {
  vi.mocked(updateTemplate).mockResolvedValue(replaced)
  vi.mocked(getBinding).mockRejectedValue(new Error('boom'))
  const wrapper = await replaceMode()
  const before = vi.mocked(listTemplates).mock.calls.length
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(vi.mocked(listTemplates).mock.calls.length).toBeGreaterThan(before)
  expect(wrapper.find('.changes').text()).toBe('Added: b. Removed: c.')
  expect(wrapper.find('[role=alert]').text()).toBe('Template saved, but the fixed-value check failed: boom')
})

test('a 409 on Replace is shown verbatim and keeps replace mode', async () => {
  const msg = 'binding scores (page 2F0): slot p9name needs a value'
  vi.mocked(updateTemplate).mockRejectedValue(new ApiError(409, 'in_use', msg))
  const wrapper = await replaceMode()
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  expect(wrapper.find('[role=alert]').text()).toBe(msg)
  expect(wrapper.find('form button[type=submit]').text()).toBe('Replace')
})

test('Cancel returns to create mode', async () => {
  const wrapper = await replaceMode()
  await wrapper.findAll('form button').find((b) => b.text() === 'Cancel')!.trigger('click')
  expect(wrapper.find('form button[type=submit]').text()).toBe('Upload')
  expect((wrapper.find('input[name=name]').element as HTMLInputElement).value).toBe('')
  expect(wrapper.find('.used-by').exists()).toBe(false)
})
