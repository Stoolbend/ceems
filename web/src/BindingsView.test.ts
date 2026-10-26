import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import BindingsView from './BindingsView.vue'
import {
  ApiError, createBinding, dataKeys, deleteBinding, getBinding, getPreview, getTemplate, listBindings, listRuns, listTemplates, runBinding,
  updateBinding, type Binding, type BindingSummary, type Template,
} from './api'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  listBindings: vi.fn(),
  listTemplates: vi.fn(),
  getTemplate: vi.fn(),
  getBinding: vi.fn(),
  createBinding: vi.fn(),
  updateBinding: vi.fn(),
  deleteBinding: vi.fn(),
  runBinding: vi.fn(),
  listRuns: vi.fn(),
  getPreview: vi.fn(),
}))

// The T-013 two-slot fixture (TemplatesView.test.ts) and a second template for switching.
const scores: Template = {
  id: 1, name: 'scores', rowRules: {}, tti: '',
  slots: [
    { name: 'p1name', sub: 0, row: 3, col: 5, width: 9, align: 'left' },
    { name: 'abc', sub: 1, row: 20, col: 0, width: 5, align: 'centre' },
  ],
}
const games: Template = {
  id: 2, name: 'games', rowRules: {}, tti: '',
  slots: [
    { name: 'gname', sub: 0, row: 1, col: 0, width: 10, align: 'left' },
    { name: 'sty', sub: 0, row: 2, col: 0, width: 3, align: 'left' },
    { name: 'abc', sub: 0, row: 3, col: 0, width: 4, align: 'left' },
    { name: 'gname', sub: 0, row: 4, col: 0, width: 20, align: 'left' },
  ],
}
const pacman: BindingSummary = { id: 7, name: 'pacman', templateId: 1, templateName: 'scores', mpp: '4A0', lastRun: null }

beforeEach(() => {
  vi.mocked(listBindings).mockReset().mockResolvedValue({ sheetsConfigured: true, bindings: [pacman] })
  vi.mocked(listTemplates).mockReset().mockResolvedValue([{ id: 1, name: 'scores', slotCount: 2 }, { id: 2, name: 'games', slotCount: 4 }])
  vi.mocked(getTemplate).mockReset().mockImplementation(async (id) => (id === 1 ? scores : games))
  for (const f of [getBinding, createBinding, updateBinding, deleteBinding, runBinding, listRuns]) vi.mocked(f).mockReset()
  vi.restoreAllMocks()
})

