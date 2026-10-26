import { renderPage } from '../teletext/render'
import { expect, test } from 'vitest'
import { PALETTE, cellOf, fromEditable, toEditable } from './cells'

test('every cell 0-127 round-trips and is one UTF-16 unit', () => {
  const all = Array.from({ length: 128 }, (_, i) => String.fromCharCode(i)).join('')
  const text = toEditable(all)
  expect(text.length).toBe(128)
  expect(new Set(text).size).toBe(128)
  expect(fromEditable(text)).toBe(all)
})

test('control codes show as Control Pictures and 0x7F as U+2421', () => {
  expect(toEditable('\x01\x1f\x7f')).toBe('␁␟␡')
})

test('a character that is not a cell is an Error', () => {
  expect(fromEditable('abcé')).toBeInstanceOf(Error)
})

test('cellOf inverts the glyphs of 0x20-0x7E and refuses anything else', () => {
  expect(cellOf('£')).toBe(0x23)
  expect(cellOf('#')).toBe(0x5f)
  expect(cellOf('A')).toBe(0x41)
  expect(cellOf('é')).toBeUndefined()
  expect(cellOf('␁')).toBeUndefined() // a control picture is not typed text
  expect(cellOf('█')).toBeUndefined() // 0x7F
})

test('the palette lists the 27 codes with an effect', () => {
  expect(PALETTE).toHaveLength(27)
  expect(PALETTE.find((p) => p.code === 0x1e)?.name).toBe('Hold')
  expect(PALETTE.every((p) => p.char.length === 1)).toBe(true)
})

const sampleOf = (code: number) => PALETTE.find((p) => p.code === code)!.sample

test('every palette sample is one or two rows of cells 0-127', () => {
  for (const p of PALETTE) {
    expect(p.rows, p.name).toBe(p.sample.includes('\x0d') ? 2 : 1)
    expect([...p.sample].every((c) => c.charCodeAt(0) < 128), p.name).toBe(true)
  }
})

test('the black samples sit on white, and Hold and Release give the held and blank cells', () => {
  for (const code of [0x00, 0x10]) {
    const cell = renderPage([sampleOf(code)])[0][2]
    expect([cell.fg, cell.bg], `code ${code}`).toEqual([0, 7])
  }
  expect(renderPage([sampleOf(0x1e)])[0][3].ch).toBe(0x7f)
  expect(renderPage([sampleOf(0x1f)])[0][4].ch).toBe(0x20)
})

test('palette groups: 8 alpha, 12 mosaic, 7 other, as listed in D-038', () => {
  const codes = (g: string) => PALETTE.filter((p) => p.group === g).map((p) => p.code)
  expect(codes('alpha')).toEqual([0, 1, 2, 3, 4, 5, 6, 7])
  expect(codes('mosaic')).toEqual([0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x19, 0x1a, 0x1e, 0x1f])
  expect(codes('other')).toEqual([0x08, 0x09, 0x0c, 0x0d, 0x18, 0x1c, 0x1d])
})

test('every palette entry has its own key, as in the key table', () => {
  const expected: Record<number, string> = {
    0x00: 'k', 0x01: 'r', 0x02: 'g', 0x03: 'y', 0x04: 'b', 0x05: 'm', 0x06: 'c', 0x07: 'w',
    0x10: 'K', 0x11: 'R', 0x12: 'G', 0x13: 'Y', 0x14: 'B', 0x15: 'M', 0x16: 'C', 0x17: 'W',
    0x08: 'f', 0x09: 'F', 0x0d: 'd', 0x0c: 'D', 0x1d: 'n', 0x1c: 'N', 0x1e: 'h', 0x1f: 'H',
    0x1a: 's', 0x19: 'S', 0x18: 'o',
  }
  expect(PALETTE.map((p) => [p.code, p.key])).toEqual(expect.arrayContaining(Object.entries(expected).map(([c, k]) => [Number(c), k])))
  expect(new Set(PALETTE.map((p) => p.key)).size).toBe(27)
})

test('every printable ASCII character can be typed', () => {
  expect(cellOf('[')).toBe(0x5b)
  expect(cellOf('_')).toBe(0x5f)
  expect(cellOf('{')).toBe(0x7b)
  expect(cellOf('`')).toBe(0x60)
  expect(cellOf('£')).toBe(0x23)
  expect(cellOf('#')).toBe(0x5f)
  expect(cellOf('é')).toBeUndefined()
  for (let c = 0x20; c < 0x7f; c++) expect(cellOf(String.fromCharCode(c))).toBeDefined()
})
