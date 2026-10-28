import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, expect, test, vi } from 'vitest'
import SettingsView from './SettingsView.vue'
import { ApiError, getSettings, putSettings, type Settings } from './api'
import { toEditable } from './editor/cells'
import { systemName, user } from './session'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  getSettings: vi.fn(),
  putSettings: vi.fn(),
}))

// The stored default: 32 cells with the cyan control cell before the clock.
const DEFAULT_HEADER = 'CeeMS   %%# %%a %e %%b \x03%H:%M:%S'
const defaults: Settings = {
  systemName: 'CeeMS', pagesDir: '/srv/pages', vbitConf: true, headerTemplate: DEFAULT_HEADER, initialTeletextPage: '100',
  rowAdaptiveMode: true, linesPerField: 16, magazinePriority: '9,1,3,3,3,9,9,9', statusDisplay: 'CeeMS',
  indexPageEnabled: false, indexPage: '100',
  networkIdentificationCode: '0000', countryNetworkIdentification: '0000', datacastLines: 0, magazineSerial: false,
}

beforeEach(() => {
  vi.mocked(getSettings).mockReset().mockResolvedValue(defaults)
  vi.mocked(putSettings).mockReset()
  user.value = { id: 1, username: 'conor', admin: true, headerTemplate: DEFAULT_HEADER }
})

async function mountView() {
  const wrapper = mount(SettingsView, { attachTo: document.body })
  await flushPromises()
  return wrapper
}
const input = (w: Awaited<ReturnType<typeof mountView>>, name: string) => w.find<HTMLInputElement>(`input[name=${name}]`)

test('the vbit fieldset is not in the DOM with the option off, and comes back with it on', async () => {
  const w = await mountView()
  expect(w.find('fieldset fieldset').exists()).toBe(true)
  await input(w, 'vbitConf').setValue(false)
  expect(w.find('fieldset fieldset').exists()).toBe(false)
  expect(input(w, 'headerTemplate').exists()).toBe(false)
  expect(w.text()).toContain('CeeMS will not write or delete vbit.conf; the last file written stays until you remove it.')
  await input(w, 'vbitConf').setValue(true)
  expect(input(w, 'headerTemplate').exists()).toBe(true)
})

test('the pages directory is read-only text; the header input is capped at 32 and counts', async () => {
  const w = await mountView()
  expect(input(w, 'pagesDir').element.disabled).toBe(true)
  expect(input(w, 'pagesDir').element.value).toBe('/srv/pages')
  expect(w.text()).toContain('Set in the config file (pagesDir). Restart CeeMS to change it.')
  expect(input(w, 'headerTemplate').attributes('maxlength')).toBe('32')
  expect(w.find('.count').text()).toBe('32/32')
  // The palette refuses to insert into a full field, as the input itself refuses a 33rd character.
  const before = input(w, 'headerTemplate').element.value
  await w.find('.palette button').trigger('click')
  expect(input(w, 'headerTemplate').element.value).toBe(before)
  await input(w, 'headerTemplate').setValue(before.slice(0, 10))
  expect(w.find('.count').text()).toBe('10/32')
})

test('the palette inserts a control code at the caret and the counter follows', async () => {
  const w = await mountView()
  const el = input(w, 'headerTemplate')
  await el.setValue('AB')
  el.element.setSelectionRange(1, 1)
  await w.find('.palette button').trigger('click')
  expect(el.element.value).toHaveLength(3)
  expect(el.element.value[0]).toBe('A')
  expect(el.element.value[2]).toBe('B')
  expect(w.find('.count').text()).toBe('3/32')
})

test('the default header, control cell included, is saved unchanged: the same 32 cells', async () => {
  vi.mocked(putSettings).mockResolvedValue(defaults)
  const w = await mountView()
  expect(input(w, 'headerTemplate').element.value).toBe(toEditable(DEFAULT_HEADER))
  await w.find('form').trigger('submit')
  await flushPromises()
  const sent = vi.mocked(putSettings).mock.calls[0][0]
  expect(sent.headerTemplate).toBe(DEFAULT_HEADER)
  expect(sent.headerTemplate).toHaveLength(32)
})

test('saving sends the full document, joins the magazines, and re-renders from the response', async () => {
  vi.mocked(putSettings).mockResolvedValue({ ...defaults, systemName: 'NEWNAME', statusDisplay: 'FROMSERVER' })
  const w = await mountView()
  expect(w.findAll<HTMLInputElement>('.mags input').map((i) => i.element.value)).toEqual(['9', '1', '3', '3', '3', '9', '9', '9'])
  await input(w, 'systemName').setValue('NEWNAME')
  await input(w, 'mag8').setValue('2')
  await input(w, 'mag7').setValue('4')
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(putSettings).toHaveBeenCalledWith({ ...defaults, systemName: 'NEWNAME', magazinePriority: '2,1,3,3,3,9,9,4' })
  expect(input(w, 'statusDisplay').element.value).toBe('FROMSERVER')
  expect(systemName.value).toBe('NEWNAME')
})

