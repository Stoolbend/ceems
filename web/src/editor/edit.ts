import { renderPage, SEXTANT_BITS } from '../teletext/render'
import { cellOf, PALETTE } from './cells'

// The page as edited: rows 1-24 are exactly 40 cell values each, and the cursor never leaves them.
// lastRow is the last row the cursor and writes may reach (default 24; 23 when a fastext group writes row 24).
// pending is true between Esc and the key that names a control code.
// anchor is the fixed corner of a block selection (the cursor is the other); undefined means no selection.
export type State = { rows: string[]; row: number; col: number; insert: boolean; pending: boolean; lastRow?: number; anchor?: { row: number; col: number } }
export type Key = { key: string; code?: string; ctrlKey?: boolean; shiftKey?: boolean; metaKey?: boolean; altKey?: boolean }

const LAST_ROW = 24 // the default State.lastRow
const LAST_COL = 39
const MODIFIERS = ['Shift', 'Control', 'Alt', 'AltGraph', 'Meta', 'CapsLock']
const SIXEL_KEYS = 'qwaszx' // in SEXTANT_BITS order

/** Whether row r is the hidden lower half of a double-height row (the rule of TeletextPage's belowDouble). */
export function lowerHalf(rows: readonly string[], r: number): boolean {
  let below = false
  for (let i = 1; i <= r; i++) below = !below && !!rows[i - 1]?.includes('\x0d')
  return below
}

// The first row from r, stepping by step, that the cursor may sit on; undefined past the edge.
function editableRow(rows: readonly string[], r: number, step: 1 | -1, last = LAST_ROW): number | undefined {
  for (; r >= 1 && r <= last; r += step) if (!lowerHalf(rows, r)) return r
  return undefined
}

const setCell = (row: string, col: number, cell: number) => row.slice(0, col) + String.fromCharCode(cell) + row.slice(col + 1)
const isBlastThrough = (v: number) => v >= 0x40 && v <= 0x5f

function withRow(s: State, text: string, col = s.col): State {
  const rows = [...s.rows]
  rows[s.row] = text
  return { ...s, rows, col, anchor: undefined }
}

// Whether mosaic mode is in force at a cell. The cell is read as a space, because a
// blast-through capital is drawn as text (mosaic false) even inside a mosaic run.
function inMosaic(rows: readonly string[], row: number, col: number): boolean {
  const spaced = rows.map((r, i) => (i === row ? setCell(r, col, 0x20) : r))
  return renderPage(spaced)[row][col].mosaic
}

/** The sixel (an index into SEXTANT_BITS, 0 top-left to 5 bottom-right) at a position inside a cell, given as fractions from 0 to 1. */
export function sixelAt(fx: number, fy: number): number {
  const line = Math.max(0, Math.min(2, Math.floor(fy * 3)))
  return line * 2 + (fx < 0.5 ? 0 : 1)
}

/** Whether a sixel of the cell is set, or undefined when the cell cannot be painted: row 0, a hidden lower half, a control code or no mosaic. */
export function sixelOn(rows: readonly string[], row: number, col: number, sixel: number, last = LAST_ROW): boolean | undefined {
  if (row < 1 || row > last || lowerHalf(rows, row)) return undefined
  const v = rows[row].charCodeAt(col)
  if (v < 0x20 || !inMosaic(rows, row, col)) return undefined
  return !isBlastThrough(v) && (v & (1 << SEXTANT_BITS[sixel])) !== 0
}

/** Sets or clears one sixel and moves the cursor to the cell. Returns s itself when nothing changes. A control code is never written. */
export function paint(s: State, row: number, col: number, sixel: number, on: boolean): State {
  const was = sixelOn(s.rows, row, col, sixel, s.lastRow)
  if (was === undefined || was === on) return s
  const v = s.rows[row].charCodeAt(col)
  const base = isBlastThrough(v) ? 0x20 : v
  const bit = 1 << SEXTANT_BITS[sixel]
  const rows = [...s.rows]
  rows[row] = setCell(rows[row], col, (on ? base | bit : base & ~bit) | 0x20)
  return { ...s, rows, row, col, pending: false, anchor: undefined }
}

/** Writes one cell at the cursor as typing does: overwrite or insert, then move right, stopping at column 39. */
export function write(s: State, cell: number): State {
  const row = s.rows[s.row]
  const ch = String.fromCharCode(cell)
  const text = s.insert ? row.slice(0, s.col) + ch + row.slice(s.col, LAST_COL) : setCell(row, s.col, cell)
  return withRow(s, text, Math.min(LAST_COL, s.col + 1))
}

/**
 * Pastes text as a block: the first line starts at the cursor and each later line at the same column on the
 * next row the cursor may sit on. Characters with no cell are dropped, a line stops at column 39 and rows
 * past 24 are dropped. Returns s itself when nothing is written.
 */
export function paste(s: State, text: string): State {
  const lines = text.split(/\r\n|\n|\r/)
  if (lines[lines.length - 1] === '') lines.pop()
  const rows = [...s.rows]
  let end: { row: number; col: number } | undefined
  let r: number | undefined = s.row
  for (const [i, line] of lines.entries()) {
    if (i > 0) r = editableRow(rows, r! + 1, 1, s.lastRow)
    if (r === undefined) break
    let col = s.col
    for (const ch of line) {
      const cell = cellOf(ch)
      if (cell === undefined) continue
      rows[r] = s.insert ? rows[r].slice(0, col) + String.fromCharCode(cell) + rows[r].slice(col, LAST_COL) : setCell(rows[r], col, cell)
      end = { row: r, col: Math.min(LAST_COL, col + 1) }
      if (col++ === LAST_COL) break
    }
  }
  return end ? { ...s, rows, ...end, pending: false, anchor: undefined } : s
}

