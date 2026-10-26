export interface Cell {
  ch: number // value 0x20-0x7F actually displayed (after hold substitution; control codes become 0x20)
  fg: number // 0-7, ETS colour index (0 black .. 7 white)
  bg: number
  mosaic: boolean // ch is drawn from G1 (mosaic mode and ch not in 0x40-0x5F)
  separated: boolean // only ever true when mosaic
  height: 'normal' | 'top' | 'bottom'
  conceal: boolean
  flash: boolean
}

const ROWS = 25
const COLS = 40

// English national option subset of G0 (ETS 300 706 table 36); other values are ASCII.
const ENGLISH: Record<number, string> = {
  0x23: '£', 0x24: '$', 0x40: '@', 0x5b: '←', 0x5c: '½', 0x5d: '→', 0x5e: '↑', 0x5f: '#',
  0x60: '—', 0x7b: '¼', 0x7c: '‖', 0x7d: '¾', 0x7e: '÷', 0x7f: '█',
}

export function glyph(ch: number): string {
  return ENGLISH[ch] ?? String.fromCharCode(ch)
}

// Bits of a mosaic cell value for top-left, top-right, mid-left, mid-right, bottom-left, bottom-right.
export const SEXTANT_BITS = [0, 1, 2, 3, 4, 6]

export function sextantChar(ch: number): string {
  let p = 0
  SEXTANT_BITS.forEach((b, i) => {
    if (ch & (1 << b)) p |= 1 << i
  })
  if (p === 0) return ' '
  if (p === 63) return '█'
  if (p === 21) return '▌'
  if (p === 42) return '▐'
  return String.fromCodePoint(0x1fb00 + p - 1 - (p > 21 ? 1 : 0) - (p > 42 ? 1 : 0))
}

const isBlastThrough = (v: number) => v >= 0x40 && v <= 0x5f

interface Held {
  ch: number
  separated: boolean
}

// Returns the row's cells and whether it contains a double-height code.
function renderRow(row: string): { cells: Cell[]; hasDouble: boolean } {
  let fg = 7
  let bg = 0
  let isMosaic = false
  let flash = false
  let conceal = false
  let separated = false
  let height: Cell['height'] = 'normal'
  let hold = false
  let held: Held = { ch: 0x20, separated: false }
  let hasDouble = false
  const cells: Cell[] = []

  for (let i = 0; i < COLS; i++) {
    const v = (i < row.length ? row.charCodeAt(i) : 0x20) & 0x7f

    // set-at codes
    if (v === 0x09) flash = false
    else if (v === 0x0c && height !== 'normal') {
      height = 'normal'
      held = { ch: 0x20, separated: false }
    } else if (v === 0x18) conceal = true
    else if (v === 0x19) separated = false
    else if (v === 0x1a) separated = true
    else if (v === 0x1c) bg = 0
    else if (v === 0x1d) bg = fg
    else if (v === 0x1e) hold = true

    // emit the cell using the state now in force
    let ch = v
    let mosaic = isMosaic && !isBlastThrough(v)
    let sep = separated
    if (v < 0x20) {
      ch = 0x20
      mosaic = isMosaic
      if (hold && isMosaic) {
        ch = held.ch
        sep = held.separated
      }
    } else if (isMosaic && !isBlastThrough(v)) {
      held = { ch: v, separated }
    }
    cells.push({ ch, fg, bg, mosaic, separated: mosaic && sep, height, conceal, flash })

    // set-after codes
    if (v <= 0x07 || (v >= 0x10 && v <= 0x17)) {
      const toMosaic = v >= 0x10
      fg = v & 7
      conceal = false
      if (toMosaic !== isMosaic) {
        isMosaic = toMosaic
        held = { ch: 0x20, separated: false }
      }
    } else if (v === 0x08) flash = true
    else if (v === 0x0d) {
      hasDouble = true
      if (height === 'normal') {
        height = 'top'
        held = { ch: 0x20, separated: false }
      }
    } else if (v === 0x1f) hold = false
  }
  return { cells, hasDouble }
}

const blank = (bg: number): Cell => ({
  ch: 0x20, fg: 7, bg, mosaic: false, separated: false, height: 'normal', conceal: false, flash: false,
})

export function renderPage(rows: readonly string[]): Cell[][] {
  const grid: Cell[][] = []
  let doubleAbove = false
  for (let r = 0; r < ROWS; r++) {
    if (doubleAbove) {
      // the row below a double-height row shows only the bottom halves of the row above
      grid.push(
        grid[r - 1].map((c) => (c.height === 'top' ? { ...c, height: 'bottom' as const } : blank(c.bg))),
      )
      doubleAbove = false
      continue
    }
    const { cells, hasDouble } = renderRow(rows[r] ?? '')
    grid.push(cells)
    doubleAbove = hasDouble && r < ROWS - 1
  }
  return grid
}

const FLAGS: [(c: Cell) => boolean, string][] = [
  [(c) => c.separated, 'S'],
  [(c) => c.height === 'top', 'T'],
  [(c) => c.height === 'bottom', 'B'],
  [(c) => c.conceal, 'C'],
  [(c) => c.flash, 'F'],
]

// Snapshot form: per row, lines of glyphs, fg digits, bg digits and flags.
export function gridText(grid: Cell[][]): string {
  const line = (row: Cell[], f: (c: Cell) => string) => row.map(f).join('')
  return (
    grid
      .map((row) =>
        [
          line(row, (c) => (c.mosaic ? sextantChar(c.ch) : glyph(c.ch))),
          line(row, (c) => String(c.fg)),
          line(row, (c) => String(c.bg)),
          line(row, (c) => FLAGS.find(([test]) => test(c))?.[1] ?? '.'),
        ].join('\n'),
      )
      .join('\n\n') + '\n'
  )
}
