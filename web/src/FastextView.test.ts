import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { beforeEach, expect, test, vi } from 'vitest'
import FastextView from './FastextView.vue'
import TeletextPage from './teletext/TeletextPage.vue'
import {
  ApiError, createFastextGroup, deleteFastextGroup, getFastextGroups, setFastextGroupPages, updateFastextGroup, type FastextGroup,
} from './api'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  getFastextGroups: vi.fn(),
  createFastextGroup: vi.fn(),
  updateFastextGroup: vi.fn(),
  deleteFastextGroup: vi.fn(),
  setFastextGroupPages: vi.fn(),
}))

const news: FastextGroup = {
  id: 7, name: 'News', paged: true, links: ['8ff', '8ff', '300', '400'], prevSlot: 0, nextSlot: 1, default: true, texts: ['', '', '', ''],
  pages: [{ mpp: '100', description: 'one' }, { mpp: '101', description: 'two' }, { mpp: '102', description: 'three' }],
}
const sport: FastextGroup = { id: 8, name: 'Sport', paged: false, links: ['200', '201', '202', '203'], prevSlot: null, nextSlot: null, default: false, texts: ['', '', '', ''], pages: [] }

beforeEach(() => {
  vi.mocked(getFastextGroups).mockReset().mockResolvedValue([news, sport])
  vi.mocked(createFastextGroup).mockReset().mockResolvedValue(news)
  vi.mocked(updateFastextGroup).mockReset().mockResolvedValue(news)
  vi.mocked(deleteFastextGroup).mockReset()
  vi.mocked(setFastextGroupPages).mockReset().mockResolvedValue(news)
  vi.restoreAllMocks()
})

async function mountView(path = '/fastext') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/fastext', component: FastextView }] })
  router.push(path)
  await router.isReady()
  const wrapper = mount(FastextView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
type Wrapper = Awaited<ReturnType<typeof mountView>>

const button = (w: Wrapper, text: string) => w.findAll('button').find((b) => b.text() === text)!
const link = (w: Wrapper, k: number) => w.find<HTMLInputElement>(`input[name=link${k}]`)
const pressed = (w: Wrapper, text: string) => w.findAll('.slots button').filter((b) => b.text() === text).map((b) => b.attributes('aria-pressed') === 'true')
const order = (w: Wrapper) => w.findAll('.members .mpp').map((m) => m.text())

test('the list shows name, type, default and member count', async () => {
  const w = await mountView()
  expect(w.findAll('tbody tr').map((r) => r.findAll('td').slice(0, 4).map((c) => c.text()))).toEqual([
    ['News', 'Paged', 'Default', '3'],
    ['Sport', 'Static', '', '0'],
  ])
})

test('a static group shows no Prev or Next toggles; a paged one shows one of each, never on the same key', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  expect(w.find('input[name=name]').attributes('maxlength')).toBe('40')
  expect(w.findAll('.slots button')).toHaveLength(0)
  await w.findAll('input[name=kind]')[1].setValue(true)
  expect(pressed(w, 'Prev')).toEqual([true, false, false, false])
  expect(pressed(w, 'Next')).toEqual([false, true, false, false])
  expect([link(w, 0), link(w, 1), link(w, 2), link(w, 3)].map((i) => i.element.disabled)).toEqual([true, true, false, false])
  expect(link(w, 0).attributes('placeholder')).toBe('prev')
  expect(link(w, 1).attributes('placeholder')).toBe('next')
  // moving Prev to the key Next holds swaps them
  await w.findAll('.slots')[1].findAll('button')[0].trigger('click')
  expect(pressed(w, 'Prev')).toEqual([false, true, false, false])
  expect(pressed(w, 'Next')).toEqual([true, false, false, false])
  // moving Next to a free key frees the old one
  await w.findAll('.slots')[3].findAll('button')[1].trigger('click')
  expect(pressed(w, 'Next')).toEqual([false, false, false, true])
  expect([link(w, 0), link(w, 1), link(w, 2), link(w, 3)].map((i) => i.element.disabled)).toEqual([false, true, false, true])
  await w.findAll('input[name=kind]')[0].setValue(true)
  expect(w.findAll('.slots button')).toHaveLength(0)
})

