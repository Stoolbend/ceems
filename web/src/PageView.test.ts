import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import PageView from './PageView.vue'
import PageEditor from './editor/PageEditor.vue'
import { PALETTE } from './editor/cells'
import { ApiError, deletePage, getPage, putPage, uploadTTI, type Page } from './api'
import { EDITOR, decode, encode } from './zxnet'
import { user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  getPage: vi.fn(),
  putPage: vi.fn(),
  uploadTTI: vi.fn(),
  deletePage: vi.fn(),
}))

const spaces = ' '.repeat(40)
const rows = (row1: string): (string | null)[] => [null, row1, ...Array(22).fill(null), spaces, null].slice(0, 25)
const page = (description: string, row1 = 'HELLO'): Page => ({
  mpp: '100', description, transmit: true, updatedAt: '2026-01-01T00:00:00Z', updatedBy: 'cli', etag: 'e', binding: null, fastextGroup: null, canWrite: true,
  subpages: [
    { cycleSeconds: 10, cycleMode: 'T', links: ['199', '200', '300', '400', '8ff', '8ff'], rows: rows(row1) },
    { cycleSeconds: null, cycleMode: null, links: null, rows: rows('SECOND') },
  ],
})

beforeEach(() => {
  vi.mocked(getPage).mockResolvedValue(page('before'))
  vi.mocked(putPage).mockReset()
  vi.mocked(uploadTTI).mockReset()
  vi.mocked(deletePage).mockReset()
  vi.restoreAllMocks() // the window.confirm spies
})

async function mountView() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<i/>' } }, { path: '/fastext', component: { template: '<i/>' } }, { path: '/pages/:mpp', component: PageView }] })
  router.push('/pages/100')
  await router.isReady()
  const wrapper = mount(PageView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>

async function choose(wrapper: Wrapper, file: File) {
  const input = wrapper.find('input[type=file]')
  Object.defineProperty(input.element, 'files', { value: [file] })
  await input.trigger('change')
}

const app = (w: Wrapper) => w.find('[role=application]')
const editorRow = (w: Wrapper, r: number) => w.findAll('.editor-grid .tt-row')[r]
// Clicks a cell of the editor, then types each character as a keydown.
async function typeAt(w: Wrapper, r: number, c: number, text: string) {
  await editorRow(w, r).findAll('.tt-cell')[c].trigger('click')
  for (const key of text) await app(w).trigger('keydown', { key })
}
const button = (w: Wrapper, text: string) => w.findAll('button').find((b) => b.text().includes(text))!
const preview = (w: Wrapper) => w.find('.editor-grid .tt-page').text()

test('shows metadata, a download link and the rows of the first subpage in the editor', async () => {
  const wrapper = await mountView()
  expect(wrapper.find<HTMLInputElement>('input.description').element.value).toBe('before')
  expect(wrapper.find('a[download]').attributes('href')).toBe('/api/v1/pages/100/tti')
  expect(editorRow(wrapper, 1).text()).toBe('HELLO')
  expect(wrapper.find('input[aria-label^="Row"]').exists()).toBe(false)
  expect(wrapper.find('.count').exists()).toBe(false)
})

test('the preview header comes from the stored header template, else the built-in one', async () => {
  user.value = { id: 1, username: 'conor', admin: true, headerTemplate: 'MYSYS %%# %H' }
  let wrapper = await mountView()
  expect(editorRow(wrapper, 0).text()).toMatch(/^MYSYS 100 \d\d/)
  wrapper.unmount()
  user.value = { id: 1, username: 'conor', admin: true }
  wrapper = await mountView()
  expect(editorRow(wrapper, 0).text()).toMatch(/^CeeMS\s+100\s+\d\d:\d\d:\d\d$/)
  user.value = null
})

test('the preview follows typing in the editor', async () => {
  const wrapper = await mountView()
  await typeAt(wrapper, 1, 0, 'WORLD!!')
  expect(editorRow(wrapper, 1).text()).toBe('WORLD!!')
  expect(preview(wrapper)).toContain('WORLD!!')
})

test('save sends the loaded etag, pads only changed rows and keeps untouched rows as loaded', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  await typeAt(wrapper, 2, 0, 'new')
  await button(wrapper, 'Save').trigger('click')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  expect(sent.etag).toBe('e')
  expect(sent.subpages[0].rows[1]).toBe('HELLO')
  expect(sent.subpages[0].rows[2]).toBe('new'.padEnd(40, ' '))
  expect(sent.subpages[0].rows[3]).toBeNull()
  expect(sent.subpages[1].links).toBeNull()
})

