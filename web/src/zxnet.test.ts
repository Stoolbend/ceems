import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, test } from 'vitest'
import { subpages } from './teletext/tti'
import { EDITOR, decode, encode } from './zxnet'

const SP = ' '.repeat(40)
const grid = (cell?: [number, number]) => {
  const rows = Array.from({ length: 25 }, () => '\0'.repeat(40))
  if (cell) rows[cell[0]] = rows[cell[0]].slice(0, cell[1]) + '\x7f' + rows[cell[0]].slice(cell[1] + 1)
  return rows
}
const dataOf = (url: string) => url.slice(EDITOR.length).split(':')[1]
const FL = ['100', '200', '300', '400', '', '100']
const enc = (rows: string[]) => encode(rows, '1a0', FL)
const frag = (data: string, extra = '') => `0:${data}${extra}`
const spaces1120 = 'QIECBAg'.repeat(160)

describe('golden vectors', () => {
  test('spaces', () => {
    const rows = Array(25).fill(SP)
    expect(dataOf(enc(rows))).toBe('QIECBAg'.repeat(166) + 'QIECA')
    expect(decode(enc(rows))).toEqual({ rows, pn: '1A0' })
  })
  test('1120-character spaces decodes with row 24 blank', () => {
    expect(decode(frag(spaces1120))).toEqual({ rows: Array(25).fill(SP), pn: null })
  })
  test('0x7F at row 0, column 0', () => {
    const rows = grid([0, 0])
    expect(dataOf(enc(rows))).toBe('_g' + 'A'.repeat(1165))
    expect((decode(enc(rows)) as { rows: string[] }).rows).toEqual(rows)
  })
  test('0x7F at row 1, column 0', () => {
    const rows = grid([1, 0])
    expect(dataOf(enc(rows))).toBe('A'.repeat(46) + 'D-' + 'A'.repeat(1119))
    expect((decode(enc(rows)) as { rows: string[] }).rows).toEqual(rows)
  })
})

test('fixture pages round trip rows 1-24', () => {
  const dir = join(__dirname, '../../testdata/sample/pages')
  let n = 0
  for (const f of readdirSync(dir).filter((f) => f.endsWith('.tti'))) {
    for (const s of subpages(readFileSync(join(dir, f), 'latin1'))) {
      const rows = [SP, ...Array.from({ length: 24 }, (_, i) => (s.rows[i + 1] ?? '').padEnd(40, ' '))]
      const back = decode(encode(rows, s.mpp, FL))
      expect(back).not.toBeInstanceOf(Error)
      expect((back as { rows: string[] }).rows).toEqual(rows)
      n++
    }
  }
  expect(n).toBe(41)
})

describe('decode refuses', () => {
  const shape = 'not a zxnet editor URL (expected meta:pagedata)'
  const chars = 'page data has a character outside A-Z a-z 0-9 - _'
  const ok = 'QIECBAg'.repeat(166) + 'QIECA'
  test.each([
    ['1119 characters', frag(spaces1120.slice(1)), 'page data must be 1120 or 1167 characters, got 1119'],
    ['1168 characters', frag(ok + 'A'), 'page data must be 1120 or 1167 characters, got 1168'],
    ['plus', frag('+' + ok.slice(1)), chars],
    ['space', frag(' ' + ok.slice(1)), chars],
    ['mid equals', frag('=' + ok.slice(1)), chars],
    ['no colon', ok, shape],
    ['meta a', 'a:' + ok, shape],
  ])('%s', (_, text, message) => {
    expect(decode(text)).toEqual(new Error(message))
  })
})

describe('decode accepts', () => {
  const ok = 'QIECBAg'.repeat(166) + 'QIECA'
  const want = { rows: Array(25).fill(SP), pn: null }
  test.each([
    ['trailing =', '0:' + ok + '=='],
    ['full URL', `https://zxnet.co.uk/teletext/editor/#0:${ok}`],
    ['bare #', `#0:${ok}`],
    ['bare', `0:${ok}`],
    ['unknown field', `0:${ok}:zx=whatever`],
  ])('%s', (_, text) => expect(decode(text)).toEqual(want))
  test('PN', () => {
    expect(decode(`0:${ok}:PN=1a0`)).toMatchObject({ pn: '1A0' })
    expect(decode(`0:${ok}:PN=9FF`)).toMatchObject({ pn: null })
  })
})

test('encode fields', () => {
  const url = encode(Array(25).fill(SP), '1a0', FL)
  expect(url.startsWith(EDITOR + '0:')).toBe(true)
  expect(url.endsWith(':PN=1A0:X270=1003F7F2003F7F3003F7F4003F7F8FF3F7F1003F7FF')).toBe(true)
  expect(url).not.toMatch(/:(PS|SC|RE)=/)
})