/** The selected rectangle, inclusive: the anchor and the cursor, normalised. */
export function selection(s: State): { top: number; left: number; bottom: number; right: number } | undefined {
  const a = s.anchor
  if (!a) return undefined
  return { top: Math.min(a.row, s.row), left: Math.min(a.col, s.col), bottom: Math.max(a.row, s.row), right: Math.max(a.col, s.col) }
}

/** The selected cells, one string per row, as stored (control codes and hidden lower halves included). */
export function block(s: State): string[] | undefined {
  const r = selection(s)
  return r && s.rows.slice(r.top, r.bottom + 1).map((row) => row.slice(r.left, r.right + 1))
}

/** Blanks the selected rectangle on every row, hidden lower halves included. Returns s itself when nothing is selected. */
export function cut(s: State): State {
  const r = selection(s)
  if (!r) return s
  const rows = s.rows.map((row, i) => (i < r.top || i > r.bottom ? row : row.slice(0, r.left) + ' '.repeat(r.right - r.left + 1) + row.slice(r.right + 1)))
  return { ...s, rows, pending: false, anchor: undefined }
}

/**
 * Overwrites a rectangle with its top-left at the cursor. Rows are consecutive (a hidden lower half is not
 * skipped), lines stop at column 39, rows past 24 are dropped and insert mode is ignored. Returns s itself
 * when nothing is written.
 */
export function pasteBlock(s: State, lines: readonly string[]): State {
  const rows = [...s.rows]
  let wrote = false
  for (const [i, line] of lines.entries()) {
    const r = s.row + i
    const part = line.slice(0, LAST_COL + 1 - s.col)
    if (r > (s.lastRow ?? LAST_ROW) || !part) continue
    rows[r] = rows[r].slice(0, s.col) + part + rows[r].slice(s.col + part.length)
    wrote = true
  }
  return wrote ? { ...s, rows, pending: false, anchor: undefined } : s
}

const EXTEND = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown']

/** The state after a key, or null when the key is not the editor's (the caller must not preventDefault). */
export function apply(s: State, key: Key): State | null {
  if (key.key === 'Escape' && s.anchor && !key.metaKey) return { ...s, anchor: undefined }
  const next = applyKey(s, key)
  if (!next) return null
  const extend = key.shiftKey && !key.ctrlKey && !s.pending && EXTEND.includes(key.key)
  return { ...next, anchor: extend ? (s.anchor ?? { row: s.row, col: s.col }) : undefined }
}

function applyKey(s: State, key: Key): State | null {
  if (key.metaKey) return null
  if (s.pending) {
    if (MODIFIERS.includes(key.key)) return null // Shift must be able to precede a digit
    const digit = /^Digit([1-7])$/.exec(key.code ?? '')
    const code = digit ? Number(digit[1]) + (key.shiftKey ? 0x10 : 0) : PALETTE.find((p) => p.key === key.key)?.code
    return code === undefined ? { ...s, pending: false } : write({ ...s, pending: false }, code)
  }
  if (key.key === 'Escape') return { ...s, pending: true }
  // Ctrl with Alt is AltGr, which types its character; plain Alt is left to the browser.
  const altGr = key.ctrlKey && key.altKey
  if (key.altKey && !key.ctrlKey) return null
  const row = s.rows[s.row]
  switch (key.key) {
    case 'Insert':
      return key.shiftKey || key.ctrlKey ? null : { ...s, insert: !s.insert } // the browser's own paste and copy
    case 'ArrowLeft':
      return { ...s, col: Math.max(0, s.col - 1) }
    case 'ArrowRight':
      return { ...s, col: Math.min(LAST_COL, s.col + 1) }
    case 'ArrowUp':
      return { ...s, row: editableRow(s.rows, s.row - 1, -1, s.lastRow) ?? s.row }
    case 'ArrowDown':
      return { ...s, row: editableRow(s.rows, s.row + 1, 1, s.lastRow) ?? s.row }
    case 'Enter': {
      const r = editableRow(s.rows, s.row + 1, 1, s.lastRow)
      return r === undefined ? s : { ...s, row: r, col: 0 }
    }
    case 'Home':
      return key.ctrlKey ? { ...s, row: editableRow(s.rows, 1, 1, s.lastRow) ?? s.row } : { ...s, col: 0 }
    case 'End':
      return key.ctrlKey ? { ...s, row: editableRow(s.rows, s.lastRow ?? LAST_ROW, -1, s.lastRow) ?? s.row } : { ...s, col: LAST_COL }
    case 'Backspace': {
      if (s.col === 0) return s
      const c = s.col - 1
      return withRow(s, s.insert ? row.slice(0, c) + row.slice(c + 1) + ' ' : setCell(row, c, 0x20), c)
    }
    case 'Delete':
      return withRow(s, row.slice(0, s.col) + row.slice(s.col + 1) + ' ')
  }
  if (key.key.length !== 1 || (key.ctrlKey && !altGr)) return null

  if ('qwaszxrfc'.includes(key.key) && inMosaic(s.rows, s.row, s.col)) {
    const v = row.charCodeAt(s.col)
    // A blast-through capital or a control code has no sixels of its own, so it starts from a space.
    const base = v < 0x20 || isBlastThrough(v) ? 0x20 : v
    const bit = SIXEL_KEYS.indexOf(key.key)
    const next = key.key === 'f' ? 0x7f : key.key === 'c' ? 0x20 : key.key === 'r' ? base ^ 0x5f : base ^ (1 << SEXTANT_BITS[bit])
    return withRow(s, setCell(row, s.col, next | 0x20))
  }
  const cell = cellOf(key.key)
  return cell === undefined ? null : write(s, cell)
}