test('editing the cycle seconds number input saves the new value', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  await wrapper.find('input.cycle').setValue(20)
  await button(wrapper, 'Save').trigger('click')
  expect(vi.mocked(putPage).mock.calls[0][0].subpages[0].cycleSeconds).toBe(20)
})

test('after a save the editor shows what the server returned', async () => {
  vi.mocked(putPage).mockResolvedValue(page('after', 'FROM SERVER'))
  const wrapper = await mountView()
  await typeAt(wrapper, 1, 0, 'TYPED')
  await button(wrapper, 'Save').trigger('click')
  await flushPromises()
  expect(editorRow(wrapper, 1).text()).toBe('FROM SERVER')
  expect(preview(wrapper)).toContain('FROM SERVER')
  expect(preview(wrapper)).not.toContain('TYPED')
  expect(wrapper.find<HTMLInputElement>('input.description').element.value).toBe('after')
})

test('a palette button writes its code at the cursor', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  await typeAt(wrapper, 1, 2, '')
  await wrapper.get('.palette button[aria-label="Alpha red"]').trigger('click')
  expect(wrapper.find('p.status').text()).toMatch(/^Row 1, column 3,/)
  await button(wrapper, 'Save').trigger('click')
  expect(vi.mocked(putPage).mock.calls[0][0].subpages[0].rows[1]).toBe('HE\x01LO'.padEnd(40, ' '))
})

test('typing X at row 3, column 5 sends only that cell changed, and every other row as loaded', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  await typeAt(wrapper, 3, 5, 'X')
  await button(wrapper, 'Save').trigger('click')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  expect(sent.subpages[0].rows[3]).toBe('     X'.padEnd(40, ' '))
  const others = (rs: (string | null)[]) => rs.filter((_, r) => r !== 3)
  expect(others(sent.subpages[0].rows)).toEqual(others(loadedSub(0).rows))
  expect(sent.subpages[1]).toEqual(loadedSub(1))
})

test('saving an unchanged page sends every row as loaded', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  await typeAt(wrapper, 4, 4, '') // moving the cursor is not an edit
  await app(wrapper).trigger('keydown', { key: 'ArrowDown' })
  await button(wrapper, 'Save').trigger('click')
  expect(vi.mocked(putPage).mock.calls[0][0].subpages).toEqual(page('before').subpages)
})

test('the Grid checkbox adds the class grid to the editor page only', async () => {
  const wrapper = await mountView()
  expect(wrapper.find('.editor-grid .tt-page').classes()).not.toContain('show-grid')
  await wrapper.find('input[name=grid]').setValue(true)
  expect(wrapper.find('.editor-grid .tt-page').classes()).toContain('show-grid')
  expect(wrapper.findAll('.palette .tt-page.show-grid')).toHaveLength(0)
})

