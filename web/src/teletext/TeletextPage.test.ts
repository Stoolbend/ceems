import { mount } from '@vue/test-utils'
import { describe, expect, test } from 'vitest'
import TeletextPage from './TeletextPage.vue'
import { CONTROL_MARKS, MOSAIC_MARK } from './render'
import { testPages } from './testpages'

test('renders 25 rows of 40 cells with attribute classes', () => {
  const w = mount(TeletextPage, { props: { rows: testPages['held-graphics'] } })
  expect(w.findAll('.tt-row')).toHaveLength(25)
  expect(w.findAll('.tt-cell')).toHaveLength(1000)
  const cells = w.findAll('.tt-row')[1].findAll('.tt-cell')
  expect(cells[1].classes()).toEqual(expect.arrayContaining(['fg-7', 'bg-0', 'mosaic']))
  expect(cells[1].findAll('i')).toHaveLength(6)
  expect(cells[1].findAll('i.on')).toHaveLength(6) // 0x7F lights every sextant
  expect(cells[6].findAll('i.on')).toHaveLength(5) // 0x3F: bit 6 clear
})

test('double height, conceal, flash and separated classes', () => {
  const dh = mount(TeletextPage, { props: { rows: testPages['double-height'] } }).findAll('.tt-row')
  expect(dh[5].findAll('.tt-cell')[4].classes()).toContain('dh-top')
  expect(dh[5].findAll('.tt-cell')[4].classes()).toContain('bg-4')
  expect(dh[6].findAll('.tt-cell')[4].classes()).toContain('dh-bottom')
  expect(mount(TeletextPage, { props: { rows: testPages.conceal } }).findAll('.tt-row')[1].findAll('.tt-cell')[1].classes()).toContain('conceal')
  expect(mount(TeletextPage, { props: { rows: testPages.flash } }).findAll('.tt-row')[1].findAll('.tt-cell')[1].classes()).toContain('flash')
  expect(mount(TeletextPage, { props: { rows: testPages['contiguous-separated'] } }).findAll('.tt-row')[1].findAll('.tt-cell')[4].classes()).toContain('sep')
})

test('aspect defaults to 4:3 and 16:9 on request, same grid', () => {
  const rows = testPages['held-graphics']
  expect(mount(TeletextPage, { props: { rows } }).classes()).toContain('aspect-4-3')
  const w = mount(TeletextPage, { props: { rows, aspect: '16:9' } })
  expect(w.classes()).toContain('aspect-16-9')
  expect(w.findAll('.tt-row')).toHaveLength(25)
  expect(w.findAll('.tt-cell')).toHaveLength(1000)
})

test('rowCount and colCount crop from row 0, column 0 and set the grid size', () => {
  const w = mount(TeletextPage, { props: { rows: testPages['held-graphics'], rowCount: 2, colCount: 5 } })
  expect(w.findAll('.tt-row')).toHaveLength(2)
  expect(w.findAll('.tt-row')[0].findAll('.tt-cell')).toHaveLength(5)
  expect(w.attributes('style')).toContain('--rows: 2')
  expect(w.attributes('style')).toContain('--cols: 5')
})

test('showCodes marks control cells with a descriptive mark, and only when asked', () => {
  const rows = ['', '\x01AB\x11\x7f']
  const on = mount(TeletextPage, { props: { rows, showCodes: true } }).findAll('.tt-row')[1].findAll('.tt-cell')
  expect(on.map((c, i) => [i, c.classes().includes('ctl')]).filter(([, m]) => m).map(([i]) => i)).toEqual([0, 3])
  const mark = (c: (typeof on)[number]) => [c.find('.big').exists() && !c.find('i.big').exists() ? c.find('span.big').text() : 'mosaic', c.find('span.sub').text()]
  expect([mark(on[0]), mark(on[3])]).toEqual([['A', 'R'], ['mosaic', 'R']])
  expect(on[3].find('i.mo.big').exists()).toBe(true)
  const off = mount(TeletextPage, { props: { rows } })
  expect(off.findAll('.ctl')).toHaveLength(0)
  expect(off.findAll('.code')).toHaveLength(0)
  expect(off.findAll('.tt-row')).toHaveLength(25)
  expect(off.findAll('.tt-cell')).toHaveLength(1000)
})

test('showCodes does not mark the bottom half of a double-height row', () => {
  const r = mount(TeletextPage, { props: { rows: ['', '\x0dA', '\x01X'], showCodes: true } }).findAll('.tt-row')
  expect(r[1].findAll('.tt-cell')[0].classes()).toContain('ctl')
  expect(r[2].findAll('.tt-cell')[0].classes()).not.toContain('ctl')
})

test('grid adds the class show-grid to .tt-page only when set, and the defaults are unchanged', () => {
  const rows = testPages['held-graphics']
  const plain = mount(TeletextPage, { props: { rows } })
  expect(plain.classes()).not.toContain('show-grid')
  expect(plain.findAll('.tt-row')).toHaveLength(25)
  expect(plain.findAll('.tt-cell')).toHaveLength(1000)
  expect(mount(TeletextPage, { props: { rows, grid: true } }).classes()).toContain('show-grid')
})