test('saving a paged group sends the slots and 8ff in them', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  await w.find('input[name=name]').setValue('News')
  await w.findAll('input[name=kind]')[1].setValue(true)
  await link(w, 2).setValue('300')
  await link(w, 3).setValue('400')
  await w.find('input[name=default]').setValue(true)
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(createFastextGroup).toHaveBeenCalledWith({
    name: 'News', paged: true, links: ['8ff', '8ff', '300', '400'], prevSlot: 0, nextSlot: 1, default: true, texts: ['', '', '', ''],
  })
  // the saved group's form and members stay open
  expect(w.text()).toContain('Members')
})

test('saving a static group sends null slots', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  await w.find('input[name=name]').setValue('Sport')
  for (const [k, v] of ['200', '201', '202', '203'].entries()) await link(w, k).setValue(v)
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(createFastextGroup).toHaveBeenCalledWith({
    name: 'Sport', paged: false, links: ['200', '201', '202', '203'], prevSlot: null, nextSlot: null, default: false, texts: ['', '', '', ''],
  })
})

test('editing a paged group keeps its slots and shows no 8ff in the disabled inputs; a 409 shows', async () => {
  vi.mocked(updateFastextGroup).mockRejectedValue(new ApiError(409, 'conflict', 'a group with that name exists'))
  const w = await mountView()
  await w.findAll('tbody tr')[0].find('button').trigger('click')
  expect(link(w, 0).element.value).toBe('')
  expect(link(w, 2).element.value).toBe('300')
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(updateFastextGroup).toHaveBeenCalledWith(7, { name: 'News', paged: true, links: ['8ff', '8ff', '300', '400'], prevSlot: 0, nextSlot: 1, default: true, texts: ['', '', '', ''] })
  expect(w.find('[role=alert]').text()).toBe('a group with that name exists')
})

test('a blank link box on a static group saves as 8ff and a stored 8ff opens blank', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  await w.find('input[name=name]').setValue('Mixed')
  await link(w, 0).setValue('100')
  await link(w, 2).setValue('300')
  await link(w, 3).setValue('400')
  expect(link(w, 1).attributes('required')).toBeUndefined()
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(createFastextGroup).toHaveBeenCalledWith({
    name: 'Mixed', paged: false, links: ['100', '8ff', '300', '400'], prevSlot: null, nextSlot: null, default: false, texts: ['', '', '', ''],
  })
  const stored: FastextGroup = { ...sport, id: 9, name: 'Mixed', links: ['100', '8FF', '300', '400'] }
  vi.mocked(getFastextGroups).mockResolvedValue([stored])
  const w2 = await mountView()
  await w2.findAll('tbody tr')[0].find('button').trigger('click')
  expect(link(w2, 1).element.value).toBe('')
  await w2.find('form').trigger('submit')
  await flushPromises()
  expect(updateFastextGroup).toHaveBeenCalledWith(9, expect.objectContaining({ links: ['100', '8ff', '300', '400'] }))
})

test('members: Add, Up, Down and Remove change the order, and Save members sends the list', async () => {
  const w = await mountView()
  await w.findAll('tbody tr')[0].find('button').trigger('click')
  expect(w.text()).toContain('Previous and Next follow this order. The first page has no Previous link and the last has no Next link.')
  expect(order(w)).toEqual(['100', '101', '102'])
  await w.findAll('.members li')[2].findAll('button')[0].trigger('click') // 102 up
  expect(order(w)).toEqual(['100', '102', '101'])
  await w.findAll('.members li')[0].findAll('button')[1].trigger('click') // 100 down
  expect(order(w)).toEqual(['102', '100', '101'])
  await w.findAll('.members li')[1].findAll('button')[2].trigger('click') // remove 100
  expect(order(w)).toEqual(['102', '101'])
  await w.find('input[name=addMpp]').setValue('1ab')
  await button(w, 'Add').trigger('click')
  expect(order(w)).toEqual(['102', '101', '1AB'])
  await button(w, 'Save members').trigger('click')
  await flushPromises()
  expect(setFastextGroupPages).toHaveBeenCalledWith(7, ['102', '101', '1AB'])
})