test('changing subpage shows its rows and resets the cursor', async () => {
  const wrapper = await mountView()
  await typeAt(wrapper, 5, 9, '')
  expect(wrapper.find('p.status').text()).toMatch(/^Row 5, column 9,/)
  await button(wrapper, '2').trigger('click')
  expect(editorRow(wrapper, 1).text()).toBe('SECOND')
  expect(wrapper.find('p.status').text()).toMatch(/^Row 1, column 0,/)
  // removing the shown subpage can leave the same tab index on another subpage
  await button(wrapper, 'Add subpage').trigger('click')
  await button(wrapper, '2').trigger('click')
  await typeAt(wrapper, 5, 9, '')
  await button(wrapper, 'Remove subpage').trigger('click')
  expect(editorRow(wrapper, 5).text()).toBe('')
  expect(wrapper.find('p.status').text()).toMatch(/^Row 1, column 0,/)
})

test('subpages can be added and removed', async () => {
  const wrapper = await mountView()
  expect(button(wrapper, 'Remove subpage').attributes('disabled')).toBeUndefined()
  await button(wrapper, 'Add subpage').trigger('click')
  expect(wrapper.findAll('.tabs button').length).toBe(3 + 2)
  expect(editorRow(wrapper, 5).text()).toBe('')
  await button(wrapper, 'Remove subpage').trigger('click')
  await button(wrapper, 'Remove subpage').trigger('click')
  expect(button(wrapper, 'Remove subpage').attributes('disabled')).toBeDefined()
})

test('a 412 shows the reload message and a Reload button that refetches', async () => {
  vi.mocked(putPage).mockRejectedValue(new ApiError(412, 'precondition_failed', 'page changed since it was loaded; reload and try again'))
  const wrapper = await mountView()
  await button(wrapper, 'Save').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role=alert]').text()).toContain('page changed since it was loaded; reload and try again')
  vi.mocked(getPage).mockResolvedValue(page('reloaded'))
  await button(wrapper, 'Reload').trigger('click')
  await flushPromises()
  expect(wrapper.find<HTMLInputElement>('input.description').element.value).toBe('reloaded')
  expect(wrapper.find('[role=alert]').exists()).toBe(false)
})

test('upload is disabled until a file is chosen', async () => {
  const wrapper = await mountView()
  expect(wrapper.find('form.upload button').attributes('disabled')).toBeDefined()
})

test('upload replaces the shown page with the response', async () => {
  vi.mocked(uploadTTI).mockResolvedValue(page('after'))
  const wrapper = await mountView()
  const file = new File(['PN,10000\r\n'], 'P100.tti')
  await choose(wrapper, file)
  await wrapper.find('form.upload').trigger('submit')
  await flushPromises()
  expect(uploadTTI).toHaveBeenCalledWith('100', file)
  expect(wrapper.find<HTMLInputElement>('input.description').element.value).toBe('after')
})

test('upload error message is shown and the page is kept', async () => {
  vi.mocked(uploadTTI).mockRejectedValue(new ApiError(400, 'mpp_mismatch', 'file is page 101 but the target is page 100'))
  const wrapper = await mountView()
  await choose(wrapper, new File(['x'], 'P101.tti'))
  await wrapper.find('form.upload').trigger('submit')
  await flushPromises()
  expect(wrapper.find('[role=alert]').text()).toBe('file is page 101 but the target is page 100')
  expect(wrapper.find<HTMLInputElement>('input.description').element.value).toBe('before')
})

test('delete asks first and does nothing when declined', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  const wrapper = await mountView()
  await button(wrapper, 'Delete').trigger('click')
  expect(confirm).toHaveBeenCalledWith('Delete page 100? Its last 20 versions are kept.')
  expect(deletePage).not.toHaveBeenCalled()
  expect(wrapper.vm.$route.path).toBe('/pages/100')
})

test('confirmed delete sends the etag and returns to the list', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.mocked(deletePage).mockResolvedValue(undefined)
  const wrapper = await mountView()
  await button(wrapper, 'Delete').trigger('click')
  await flushPromises()
  expect(deletePage).toHaveBeenCalledWith('100', 'e')
  expect(wrapper.vm.$route.path).toBe('/')
})

