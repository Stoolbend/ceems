import { glyph } from '../teletext/render'

// A row is edited as text in which every cell is one UTF-16 unit:
// control cells 0x00-0x1F show as Control Pictures (U+2400-U+241F), 0x7F as U+2421,
// and 0x20-0x7E as the glyph the preview draws.
function editableChar(cell: number): string {
  if (cell < 0x20) return String.fromCharCode(0x2400 + cell)
  if (cell === 0x7f) return String.fromCharCode(0x2421)
  return glyph(cell)
}

const toCell = new Map<string, number>()
for (let c = 0; c < 0x80; c++) toCell.set(editableChar(c), c)

/** Cell values 0-127 as editable text, one character per cell. */
export function toEditable(cells: string): string {
  return Array.from(cells, (ch) => editableChar(ch.charCodeAt(0))).join('')
}

/** Editable text back to cell values, or an Error naming the first character that is not a cell. */
export function fromEditable(text: string): string | Error {
  let out = ''
  for (const ch of text) {
    const c = toCell.get(ch)
    if (c === undefined) return new Error(`"${ch}" is not a teletext character`)
    out += String.fromCharCode(c)
  }
  return out
}

/** The cell a typed character stands for: the inverse of glyph over 0x20-0x7E, else its own ASCII code, else undefined. */
export function cellOf(ch: string): number | undefined {
  const c = toCell.get(ch)
  if (c !== undefined && c >= 0x20 && c < 0x7f) return c
  const a = ch.length === 1 ? ch.charCodeAt(0) : 0
  return a >= 0x20 && a < 0x7f ? a : undefined
}

const NAMES: [number, string, string][] = [
  [0x00, 'Alpha black', 'k'], [0x01, 'Alpha red', 'r'], [0x02, 'Alpha green', 'g'], [0x03, 'Alpha yellow', 'y'],
  [0x04, 'Alpha blue', 'b'], [0x05, 'Alpha magenta', 'm'], [0x06, 'Alpha cyan', 'c'], [0x07, 'Alpha white', 'w'],
  [0x08, 'Flash', 'f'], [0x09, 'Steady', 'F'], [0x0c, 'Normal height', 'D'], [0x0d, 'Double height', 'd'],
  [0x10, 'Mosaic black', 'K'], [0x11, 'Mosaic red', 'R'], [0x12, 'Mosaic green', 'G'], [0x13, 'Mosaic yellow', 'Y'],
  [0x14, 'Mosaic blue', 'B'], [0x15, 'Mosaic magenta', 'M'], [0x16, 'Mosaic cyan', 'C'], [0x17, 'Mosaic white', 'W'],
  [0x18, 'Conceal', 'o'], [0x19, 'Contiguous mosaic', 'S'], [0x1a, 'Separated mosaic', 's'],
  [0x1c, 'Black background', 'N'], [0x1d, 'New background', 'n'], [0x1e, 'Hold', 'h'], [0x1f, 'Release', 'H'],
]

// What each code does, as cell values: numbers are cells, strings are literal characters.
const sample = (...v: (number | string)[]) => v.map((x) => (typeof x === 'string' ? x : String.fromCharCode(x))).join('')
const SAMPLES: Record<number, string> = {
  0x00: sample(0x1d, 0x00, 'A'),
  0x08: sample(0x08, 'A'),
  0x09: sample(0x08, 'A', 0x09, 'A'),
  0x0c: sample(0x0d, 'A', 0x0c, 'A'),
  0x0d: sample(0x0d, 'A'),
  0x10: sample(0x1d, 0x10, 0x7f),
  0x18: sample('A', 0x18, 'B'),
  0x19: sample(0x17, 0x1a, 0x7f, 0x19, 0x7f),
  0x1a: sample(0x17, 0x7f, 0x1a, 0x7f),
  0x1c: sample(0x01, 0x1d, 0x07, 'A', 0x1c, 'A'),
  0x1d: sample(0x01, 0x1d, 0x07, 'A'),
  0x1e: sample(0x17, 0x7f, 0x1e, 0x12, 0x7f),
  0x1f: sample(0x17, 0x7f, 0x1e, 0x1f, 0x12, 0x7f),
}

const OTHER = new Set([0x08, 0x09, 0x0c, 0x0d, 0x18, 0x1c, 0x1d])
const groupOf = (code: number) => (code < 0x08 ? ('alpha' as const) : OTHER.has(code) ? ('other' as const) : ('mosaic' as const))

const GROUPS = ['alpha', 'mosaic', 'other']

/** The control codes that have an effect in the preview, with the text to insert and a sample of each. */
export const PALETTE = NAMES.map(([code, name, key]) => {
  const cells = SAMPLES[code] ?? sample(code, code < 0x10 ? 'A' : 0x7f) // the plain colour codes
  return { code, name, key, group: groupOf(code), char: editableChar(code), sample: cells, rows: cells.includes('\x0d') ? (2 as const) : (1 as const) }
}).sort((a, b) => GROUPS.indexOf(a.group) - GROUPS.indexOf(b.group)) // stable: NAMES order within a group, so the palette reads section by section