test('Add refuses a page that is not three hex digits', async () => {
  const w = await mountView()
  await w.findAll('tbody tr')[0].find('button').trigger('click')
  await w.find('input[name=addMpp]').setValue('9zz')
  expect(button(w, 'Add').element.disabled).toBe(true)
})

test('a 409 in_group from Save members shows in the alert', async () => {
  vi.mocked(setFastextGroupPages).mockRejectedValue(new ApiError(409, 'in_group', 'page 105 is already in group Sport'))
  const w = await mountView()
  await w.findAll('tbody tr')[0].find('button').trigger('click')
  await button(w, 'Save members').trigger('click')
  await flushPromises()
  expect(w.find('[role=alert]').text()).toBe('page 105 is already in group Sport')
})

test('/fastext?group=<id> opens that group, with its members', async () => {
  const w = await mountView('/fastext?group=8')
  expect(w.find<HTMLInputElement>('input[name=name]').element.value).toBe('Sport')
  expect(w.findAll('.slots button')).toHaveLength(0)
  expect(w.text()).toContain('Members')
  const none = await mountView('/fastext?group=99')
  expect(none.find('form').exists()).toBe(false)
})

test('Delete confirms, says the pages keep their links, and reloads', async () => {
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
  const w = await mountView()
  await w.findAll('tbody tr')[1].findAll('button')[1].trigger('click')
  await flushPromises()
  expect(confirm).toHaveBeenCalledWith('Delete group Sport? Its pages keep their current links and lose row 24.')
  expect(deleteFastextGroup).toHaveBeenCalledWith(8)
  expect(getFastextGroups).toHaveBeenCalledTimes(2)
})

test('Delete does nothing when declined', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false)
  const w = await mountView()
  await w.findAll('tbody tr')[1].findAll('button')[1].trigger('click')
  expect(deleteFastextGroup).not.toHaveBeenCalled()
})

const box = (w: Wrapper, name: string) => w.find<HTMLInputElement>(`input[aria-label="${name} text"]`)

test('the four text boxes load from the group, a Prev or Next slot is disabled, and save sends them', async () => {
  vi.mocked(getFastextGroups).mockResolvedValue([{ ...news, texts: ['Index', '', '', 'Art\u0001'] }, sport])
  const w = await mountView()
  await w.findAll('tbody tr')[0].findAll('button')[0].trigger('click')
  const boxes = w.findAll<HTMLInputElement>('input[aria-label$=" text"]')
  expect(boxes.map((b) => b.element.value)).toEqual(['Index', '', '', 'Art␁'])
  expect(boxes.map((b) => b.element.disabled)).toEqual([true, true, false, false]) // keys 0 and 1 are Prev and Next
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(updateFastextGroup).toHaveBeenCalledWith(7, expect.objectContaining({ texts: ['Index', '', '', 'Art\u0001'] }))
})

test('the preview shows the middle-member row of the D-066 example', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  await w.findAll('input[name=kind]')[1].setValue(true)
  // Prev is Red and Next Green by default; move Prev to Green and Next to Yellow
  await w.findAll('.slots')[1].findAll('button')[0].trigger('click')
  await w.findAll('.slots')[2].findAll('button')[1].trigger('click')
  await box(w, 'Red').setValue('Index')
  await box(w, 'Cyan').setValue('Art index')
  expect(w.findComponent(TeletextPage).props('rows')).toEqual(['Index     [ Prev    Next ] Art index'])
})

test('a new group sends four empty texts, and an invalid character disables Save', async () => {
  const w = await mountView()
  await button(w, 'New').trigger('click')
  await w.find('input[name=name]').setValue('Sport')
  for (const [k, v] of ['200', '201', '202', '203'].entries()) await link(w, k).setValue(v)
  await box(w, 'Red').setValue('é')
  expect(button(w, 'Save').attributes('disabled')).toBeDefined()
  await box(w, 'Red').setValue('')
  expect(button(w, 'Save').attributes('disabled')).toBeUndefined()
  await w.find('form').trigger('submit')
  await flushPromises()
  expect(createFastextGroup).toHaveBeenCalledWith(expect.objectContaining({ texts: ['', '', '', ''] }))
})