test('a refused delete shows the message, offers Reload and stays', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.mocked(deletePage).mockRejectedValue(new ApiError(412, 'precondition_failed', 'page changed'))
  const wrapper = await mountView()
  await button(wrapper, 'Delete').trigger('click')
  await flushPromises()
  expect(wrapper.find('[role=alert]').text()).toContain('page changed')
  expect(button(wrapper, 'Reload')).toBeDefined()
  expect(wrapper.vm.$route.path).toBe('/pages/100')
})

test('a page generated by a binding is read-only, the editor included', async () => {
  vi.mocked(getPage).mockResolvedValue({ ...page('gen'), binding: { id: 3, name: 'pacman' } })
  const wrapper = await mountView()
  expect(wrapper.find('[role=status]').text()).toBe('Generated by binding pacman. Edit its template or binding instead.')
  const inputs = wrapper.findAll<HTMLInputElement | HTMLSelectElement>('.editor input, .editor select')
  expect(wrapper.find('.bound').classes()).toContain('warning')
  expect(inputs).toHaveLength(14) // description, transmit, cycle, mode, the six links and the four view options
  for (const i of inputs) expect(i.element.disabled).toBe(true)
  expect(wrapper.findComponent(PageEditor).props()).toMatchObject({ showCodes: false, showSlots: false, grid: false, draw: false, disabled: true })
  expect(app(wrapper).attributes('tabindex')).toBeUndefined()
  await app(wrapper).trigger('keydown', { key: 'X' })
  expect(editorRow(wrapper, 1).text()).toBe('HELLO')
  const labels = wrapper.findAll('button').map((b) => b.text())
  expect(labels).not.toContain('Save')
  expect(labels).not.toContain('Delete')
  expect(labels).not.toContain('Upload')
  expect(wrapper.find('input[type=file]').exists()).toBe(false)
  expect(wrapper.find('a[download]').exists()).toBe(true)
})

test('an unbound page shows no banner', async () => {
  const wrapper = await mountView()
  expect(wrapper.find('[role=status]').exists()).toBe(false)
  expect(app(wrapper).attributes('tabindex')).toBe('0')
})

const zxnetLink = (w: Wrapper) => w.findAll('a').find((a) => a.text() === 'Open subpage 1 in zxnet editor')!

test('the zxnet link carries the subpage as edited, with a blank row 0', async () => {
  const wrapper = await mountView()
  await typeAt(wrapper, 1, 0, 'EDITED')
  const a = zxnetLink(wrapper)
  expect(a.attributes('target')).toBe('_blank')
  expect(a.attributes('rel')).toBe('noopener')
  const href = a.attributes('href')!
  expect(href.startsWith(EDITOR)).toBe(true)
  const back = decode(href) as { rows: string[]; pn: string | null }
  expect(back.rows[0]).toBe(spaces)
  expect(back.rows[1]).toBe('EDITED'.padEnd(40, ' '))
  expect(back.rows[23]).toBe(spaces)
  expect(back.pn).toBe('100')
  await button(wrapper, '2').trigger('click')
  expect(wrapper.findAll('a').some((x) => x.text() === 'Open subpage 2 in zxnet editor')).toBe(true)
})

test('the zxnet link is present on a generated page', async () => {
  vi.mocked(getPage).mockResolvedValue({ ...page('gen'), binding: { id: 3, name: 'pacman' } })
  expect(zxnetLink(await mountView()).exists()).toBe(true)
})

// Import from a zxnet URL (T-026).
const urlRow = (r: number) => `R${r} `.padEnd(40, '.')
const urlRows = (row0 = 'X'.repeat(40)) => [row0, ...Array.from({ length: 24 }, (_, i) => urlRow(i + 1))]
const urlOf = (rs: string[]) => encode(rs, '1A0', [])
const importInput = (w: Wrapper) => w.find<HTMLInputElement>('input[name=zxnet]')
async function doImport(w: Wrapper, text: string) {
  await importInput(w).setValue(text)
  await button(w, 'Import into subpage').trigger('click')
}
const loadedSub = (i: number) => page('before').subpages[i]

