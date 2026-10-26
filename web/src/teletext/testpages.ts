import { DEFAULT_HEADER, headerRow } from './header'

// Control-code test pages shared by the tests and the gallery. Numbers are cell values, strings are text.
const row = (...parts: (string | number)[]) =>
  parts.map((p) => (typeof p === 'number' ? String.fromCharCode(p) : p)).join('')

const page = (rows: Record<number, string>): string[] => {
  const out = Array<string>(25).fill('')
  for (const [r, s] of Object.entries(rows)) out[Number(r)] = s
  return out
}

const seq = (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, i) => from + i)

export const testPages: Record<string, string[]> = {
  'alpha-colours': page({
    1: row(...seq(0, 7).flatMap((c) => [c, 'A'])),
  }),
  'mosaic-colours': page({
    1: row(...seq(0x10, 0x17).flatMap((c) => [c, 0x7f])),
  }),
  'contiguous-separated': page({
    1: row(0x17, 0x7f, 0x7f, 0x1a, 0x7f, 0x7f, 0x19, 0x7f, 0x7f),
  }),
  'held-graphics': page({
    // 0x17 0x7F 0x1E 0x13 0x7F 0x1F 0x3F, then blast-through 0x41 and an alpha code after hold.
    1: row(0x17, 0x7f, 0x1e, 0x13, 0x7f, 0x1f, 0x3f, 0x0a),
    2: row(0x17, 0x7f, 0x1e, 0x41, 0x1f, 0x20),
    3: row(0x17, 0x7f, 0x1e, 0x02, 'A'),
  }),
  'double-height': page({
    5: row(0x01, 0x04, 0x1d, 0x0d, 'AB', 0x0c, 'C'),
    6: row('NOT SHOWN', 0x0d),
    7: row('normal'),
    24: row(0x0d, 'LAST'),
  }),
  background: page({
    1: row(0x01, 0x1d, 'AB', 0x1c, 'CD'),
    2: row(0x04, 0x1d, 'EF'),
  }),
  conceal: page({
    1: row(0x18, 'AB', 0x02, 'CD'),
  }),
  flash: page({
    1: row(0x08, 'AB', 0x09, 'CD'),
  }),
  charset: page({
    1: row('A', 0x23, 0x5b, 0x5c, 0x5d, 0x5e, 0x5f, 0x60, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f),
    2: row(0x17, ...seq(0x40, 0x5f)),
  }),
  rows: page({
    1: row('short'),
    // row 2 missing
    3: row(0x01, 0x1d, 0x08, 0x1a, 'X'),
    4: row('clean'),
    5: 'x'.repeat(41),
    6: row(0x02, 0x0a, 0x0b, 0x0e, 0x0f, 0x1b, 'Z'),
  }),
}

testPages.header = page({ 0: headerRow(DEFAULT_HEADER, '81A', new Date(2026, 0, 1, 12, 34, 56)) })