test('the index link settings are sent on every save, and a save updates the user', async () => {
  vi.mocked(putSettings).mockImplementation(async (s) => s)
  const w = await mountView()
  expect(input(w, 'indexPageEnabled').element.checked).toBe(false)
  expect(input(w, 'indexPage').attributes('pattern')).toBe('[1-8][0-9A-Fa-f]{2}')
  expect(input(w, 'indexPage').attributes('required')).toBeDefined()
  expect(w.text()).toContain("When on, every page's index link (the sixth) is this page. Turning it on or changing the page rewrites every page.")
  expect(w.find('fieldset fieldset').element.contains(input(w, 'indexPage').element)).toBe(false)
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(vi.mocked(putSettings).mock.calls[0][0]).toMatchObject({ indexPageEnabled: false, indexPage: '100' })
  expect(user.value?.indexPage).toBeNull()
  await input(w, 'indexPageEnabled').setValue(true)
  await input(w, 'indexPage').setValue('1AC')
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(vi.mocked(putSettings).mock.calls[1][0]).toMatchObject({ indexPageEnabled: true, indexPage: '1AC' })
  expect(user.value?.indexPage).toBe('1AC')
})

test('the new vbit2 keys load from and save to the payload; the wiki link and the 16 line cap are set', async () => {
  vi.mocked(getSettings).mockResolvedValue({ ...defaults, networkIdentificationCode: 'ABCD', datacastLines: 2, magazineSerial: true })
  vi.mocked(putSettings).mockImplementation(async (s) => s)
  const w = await mountView()
  expect(input(w, 'networkIdentificationCode').element.value).toBe('ABCD')
  expect(input(w, 'countryNetworkIdentification').element.value).toBe('0000')
  expect(input(w, 'datacastLines').element.value).toBe('2')
  expect(input(w, 'magazineSerial').element.checked).toBe(true)
  expect(input(w, 'networkIdentificationCode').attributes('maxlength')).toBe('4')
  expect(input(w, 'linesPerField').attributes('max')).toBe('16')
  expect(input(w, 'datacastLines').attributes('max')).toBe('16')
  await input(w, 'linesPerField').setValue(8)
  expect(input(w, 'datacastLines').attributes('max')).toBe('8')
  const a = w.find('a[href="https://github.com/peterkvt80/vbit2/wiki/Manual-configuration"]')
  expect(a.text()).toBe('vbit2 settings reference')
  expect(a.attributes('target')).toBe('_blank')
  expect(a.attributes('rel')).toBe('noopener')
  await input(w, 'countryNetworkIdentification').setValue('1f2e')
  await input(w, 'datacastLines').setValue(3)
  await input(w, 'magazineSerial').setValue(false)
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(vi.mocked(putSettings).mock.calls[0][0]).toMatchObject({
    networkIdentificationCode: 'ABCD', countryNetworkIdentification: '1f2e', datacastLines: 3, magazineSerial: false, linesPerField: 8,
  })
})

test('the status display input refuses spaces and semicolons by pattern', async () => {
  const w = await mountView()
  const re = new RegExp('^(?:' + input(w, 'statusDisplay').attributes('pattern') + ')$')
  expect(input(w, 'statusDisplay').attributes('pattern')).toBe('[!-:<-~]*')
  expect(re.test('LIVE-1')).toBe(true)
  expect(re.test('A B')).toBe(false)
  expect(re.test('A;B')).toBe(false)
})

test('an unchanged form joins the magazines to the stored list', async () => {
  vi.mocked(putSettings).mockResolvedValue(defaults)
  const w = await mountView()
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(vi.mocked(putSettings).mock.calls[0][0].magazinePriority).toBe('9,1,3,3,3,9,9,9')
})

test('a 400 message is shown in the alert and the form keeps what was typed', async () => {
  vi.mocked(putSettings).mockRejectedValue(new ApiError(400, 'bad_request', 'initialTeletextPage: must look like 100'))
  const w = await mountView()
  await input(w, 'systemName').setValue('KEPT')
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(w.find('[role=alert]').text()).toBe('initialTeletextPage: must look like 100')
  expect(input(w, 'systemName').element.value).toBe('KEPT')
})