test('import then Save sends rows 1 to 24 from the URL and everything else as loaded', async () => {
  const wrapper = await mountView()
  await doImport(wrapper, urlOf(urlRows()))
  expect(editorRow(wrapper, 1).text()).toBe(urlRow(1))
  vi.mocked(putPage).mockResolvedValue(page('before'))
  await button(wrapper, 'Save').trigger('click')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  const s0 = sent.subpages[0]
  expect(s0.rows[0]).toBeNull() // loaded value, not the URL's row 0 of X
  for (let r = 1; r <= 24; r++) expect(s0.rows[r]).toBe(urlRow(r))
  expect({ ...s0, rows: [] }).toEqual({ ...loadedSub(0), rows: [] })
  expect(sent.subpages[1]).toEqual(loadedSub(1))
  expect(sent.description).toBe('before')
  expect(sent.transmit).toBe(true)
})

test('an 1120-character fragment imports row 24 as 40 spaces', async () => {
  const wrapper = await mountView()
  const frag = urlOf(urlRows().slice(0, 24))
  expect(frag.split('#')[1].split(':')[1]).toHaveLength(1120)
  await doImport(wrapper, frag.slice(frag.indexOf('#') + 1)) // bare fragment
  vi.mocked(putPage).mockResolvedValue(page('before'))
  await button(wrapper, 'Save').trigger('click')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  expect(sent.subpages[0].rows[24]).toBe(spaces)
  expect(sent.subpages[0].rows[23]).toBe(urlRow(23))
})

test('import into subpage 2 changes only subpage 2', async () => {
  const wrapper = await mountView()
  await button(wrapper, '2').trigger('click')
  expect(button(wrapper, 'Import into subpage').text()).toBe('Import into subpage 2')
  await doImport(wrapper, urlOf(urlRows()))
  vi.mocked(putPage).mockResolvedValue(page('before'))
  await button(wrapper, 'Save').trigger('click')
  const sent = vi.mocked(putPage).mock.calls[0][0]
  expect(sent.subpages[0]).toEqual(loadedSub(0))
  expect(sent.subpages[1].rows[0]).toBeNull()
  for (let r = 1; r <= 24; r++) expect(sent.subpages[1].rows[r]).toBe(urlRow(r))
})

test('import alone sends nothing, and Reload restores the loaded rows', async () => {
  const wrapper = await mountView()
  await doImport(wrapper, urlOf(urlRows()))
  expect(putPage).not.toHaveBeenCalled()
  vi.mocked(putPage).mockRejectedValue(new ApiError(412, 'precondition_failed', 'page changed since it was loaded; reload and try again'))
  await button(wrapper, 'Save').trigger('click')
  await flushPromises()
  await button(wrapper, 'Reload').trigger('click')
  await flushPromises()
  expect(editorRow(wrapper, 1).text()).toBe('HELLO')
})

test.each([
  ['wrong length', 'https://zxnet.co.uk/teletext/editor/#0:ABC:PN=100', 'page data must be 1120 or 1167 characters, got 3'],
  ['bad character', `#0:${'A'.repeat(1119)}!`, 'page data has a character outside A-Z a-z 0-9 - _'],
  ['bad shape', 'hello', 'not a zxnet editor URL (expected meta:pagedata)'],
])('a refused import (%s) shows the message, changes nothing and sends nothing', async (_n, text, msg) => {
  const wrapper = await mountView()
  await typeAt(wrapper, 2, 0, 'KEEP')
  await doImport(wrapper, text)
  expect(wrapper.find('[role=alert]').text()).toContain(msg)
  expect(editorRow(wrapper, 1).text()).toBe('HELLO')
  expect(editorRow(wrapper, 2).text()).toBe('KEEP')
  expect(putPage).not.toHaveBeenCalled()
})

