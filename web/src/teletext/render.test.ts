import { expect, test } from 'vitest'
import { glyph, gridText, renderPage, sextantChar, type Cell } from './render'
import { testPages } from './testpages'

const page = (name: string) => renderPage(testPages[name])
const sp = (g: Cell[][], r: number) => g[r]

test('always 25 x 40, even for empty or oversize input', () => {
  for (const rows of [[], ['x'.repeat(100)], Array(40).fill('hi')]) {
    const g = renderPage(rows)
    expect(g).toHaveLength(25)
    expect(g.every((r) => r.length === 40)).toBe(true)
  }
  expect(renderPage([String.fromCharCode(0xc1)])[0][0].ch).toBe(0x41) // masked with & 0x7F
})

test('alpha-colours: colour applies after the code cell', () => {
  const r = sp(page('alpha-colours'), 1)
  for (let k = 0; k < 8; k++) {
    expect(r[2 * k].fg).toBe(k === 0 ? 7 : k - 1)
    expect(r[2 * k].ch).toBe(0x20)
    expect(r[2 * k + 1]).toMatchObject({ ch: 0x41, fg: k, mosaic: false })
  }
})

test('mosaic-colours', () => {
  const r = sp(page('mosaic-colours'), 1)
  for (let k = 0; k < 8; k++) {
    expect(r[2 * k].ch).toBe(0x20)
    expect(r[2 * k + 1]).toMatchObject({ ch: 0x7f, fg: k, mosaic: true })
  }
})

test('contiguous-separated flips at the code cell', () => {
  const r = sp(page('contiguous-separated'), 1)
  expect(r.map((c) => c.separated).slice(0, 9)).toEqual([false, false, false, true, true, true, false, false, false])
})

test('held-graphics', () => {
  const g = page('held-graphics')
  const r = sp(g, 1)
  expect(r[2]).toMatchObject({ ch: 0x7f, fg: 7, mosaic: true }) // 0x1E shows the held character
  expect(r[3]).toMatchObject({ ch: 0x7f, fg: 7, mosaic: true }) // 0x13: held, old colour
  expect(r[4]).toMatchObject({ ch: 0x7f, fg: 3 })
  expect(r[5]).toMatchObject({ ch: 0x7f, fg: 3 }) // 0x1F still shows it
  expect(r[6].ch).toBe(0x3f)
  expect(r[7].ch).toBe(0x20) // code cell after release
  expect(sp(g, 2)[3]).toMatchObject({ ch: 0x41, mosaic: false }) // blast-through
  expect(sp(g, 2)[4].ch).toBe(0x7f) // not replaced by blast-through
  expect(sp(g, 3)[3]).toMatchObject({ ch: 0x7f, fg: 7, mosaic: true }) // alpha code with hold on
  expect(sp(g, 3)[4]).toMatchObject({ ch: 0x41, fg: 2, mosaic: false })
})

test('double-height', () => {
  const g = page('double-height')
  expect(g[5].map((c) => c.height).slice(0, 8)).toEqual(['normal', 'normal', 'normal', 'normal', 'top', 'top', 'normal', 'normal'])
  expect(g[6][4]).toMatchObject({ ch: 0x41, height: 'bottom', bg: 4 })
  expect(g[6][5]).toMatchObject({ ch: 0x42, height: 'bottom' })
  expect(g[6][7]).toMatchObject({ ch: 0x20, height: 'normal', bg: 4 })
  expect(g[6][0]).toMatchObject({ ch: 0x20, bg: 0 })
  expect(g[6].map((c) => glyph(c.ch)).join('')).not.toContain('NOT')
  expect(g[7].map((c) => glyph(c.ch)).join('')).toContain('normal') // row 6's 0x0D did not carry on
  expect(g[7].every((c) => c.height === 'normal')).toBe(true)
  expect(g).toHaveLength(25) // 0x0D on row 24 suppresses nothing
  expect(g[24][1].height).toBe('top')
})

test('background', () => {
  const g = page('background')
  expect(g[1][2].bg).toBe(1)
  expect(g[1][3].bg).toBe(1)
  expect(g[1][4].bg).toBe(0) // 0x1C black background is set-at
  expect(g[1][5].bg).toBe(0)
  expect(g[2][1].bg).toBe(4) // new background takes the fg in force
  expect(g[2][2].bg).toBe(4)
})

test('conceal ends at the colour code, glyphs stay', () => {
  const r = page('conceal')[1]
  expect(r.map((c) => c.conceal).slice(0, 5)).toEqual([true, true, true, true, false])
  expect(r[1].ch).toBe(0x41)
})

test('flash flag', () => {
  const r = page('flash')[1]
  expect(r.map((c) => c.flash).slice(0, 5)).toEqual([false, true, true, false, false])
})

test('charset', () => {
  const g = page('charset')
  expect(g[1].slice(1, 13).map((c) => glyph(c.ch)).join('')).toBe('£←½→↑#—¼‖¾÷█')
  expect(g[2].slice(1, 33).every((c, i) => c.ch === 0x40 + i && !c.mosaic)).toBe(true)
  expect(sextantChar(0x7f)).toBe('█')
  expect(sextantChar(0x20)).toBe(' ')
})

test('rows: padding, truncation, no leakage, ignored codes', () => {
  const g = page('rows')
  expect(g[1][5]).toMatchObject({ ch: 0x20, fg: 7 })
  expect(g[2].every((c) => c.ch === 0x20 && c.fg === 7 && c.bg === 0)).toBe(true)
  expect(g[3][4]).toMatchObject({ ch: 0x58, bg: 1, flash: true, separated: false })
  expect(g[4][0]).toMatchObject({ ch: 0x63, fg: 7, bg: 0, flash: false })
  expect(g[5].every((c) => c.ch === 0x78)).toBe(true)
  for (let i = 1; i <= 5; i++) expect(g[6][i]).toMatchObject({ ch: 0x20, fg: 2, bg: 0 })
  expect(g[6][6]).toMatchObject({ ch: 0x5a, fg: 2, mosaic: false, flash: false, conceal: false })
})

test('gridText has four lines per row', () => {
  expect(gridText(page('flash')).split('\n\n')).toHaveLength(25)
})

test('header page: time and page number substituted, cells 0-7 untouched', () => {
  const r = page('header')[0]
  expect(r.map((c) => glyph(c.ch)).join('').trimEnd()).toBe('        CeeMS        81A        12:34:56')
  expect(r.slice(0, 8).every((c) => c.ch === 0x20 && c.fg === 7)).toBe(true)
})