// Pico.css owns the bare class grid (a column layout at 768px and up), so the page must never carry it.
test('no prop combination puts the bare class grid on .tt-page', () => {
  const rows = testPages['held-graphics']
  for (const grid of [false, true])
    for (const showCodes of [false, true])
      for (const showSlots of [false, true])
        for (const aspect of ['4:3', '16:9'] as const) expect(mount(TeletextPage, { props: { rows, grid, showCodes, showSlots, aspect } }).classes()).not.toContain('grid')
})

describe('showSlots', () => {
  const cellsOf = (rows: string[], r: number, props = {}) => mount(TeletextPage, { props: { rows, ...props } }).findAll('.tt-row')[r].findAll('.tt-cell')
  const rowsOf = (over: Record<number, string>) => Array.from({ length: 25 }, (_, r) => over[r] ?? '')
  const has = (c: { classes: () => string[] }, k: string) => c.classes().includes(k)

  test('off: no slot class, the brace still reads as a quarter, and the page is 25x40', () => {
    const w = mount(TeletextPage, { props: { rows: rowsOf({ 1: '{ab}' }) } })
    expect(w.findAll('.slot')).toHaveLength(0)
    expect(w.findAll('.tt-cell')).toHaveLength(1000)
    expect(w.findAll('.tt-row')[1].findAll('.tt-cell')[0].find('.g').text()).toBe('¼')
  })

  test('on: the whole token is marked and drawn as typed', () => {
    const c = cellsOf(rowsOf({ 1: '{^p_1.}x' }), 1, { showSlots: true })
    for (let i = 0; i <= 6; i++) expect(has(c[i], 'slot'), `cell ${i}`).toBe(true)
    expect(has(c[7], 'slot')).toBe(false)
    expect([0, 1, 3, 6].map((i) => c[i].find('.g').text())).toEqual(['{', '^', '_', '}'])
  })

  test('on: row 0 is never marked', () => {
    expect(mount(TeletextPage, { props: { rows: rowsOf({ 0: '{head}' }), showSlots: true } }).findAll('.slot')).toHaveLength(0)
  })

  test('on: a double-height token is marked on both halves, and the hidden source row is not', () => {
    const rows = rowsOf({ 1: '\x0d{ab}', 2: '{cd}' })
    const top = cellsOf(rows, 1, { showSlots: true })
    const bottom = cellsOf(rows, 2, { showSlots: true })
    for (let i = 1; i <= 4; i++) {
      expect(top[i].classes()).toEqual(expect.arrayContaining(['slot', 'dh-top']))
      expect(bottom[i].classes()).toEqual(expect.arrayContaining(['slot', 'dh-bottom']))
    }
    expect(has(bottom[0], 'slot')).toBe(false)
  })

  test('on: a mosaic run that reads as a token keeps its sextants', () => {
    const c = cellsOf(rowsOf({ 1: '\x11{ab}' }), 1, { showSlots: true })
    for (let i = 1; i <= 4; i++) {
      expect(c[i].classes()).toEqual(expect.arrayContaining(['slot', 'mosaic']))
      expect(c[i].findAll('i')).toHaveLength(6)
      expect(c[i].find('.g').exists()).toBe(false)
    }
  })

  test('with showCodes too, the control cell is ctl only and the token cells are slot only', () => {
    const c = cellsOf(rowsOf({ 1: '\x01{ab}' }), 1, { showSlots: true, showCodes: true })
    expect(has(c[0], 'ctl')).toBe(true)
    expect(has(c[0], 'slot')).toBe(false)
    for (let i = 1; i <= 4; i++) {
      expect(has(c[i], 'slot')).toBe(true)
      expect(has(c[i], 'ctl')).toBe(false)
    }
  })
})

test('CONTROL_MARKS holds the design table for all 32 codes', () => {
  const M = MOSAIC_MARK
  const k = 'KRGYBMCW'
  const expected = [
    ...[...k].map((c) => ['A', c]),
    ['F', 'L'], ['S', 't'], ['E', 'X'], ['S', 'X'], ['N', 'S'], ['D', 'H'], ['D', 'W'], ['D', 'S'],
    ...[...k].map((c) => [M, c]),
    ['C', 'O'], ['C', M], ['S', M], ['S', 'W'], ['K', 'B'], ['N', 'B'], ['H', M], ['R', M],
  ]
  expect(CONTROL_MARKS).toHaveLength(32)
  expect(CONTROL_MARKS.map((m) => [...m])).toEqual(expected)
})

test('showCodes draws 0x1d as N with subscript B', () => {
  const c = mount(TeletextPage, { props: { rows: [''], showCodes: true } }).find('.tt-cell')
  expect([c.find('span.big').text(), c.find('span.sub').text()]).toEqual(['N', 'B'])
})