async function mountView() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<i/>' } }, { path: '/bindings', component: BindingsView }, { path: '/pages/:mpp', component: { template: '<i/>' } }] })
  router.push('/bindings')
  await router.isReady()
  const wrapper = mount(BindingsView, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>

// openNew opens the create form on template id with columns p#name and rows 1.
async function openNew(wrapper: Wrapper, id: number) {
  await wrapper.find('button.new').trigger('click')
  await wrapper.find('input[name=name]').setValue('pacman')
  await wrapper.find('select[name=template]').setValue(id)
  await flushPromises()
  await wrapper.find('input[name=columns]').setValue('p#name')
  await wrapper.find('input[name=rows]').setValue(1)
}

const staticNames = (w: Wrapper) => w.findAll('.slot input').map((i) => i.attributes('aria-label'))

test('dataKeys matches jobs.DataKeys on the shared examples', () => {
  expect([...dataKeys(['p#name', 'p#score'], 2)].sort()).toEqual(['p1name', 'p1score', 'p2name', 'p2score', 'updated'])
  expect([...dataKeys(['p#name'], 1)].sort()).toEqual(['p1name', 'updated'])
})

test('shows the not-configured line only when Sheets is not configured', async () => {
  let wrapper = await mountView()
  expect(wrapper.find('[role=status]').exists()).toBe(false)
  wrapper.unmount()
  vi.mocked(listBindings).mockResolvedValue({ sheetsConfigured: false, bindings: [] })
  wrapper = await mountView()
  expect(wrapper.find('[role=status]').text()).toContain('Google Sheets not configured: scheduled runs are off.')
  expect(wrapper.find('.off').classes()).toContain('warning')
  wrapper.unmount()
})

test('lists bindings with a page link and never loads a single binding until Edit', async () => {
  vi.mocked(listBindings).mockResolvedValue({
    sheetsConfigured: true,
    bindings: [{ ...pacman, lastRun: { id: 1, actor: 'schedule', startedAt: 's', finishedAt: '2026-10-12T10:00:01Z', outcome: 'failed', error: 'sheet unreachable' } }],
  })
  vi.mocked(getBinding).mockResolvedValue({ ...pacman, source: { sheet: 'S', range: 'C5:E', columns: ['p#name'], rows: 1, format: {} }, static: { abc: 'x' } })
  const wrapper = await mountView()
  const row = wrapper.find('tbody tr')
  expect(row.text()).toContain('scores')
  expect(row.text()).toContain('failed: sheet unreachable')
  expect(row.find('a').attributes('href')).toBe('/pages/4A0')
  expect(getBinding).not.toHaveBeenCalled()
  await row.find('button.edit').trigger('click')
  await flushPromises()
  expect(getBinding).toHaveBeenCalledWith(7)
  expect((wrapper.find('input[name=sheet]').element as HTMLInputElement).value).toBe('S')
  expect(wrapper.find('input[name=mpp]').attributes('disabled')).toBeUndefined()
  wrapper.unmount()
})

test('one input per fixed slot; slots the sheet fills get none', async () => {
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  expect(staticNames(wrapper)).toEqual(['abc'])
  const input = wrapper.find('.slot input')
  expect(input.attributes('maxlength')).toBe('5')
  expect(wrapper.find('.slot').text()).toContain('abc (width 5)')
  expect(wrapper.find('.slot .count').text()).toBe('0/5')
  expect(wrapper.find('.filled').text()).toBe('filled from the sheet: p1name')
  wrapper.unmount()
})

test('changing the template rebuilds the inputs and warns about dropped values', async () => {
  const wrapper = await mountView()
  await openNew(wrapper, 2)
  expect(staticNames(wrapper)).toEqual(['gname', 'sty', 'abc'])
  expect(wrapper.find('.slot input').attributes('maxlength')).toBe('10') // the first occurrence's width
  const inputs = wrapper.findAll('.slot input')
  await inputs[0].setValue('Pac-Man')
  await inputs[1].setValue('ab')
  await inputs[2].setValue('kept')
  await wrapper.find('select[name=template]').setValue(1)
  await flushPromises()
  expect(staticNames(wrapper)).toEqual(['abc'])
  expect((wrapper.find('.slot input').element as HTMLInputElement).value).toBe('kept')
  expect(wrapper.find('.dropped').text()).toBe('Dropped values for: gname, sty (not fixed slots of the selected template)')
  wrapper.unmount()
})

test('a palette click inserts the control picture at the caret and the counter follows', async () => {
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  const input = wrapper.find('.slot input')
  await input.setValue('AB')
  const el = input.element as HTMLInputElement
  el.setSelectionRange(1, 1)
  await input.trigger('focus')
  await wrapper.find('.palette button[title="Alpha red"]').trigger('click')
  expect(el.value).toBe('A␁B')
  expect(wrapper.find('.slot .count').text()).toBe('3/5')
  wrapper.unmount()
})

test('save sends fromEditable output for every fixed slot', async () => {
  vi.mocked(createBinding).mockResolvedValue({} as Binding)
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  await wrapper.find('input[name=mpp]').setValue('4a0')
  await wrapper.find('input[name=sheet]').setValue('FAKE-SHEET')
  await wrapper.find('input[name=range]').setValue('C5:E')
  await wrapper.find('textarea[name=formats]').setValue('p#name=text\n')
  await wrapper.find('.slot input').setValue('␁Hi␡')
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  expect(createBinding).toHaveBeenCalledWith({
    name: 'pacman', templateId: 1, mpp: '4A0',
    source: { sheet: 'FAKE-SHEET', range: 'C5:E', columns: ['p#name'], rows: 1, format: { 'p#name': 'text' } },
    static: { abc: '\x01Hi\x7f' },
  })
  expect(wrapper.find('form.binding').exists()).toBe(false)
  wrapper.unmount()
})

test('a value that is not a teletext character blocks save', async () => {
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  await wrapper.find('.slot input').setValue('café')
  expect(wrapper.find('.slot input').attributes('aria-invalid')).toBe('true')
  expect(wrapper.find('form.binding button[type=submit]').attributes('disabled')).toBeDefined()
  await wrapper.find('form.binding').trigger('submit')
  expect(createBinding).not.toHaveBeenCalled()
  wrapper.unmount()
})

test('a server refusal is shown next to the form', async () => {
  vi.mocked(createBinding).mockRejectedValue(new ApiError(400, 'invalid_static', 'slot abc needs a value'))
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  expect(wrapper.find('form.binding [role=alert]').text()).toBe('slot abc needs a value')
  wrapper.unmount()
})

test('editing the page number sends the new mpp, and a 409 is shown next to the form', async () => {
  vi.mocked(getBinding).mockResolvedValue({ ...pacman, source: { sheet: 'S', range: 'C5:E', columns: ['p#name'], rows: 1, format: {} }, static: { abc: 'x' } })
  vi.mocked(updateBinding).mockRejectedValueOnce(new ApiError(409, 'exists', 'page 4A1 already exists; the binding was not moved'))
  const wrapper = await mountView()
  await wrapper.find('tbody tr button.edit').trigger('click')
  await flushPromises()
  await wrapper.find('input[name=mpp]').setValue('4a1')
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  expect(updateBinding).toHaveBeenCalledWith(7, expect.objectContaining({ mpp: '4A1' }))
  expect(wrapper.find('form.binding [role=alert]').text()).toBe('page 4A1 already exists; the binding was not moved')
  wrapper.unmount()
})

test('run now shows the outcome, then a 409 as a retry hint, and the runs table loads on select', async () => {
  vi.mocked(runBinding).mockResolvedValueOnce({ id: 3, actor: 'user:1', startedAt: 'a', finishedAt: 'b', outcome: 'failed', error: 'googleKeyFile is not configured' })
  vi.mocked(listRuns).mockResolvedValue([{ id: 3, actor: 'user:1', startedAt: 'a', finishedAt: 'b', outcome: 'failed', error: 'googleKeyFile is not configured' }])
  const wrapper = await mountView()
  await wrapper.find('button.run').trigger('click')
  await flushPromises()
  expect(wrapper.find('.result').text()).toBe('failed: googleKeyFile is not configured')
  vi.mocked(runBinding).mockRejectedValueOnce(new ApiError(409, 'run_in_progress', 'a run is in progress; try again shortly'))
  await wrapper.find('button.run').trigger('click')
  await flushPromises()
  expect(wrapper.find('.result').text()).toBe('a run is in progress; try again shortly')
  await wrapper.find('button.select').trigger('click')
  await flushPromises()
  expect(listRuns).toHaveBeenCalledWith(7)
  expect(wrapper.find('.runs tbody tr').text()).toContain('googleKeyFile is not configured')
  wrapper.unmount()
})

test('run now is disabled while in flight', async () => {
  let finish!: () => void
  vi.mocked(runBinding).mockReturnValue(new Promise((resolve) => { finish = () => resolve({ id: 1, actor: 'user:1', startedAt: 'a', finishedAt: 'b', outcome: 'ok', error: '' }) }))
  const wrapper = await mountView()
  await wrapper.find('button.run').trigger('click')
  expect(wrapper.find('button.run').attributes('disabled')).toBeDefined()
  finish()
  await flushPromises()
  expect(wrapper.find('button.run').attributes('disabled')).toBeUndefined()
  expect(wrapper.find('.result').text()).toBe('ok')
  wrapper.unmount()
})

test('delete asks first with the page it releases', async () => {
  vi.mocked(deleteBinding).mockResolvedValue()
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  const wrapper = await mountView()
  await wrapper.find('button.delete').trigger('click')
  expect(confirm).toHaveBeenCalledWith('Delete binding pacman? Page 4A0 keeps its content and becomes editable.')
  expect(deleteBinding).not.toHaveBeenCalled()
  confirm.mockReturnValue(true)
  await wrapper.find('button.delete').trigger('click')
  await flushPromises()
  expect(deleteBinding).toHaveBeenCalledWith(7)
  wrapper.unmount()
})

test('a closed Help panel links to the full guide', async () => {
  const wrapper = await mountView()
  const help = wrapper.find('details.help')
  expect(help.find('summary').text()).toBe('Help')
  expect(help.attributes('open')).toBeUndefined()
  expect(help.find('a').attributes('href')).toBe('/help')
})

const full = { ...pacman, source: { sheet: 'S1', range: 'C5:E', columns: ['p#name', 'p#date'], rows: 2, format: { 'p#date': 'date' } }, static: { abc: 'x' } }

test('copy fills everything but the page, shows the copy heading, and saves as a create', async () => {
  vi.mocked(getBinding).mockResolvedValue(full)
  vi.mocked(createBinding).mockResolvedValue({} as Binding)
  const wrapper = await mountView()
  await wrapper.find('tbody tr button.copy').trigger('click')
  await flushPromises()
  const val = (n: string) => (wrapper.find(`input[name=${n}]`).element as HTMLInputElement).value
  expect(wrapper.find('form.binding h2').text()).toBe('New binding (copy of pacman)')
  expect(val('name')).toBe('pacman copy')
  expect(val('mpp')).toBe('')
  expect(document.activeElement).toBe(wrapper.find('input[name=mpp]').element)
  expect((wrapper.find('select[name=template]').element as HTMLSelectElement).value).toBe('1')
  expect(val('sheet')).toBe('S1')
  expect(val('range')).toBe('C5:E')
  expect(val('columns')).toBe('p#name, p#date')
  expect(val('rows')).toBe('2')
  expect((wrapper.find('textarea[name=formats]').element as HTMLTextAreaElement).value).toBe('p#date=date')
  expect((wrapper.find('.slot input').element as HTMLInputElement).value).toBe('x')
  await wrapper.find('input[name=mpp]').setValue('4b0')
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  expect(updateBinding).not.toHaveBeenCalled()
  expect(createBinding).toHaveBeenCalledWith({
    name: 'pacman copy', templateId: 1, mpp: '4B0',
    source: { sheet: 'S1', range: 'C5:E', columns: ['p#name', 'p#date'], rows: 2, format: { 'p#date': 'date' } },
    static: { abc: 'x' },
  })
  wrapper.unmount()
})

test('copy of a 100-character name stays 100 characters', async () => {
  const long = 'n'.repeat(100)
  vi.mocked(getBinding).mockResolvedValue({ ...full, name: long })
  const wrapper = await mountView()
  await wrapper.find('tbody tr button.copy').trigger('click')
  await flushPromises()
  expect((wrapper.find('input[name=name]').element as HTMLInputElement).value).toBe(`${long} copy`.slice(0, 100))
  expect((wrapper.find('input[name=name]').element as HTMLInputElement).value).toHaveLength(100)
  wrapper.unmount()
})

test('a Formats line with options round-trips to the source', async () => {
  vi.mocked(createBinding).mockResolvedValue({} as Binding)
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  await wrapper.find('input[name=mpp]').setValue('4a0')
  await wrapper.find('input[name=sheet]').setValue('FAKE-SHEET')
  await wrapper.find('input[name=range]').setValue('C5:E')
  await wrapper.find('textarea[name=formats]').setValue('p#name=number unit=sec')
  await wrapper.find('.slot input').setValue('Hi')
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  const arg = vi.mocked(createBinding).mock.calls.at(-1)![0]
  expect(arg.source.format['p#name']).toBe('number unit=sec')
  wrapper.unmount()
})

async function saveWithFormats(text: string) {
  vi.mocked(createBinding).mockClear()
  vi.mocked(createBinding).mockResolvedValue({} as Binding)
  const wrapper = await mountView()
  await openNew(wrapper, 1)
  await wrapper.find('input[name=mpp]').setValue('4a0')
  await wrapper.find('input[name=sheet]').setValue('FAKE-SHEET')
  await wrapper.find('input[name=range]').setValue('C5:E')
  await wrapper.find('textarea[name=formats]').setValue(text)
  await wrapper.find('.slot input').setValue('Hi')
  await wrapper.find('form.binding').trigger('submit')
  await flushPromises()
  return wrapper
}

test('a comma-joined Formats line gives an error and sends nothing', async () => {
  const wrapper = await saveWithFormats('p#scre=number unit=sec, p#date=date')
  expect(wrapper.find('.form-error').text()).toBe('Formats line 1: put each column on its own line')
  expect(createBinding).not.toHaveBeenCalled()
  expect(wrapper.find('textarea[name=formats]').attributes('placeholder')).toContain('p#date=date')
  expect(wrapper.text()).toContain('One per line: column=formatter')
  wrapper.unmount()
})

test('two Formats lines save as two entries', async () => {
  const wrapper = await saveWithFormats('p#scre=number unit=sec\np#date=date')
  expect(createBinding).toHaveBeenCalledOnce()
  expect(vi.mocked(createBinding).mock.calls[0][0].source.format).toEqual({ 'p#scre': 'number unit=sec', 'p#date': 'date' })
  wrapper.unmount()
})

test('Preview on a row opens the preview dialog for the binding page', async () => {
  const w = await mountView()
  const showModal = vi.fn(); HTMLDialogElement.prototype.showModal = showModal
  vi.mocked(getPreview).mockResolvedValue({ mpp: '4A0', description: '', headerTemplate: '', subpages: [{ rows: Array(25).fill(null), seconds: 8 }] })
  await w.find('button.preview').trigger('click')
  expect(getPreview).toHaveBeenCalledWith('4A0')
  expect(showModal).toHaveBeenCalled()
})