test('a generated page shows no import input or button', async () => {
  vi.mocked(getPage).mockResolvedValue({ ...page('gen'), binding: { id: 3, name: 'pacman' } })
  const wrapper = await mountView()
  expect(importInput(wrapper).exists()).toBe(false)
  expect(wrapper.findAll('button').some((b) => b.text().includes('Import'))).toBe(false)
})

test('every palette button holds a sample sized to its entry', async () => {
  const wrapper = await mountView()
  const buttons = wrapper.findAll('.palette button')
  expect(buttons).toHaveLength(PALETTE.length)
  buttons.forEach((b, i) => {
    expect(b.findAll('.tt-row'), PALETTE[i].name).toHaveLength(PALETTE[i].rows)
    expect(b.find('.tt-row').findAll('.tt-cell'), PALETTE[i].name).toHaveLength(PALETTE[i].sample.length)
  })
})

test('the fastext inputs sit under Row 24, in order, and the first four are coloured', async () => {
  vi.mocked(putPage).mockResolvedValue(page('before'))
  const wrapper = await mountView()
  const rows = wrapper.find('.rows')
  expect(rows.find('.links').element).toBe(rows.element.lastElementChild)
  const labels = rows.findAll('.links label').map((l) => l.text())
  expect(labels).toEqual(['Red', 'Green', 'Yellow', 'Cyan', 'Link 5', 'Index'])
  expect(rows.findAll('.links input').map((i) => i.classes().filter((c) => c !== 'link'))).toEqual([['red'], ['green'], ['yellow'], ['cyan'], [], []])
  await rows.find('.links input').setValue('150')
  await button(wrapper, 'Save').trigger('click')
  expect(vi.mocked(putPage).mock.calls[0][0].subpages[0].links?.[0]).toBe('150')
})

test('the palette has Alpha, Mosaic and Other sections of 8, 12 and 7 buttons', async () => {
  const wrapper = await mountView()
  const sections = wrapper.findAll('.palette section')
  expect(sections.map((s) => s.find('h3').text())).toEqual(['Alpha', 'Mosaic', 'Other'])
  expect(sections.map((s) => s.findAll('button').length)).toEqual([8, 12, 7])
})

test('the Draw checkbox is off by default and reaches the editor', async () => {
  const wrapper = await mountView()
  const box = wrapper.find('input[name=draw]')
  expect((box.element as HTMLInputElement).checked).toBe(false)
  expect(wrapper.findComponent(PageEditor).props('draw')).toBe(false)
  await box.setValue(true)
  expect(wrapper.findComponent(PageEditor).props('draw')).toBe(true)
})

test('Show control codes is off by default and marks the preview only, never a palette sample', async () => {
  const wrapper = await mountView()
  const box = wrapper.find('input[name=showCodes]')
  expect((box.element as HTMLInputElement).checked).toBe(false)
  await typeAt(wrapper, 1, 0, '')
  await wrapper.get('.palette button[aria-label="Alpha red"]').trigger('click') // a control code in row 1
  expect(wrapper.findAll('.editor-grid .tt-page .ctl')).toHaveLength(0)
  await box.setValue(true)
  expect(wrapper.findAll('.editor-grid .tt-page .ctl').length).toBeGreaterThan(0)
  expect(wrapper.findAll('.palette .ctl')).toHaveLength(0)
})

test('Show template slots is off by default, marks the preview only, and sits between the codes and grid options', async () => {
  const wrapper = await mountView()
  const box = wrapper.find('input[name=showSlots]')
  expect((box.element as HTMLInputElement).checked).toBe(false)
  await typeAt(wrapper, 1, 0, '{ab}')
  expect(wrapper.findAll('.editor-grid .tt-page .slot')).toHaveLength(0)
  await box.setValue(true)
  expect(wrapper.findAll('.editor-grid .tt-page .slot')).toHaveLength(4)
  expect(wrapper.findAll('.palette .slot')).toHaveLength(0)
  const labels = wrapper.findAll('p.view label').map((l) => l.text())
  expect(labels.indexOf('Show template slots')).toBe(labels.indexOf('Show control codes') + 1)
  expect(labels.indexOf('Grid')).toBe(labels.indexOf('Show template slots') + 1)
})

