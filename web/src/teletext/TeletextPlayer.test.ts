import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import TeletextPlayer from './TeletextPlayer.vue'
import TeletextPage from './TeletextPage.vue'
import type { Preview } from '../api'

beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 0, 2, 3, 4, 5)) })
afterEach(() => { vi.useRealTimers() })

const sub = (text: string, seconds: number) => ({ rows: [null, text, ...Array(23).fill(null)], seconds })
const make = (subs: Preview['subpages'], headerTemplate = ''): Preview => ({ mpp: '1A0', description: 'x', headerTemplate, subpages: subs })
const rows = (w: ReturnType<typeof mount>) => w.findComponent(TeletextPage).props('rows') as string[]

test('rotates subpages on their own seconds and wraps', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('one', 4), sub('two', 2)]) } })
  expect(rows(w)[1]).toBe('one')
  await vi.advanceTimersByTimeAsync(3999)
  expect(rows(w)[1]).toBe('one')
  await vi.advanceTimersByTimeAsync(1)
  expect(rows(w)[1]).toBe('two')
  await vi.advanceTimersByTimeAsync(2000)
  expect(rows(w)[1]).toBe('one')
})

test('a single subpage never changes and sets no rotation timer', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('only', 4)]) } })
  expect(vi.getTimerCount()).toBe(1) // the clock only
  await vi.advanceTimersByTimeAsync(60000)
  expect(rows(w)[1]).toBe('only')
})

test('the header shows the page number and the clock moves each second', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('a', 4)]) } })
  expect(rows(w)[0]).toContain('1A0')
  expect(rows(w)[0]).toContain('03:04:05')
  await vi.advanceTimersByTimeAsync(1000)
  expect(rows(w)[0]).toContain('03:04:06')
})

test('an empty template uses the default header and a set one uses vbitHeader', () => {
  const dflt = mount(TeletextPlayer, { props: { preview: make([sub('a', 4)]) } })
  expect(rows(dflt)[0]).toBe('        CeeMS        1A0        03:04:05')
  const tpl = mount(TeletextPlayer, { props: { preview: make([sub('a', 4)], 'P%%# %H:%M:%S'.padEnd(32)) } })
  expect(rows(tpl)[0]).toBe('        P1A0 03:04:05'.padEnd(40))
})

test('empty subpages draw the header over a blank page and start no rotation', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([]) } })
  expect(rows(w)).toHaveLength(25)
  expect(rows(w)[0]).toContain('1A0')
  expect(rows(w).slice(1).every((r) => r === '')).toBe(true)
  expect(vi.getTimerCount()).toBe(1) // the clock only
})

test('seconds below 1 count as 1', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('one', 0), sub('two', 0)]) } })
  await vi.advanceTimersByTimeAsync(999)
  expect(rows(w)[1]).toBe('one')
  await vi.advanceTimersByTimeAsync(1)
  expect(rows(w)[1]).toBe('two')
})

test('a new preview restarts at subpage 0', async () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('one', 1), sub('two', 1)]) } })
  await vi.advanceTimersByTimeAsync(1000)
  expect(rows(w)[1]).toBe('two')
  await w.setProps({ preview: make([sub('x', 1), sub('y', 1)]) })
  expect(rows(w)[1]).toBe('x')
})

test('unmounting leaves no timer running', () => {
  const w = mount(TeletextPlayer, { props: { preview: make([sub('one', 1), sub('two', 1)]) } })
  expect(vi.getTimerCount()).toBeGreaterThan(0)
  w.unmount()
  expect(vi.getTimerCount()).toBe(0)
})