test('each palette button names its Esc key in its title', async () => {
  const wrapper = await mountView()
  for (const p of PALETTE) {
    const b = wrapper.get(`.palette button[aria-label="${p.name}"]`)
    expect(b.attributes('title')).toBe(`${p.name} (Esc ${p.key})`)
    expect(b.get('kbd').text()).toBe(p.key) // the aria-hidden sample's cells also add text to b.text()
    expect(b.get('kbd').attributes('aria-hidden')).toBe('true')
  }
})

test('a failed URL paste into the editor shows its message in the error line', async () => {
  const wrapper = await mountView()
  const ev = new Event('paste', { cancelable: true, bubbles: true }) as Event & { clipboardData: unknown }
  ev.clipboardData = { getData: () => 'https://x/#0:abc' }
  app(wrapper).element.dispatchEvent(ev)
  await flushPromises()
  expect(wrapper.find('[role=alert]').text()).toBe((decode('https://x/#0:abc') as Error).message)
})

describe('links the server rules own (D-065)', () => {
  const grouped = { ...page('g'), fastextGroup: { id: 7, name: 'News', paged: true } }
  const links = (w: Wrapper) => w.findAll<HTMLInputElement>('.links input')

  beforeEach(() => {
    user.value = { id: 1, username: 'conor', admin: true }
  })
  afterEach(() => {
    user.value = null
  })

  test('a grouped page disables the four coloured inputs, keeps Link 5, and the admin gets a link to the group', async () => {
    vi.mocked(getPage).mockResolvedValue(grouped)
    const w = await mountView()
    expect(links(w).map((i) => i.element.disabled)).toEqual([true, true, true, true, false, false])
    expect(links(w)[0].attributes('title')).toBe('From fastext group News')
    expect(links(w)[4].attributes('title')).toBeUndefined()
    const line = w.find('.links .fastext')
    expect(line.attributes('role')).toBe('note')
    expect(line.text()).toBe('► This page is part of fastext group News (paged).')
    expect(line.find('a').attributes('href')).toBe('/fastext?group=7')
    expect(line.find('span').attributes('aria-hidden')).toBe('true')
    expect(w.find('.side .fastext').exists()).toBe(false)
    expect(w.find('.editor').classes()).toContain('grouped')
    await links(w)[4].setValue('150')
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(vi.mocked(putPage).mock.calls[0][0].subpages[0].links).toEqual(['199', '200', '300', '400', '150', '8ff'])
  })

  test('a non-admin sees the group name only', async () => {
    user.value = { id: 2, username: 'guest', admin: false }
    vi.mocked(getPage).mockResolvedValue({ ...grouped, fastextGroup: { id: 7, name: 'News', paged: false } })
    const w = await mountView()
    expect(w.find('.fastext').text()).toBe('► This page is part of fastext group News.')
    expect(w.find('.fastext a').exists()).toBe(false)
  })

  test('an ungrouped page has no group line and editable coloured inputs', async () => {
    const w = await mountView()
    expect(w.find('.fastext').exists()).toBe(false)
    expect(w.find('.editor').classes()).not.toContain('grouped')
    expect(links(w).map((i) => i.element.disabled)).toEqual(Array(6).fill(false))
  })

  test('with the index link on, the Index input is disabled and shows the index page', async () => {
    user.value = { id: 1, username: 'conor', admin: true, indexPage: '1ac' }
    const w = await mountView()
    expect(links(w).map((i) => i.element.disabled)).toEqual([false, false, false, false, false, true])
    expect(links(w)[5].element.value).toBe('1ac')
    expect(links(w)[5].attributes('title')).toBe('Index page from system settings')
  })
})

describe('row 24 written by the group', () => {
  const row24 = '\x01Index    \x02 [ Prev  \x03  Next ] \x06Art index'
  const withRow24 = (g: boolean): Page => {
    const p = page('g')
    for (const s of p.subpages) s.rows[24] = row24
    return g ? { ...p, fastextGroup: { id: 7, name: 'News', paged: true } } : p
  }

  test('a grouped page says so, locks the editor at row 23, and sends the server row 24 on every subpage', async () => {
    vi.mocked(getPage).mockResolvedValue(withRow24(true))
    vi.mocked(putPage).mockResolvedValue(withRow24(true))
    const w = await mountView()
    expect(w.find('.fastext').text()).toContain('This page is part of fastext group News')
    expect(w.findComponent(PageEditor).props('lastRow')).toBe(23)
    // an update from the editor that changed row 24 is not kept
    const changed = (w.findComponent(PageEditor).props('rows') as string[]).map((r, i) => (i === 24 ? 'X'.repeat(40) : r))
    w.findComponent(PageEditor).vm.$emit('update:rows', changed)
    await button(w, 'Save').trigger('click')
    await flushPromises()
    expect(vi.mocked(putPage).mock.calls[0][0].subpages.map((s) => s.rows[24])).toEqual([row24, row24])
  })

  test('an ungrouped page can edit row 24', async () => {
    vi.mocked(getPage).mockResolvedValue(withRow24(false))
    const w = await mountView()
    expect(w.find('.fastext').exists()).toBe(false)
    expect(w.findComponent(PageEditor).props('lastRow')).toBe(24)
  })
})

test('a page the caller cannot write shows a read-only line and disables every edit control', async () => {
  vi.mocked(getPage).mockResolvedValue({ ...page('ro'), canWrite: false })
  const wrapper = await mountView()
  expect(wrapper.find('[role=status]').text()).toBe('Read only: you can view this page but not change it.')
  expect(wrapper.find('.readonly').classes()).toContain('warning')
  expect(button(wrapper, 'Save').element.disabled).toBe(true)
  expect(button(wrapper, 'Delete').element.disabled).toBe(true)
  expect(wrapper.findComponent(PageEditor).props('disabled')).toBe(true)
  const inputs = wrapper.findAll<HTMLInputElement | HTMLSelectElement>('.editor input, .editor select')
  expect(inputs.length).toBeGreaterThan(14) // the 14 as for a bound page, plus the zxnet field and the file input
  for (const i of inputs) expect(i.element.disabled).toBe(true)
  expect(button(wrapper, 'Upload').element.disabled).toBe(true)
  expect(button(wrapper, 'Import').element.disabled).toBe(true)
  expect(wrapper.find('a[download]').exists()).toBe(true)
  await app(wrapper).trigger('keydown', { key: 'X' })
  expect(editorRow(wrapper, 1).text()).toBe('HELLO')
})

test('a writable page shows no read-only line and enables Save and Delete', async () => {
  const wrapper = await mountView()
  expect(wrapper.find('[role=status]').exists()).toBe(false)
  expect(button(wrapper, 'Save').element.disabled).toBe(false)
  expect(button(wrapper, 'Delete').element.disabled).toBe(false)
})

test('a page document without canWrite (an older server) shows no read-only line and is not locked', async () => {
  const { canWrite: _omit, ...older } = page('old')
  vi.mocked(getPage).mockResolvedValue(older as ReturnType<typeof page>)
  const wrapper = await mountView()
  expect(wrapper.find('.readonly').exists()).toBe(false)
  expect(button(wrapper, 'Save').element.disabled).toBe(false)
  expect(wrapper.findComponent(PageEditor).props('disabled')).toBe(false)
})
