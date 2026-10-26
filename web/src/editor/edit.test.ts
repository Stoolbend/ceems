import { describe, expect, test } from 'vitest'
import { PALETTE } from './cells'
import { apply, block, cut, lowerHalf, paint, paste, pasteBlock, selection, sixelAt, sixelOn, write, type Key, type State } from './edit'

const SPACES = ' '.repeat(40)
// A page whose rows 1-24 are 40 spaces, with the given rows replaced (each padded to 40).
function page(over: Record<number, string> = {}): string[] {
  return Array.from({ length: 25 }, (_, r) => (r === 0 ? 'HEADER' : (over[r] ?? '').padEnd(40, ' ')))
}
const at = (row: number, col: number, over: Partial<State> = {}): State => ({ rows: page(), row, col, insert: false, pending: false, ...over })
const run = (s: State, ...keys: (string | Key)[]) =>
  keys.reduce<State>((st, k) => {
    const next = apply(st, typeof k === 'string' ? { key: k } : k)
    if (!next) throw new Error(`key ${JSON.stringify(k)} not handled`)
    return next
  }, s)
const cell = (s: State, r = s.row, c = s.col) => s.rows[r].charCodeAt(c)

describe('typing', () => {
  test('overwrite sets the cell and moves right', () => {
    const s = run(at(3, 5, { rows: page({ 3: 'abcdefgh' }) }), 'X')
    expect(s.rows[3]).toBe('abcdeXgh'.padEnd(40, ' '))
    expect([s.row, s.col]).toEqual([3, 6])
  })

  test('insert shifts the rest of the row right, dropping cell 39', () => {
    const s = run(at(1, 1, { rows: page({ 1: 'ab'.padEnd(39, '.') + 'Z' }), insert: true }), 'X')
    expect(s.rows[1]).toBe('aXb'.padEnd(40, '.'))
    expect(s.rows[1]).toHaveLength(40)
    expect(s.col).toBe(2)
  })

  test('stops at column 39 and never wraps, in both modes', () => {
    for (const insert of [false, true]) {
      const s = run(at(2, 39, { insert }), 'A', 'B')
      expect([s.row, s.col]).toEqual([2, 39])
      expect(s.rows[2]).toBe(' '.repeat(39) + 'B')
      expect(s.rows[3]).toBe(SPACES)
    }
  })

  test('£ and # go through cellOf', () => {
    const s = run(at(1, 0), '£', '#')
    expect([cell(s, 1, 0), cell(s, 1, 1)]).toEqual([0x23, 0x5f])
  })

  test('only the edited row is a new string', () => {
    const s0 = at(4, 0)
    const s = run(s0, 'A')
    expect(s.rows).not.toBe(s0.rows)
    expect(s.rows[5]).toBe(s0.rows[5])
    expect(s0.rows[4]).toBe(SPACES) // the input state is untouched
  })

  test('write puts any cell value, a control code included', () => {
    const s = write(at(1, 0, { rows: page({ 1: 'HELLO' }) }), 0x01)
    expect(s.rows[1]).toBe('\x01ELLO'.padEnd(40, ' '))
    expect(s.col).toBe(1)
  })
})

describe('Backspace and Delete', () => {
  test('Backspace in overwrite moves left and blanks the cell', () => {
    const s = run(at(1, 3, { rows: page({ 1: 'abcdef' }) }), 'Backspace')
    expect(s.rows[1]).toBe('ab def'.padEnd(40, ' '))
    expect(s.col).toBe(2)
  })

  test('Backspace in insert moves left and shifts the rest left, padding with a space', () => {
    const s = run(at(1, 3, { rows: page({ 1: 'abcdef'.padEnd(40, 'z') }), insert: true }), 'Backspace')
    expect(s.rows[1]).toBe('abdef'.padEnd(39, 'z') + ' ')
    expect(s.col).toBe(2)
  })

  test('Backspace at column 0 does nothing but is handled', () => {
    for (const insert of [false, true]) {
      const s0 = at(1, 0, { rows: page({ 1: 'abc' }), insert })
      expect(apply(s0, { key: 'Backspace' })).toEqual(s0)
    }
  })

  test('Delete shifts the row left from the cursor in both modes', () => {
    for (const insert of [false, true]) {
      const s = run(at(1, 1, { rows: page({ 1: 'abcdef'.padEnd(40, 'z') }), insert }), 'Delete')
      expect(s.rows[1]).toBe('acdef'.padEnd(39, 'z') + ' ')
      expect(s.col).toBe(1)
    }
  })
})

describe('movement', () => {
  test('Enter goes to column 0 of the next row, and past row 24 does nothing', () => {
    expect(run(at(3, 17), 'Enter')).toMatchObject({ row: 4, col: 0 })
    expect(run(at(24, 17), 'Enter')).toMatchObject({ row: 24, col: 17 })
  })

  test('Home and End go to column 0 and 39', () => {
    expect(run(at(5, 17), 'Home')).toMatchObject({ row: 5, col: 0 })
    expect(run(at(5, 17), 'End')).toMatchObject({ row: 5, col: 39 })
  })

  test('Ctrl+Home and Ctrl+End go to row 1 and 24, keeping the column', () => {
    expect(run(at(5, 17), { key: 'Home', ctrlKey: true })).toMatchObject({ row: 1, col: 17 })
    expect(run(at(5, 17), { key: 'End', ctrlKey: true })).toMatchObject({ row: 24, col: 17 })
  })

  test('the arrows move and stop at every edge', () => {
    expect(run(at(5, 5), 'ArrowUp', 'ArrowLeft')).toMatchObject({ row: 4, col: 4 })
    expect(run(at(5, 5), 'ArrowDown', 'ArrowRight')).toMatchObject({ row: 6, col: 6 })
    expect(run(at(1, 0), 'ArrowUp', 'ArrowLeft')).toMatchObject({ row: 1, col: 0 })
    expect(run(at(24, 39), 'ArrowDown', 'ArrowRight')).toMatchObject({ row: 24, col: 39 })
  })

  test('Insert toggles insert mode', () => {
    expect(run(at(1, 0), 'Insert').insert).toBe(true)
    expect(run(at(1, 0), 'Insert', 'Insert').insert).toBe(false)
  })
})

describe('double height', () => {
  const rows = page({ 5: '\x0dBIG', 24: '\x0dLAST' })

  test('lowerHalf follows the belowDouble rule', () => {
    expect([4, 5, 6, 7].map((r) => lowerHalf(rows, r))).toEqual([false, false, true, false])
    // a double-height code on a lower half does not double the row after it
    expect(lowerHalf(page({ 5: '\x0d', 6: '\x0d' }), 7)).toBe(false)
  })

  test('Up, Down and Enter skip the hidden lower half', () => {
    expect(run(at(5, 3, { rows }), 'ArrowDown')).toMatchObject({ row: 7, col: 3 })
    expect(run(at(7, 3, { rows }), 'ArrowUp')).toMatchObject({ row: 5, col: 3 })
    expect(run(at(5, 3, { rows }), 'Enter')).toMatchObject({ row: 7, col: 0 })
  })

  test('double height on row 24 is allowed, and Ctrl+End skips a lower-half row 24', () => {
    expect(run(at(23, 0, { rows }), 'ArrowDown').row).toBe(24)
    const r23 = page({ 23: '\x0d' })
    expect(run(at(5, 0, { rows: r23 }), { key: 'End', ctrlKey: true }).row).toBe(23)
    expect(run(at(23, 0, { rows: r23 }), 'ArrowDown').row).toBe(23)
    expect(r23[24]).toBe(SPACES) // the stored lower half is untouched
  })
})

describe('mosaic cells', () => {
  // 0x17 (mosaic white) at column 0, then the cell under test at column 1.
  const mosaicAt = (v: number) => at(1, 1, { rows: page({ 1: '\x17' + String.fromCharCode(v) }) })
  const keyResult = (v: number, key: string) => cell(run(mosaicAt(v), key), 1, 1)

  test.each([
    ['q', 0],
    ['w', 1],
    ['a', 2],
    ['s', 3],
    ['z', 4],
    ['x', 6],
  ])('%s toggles sextant bit %i on a contiguous mosaic cell, keeping bit 5', (key, bit) => {
    expect(keyResult(0x20, key)).toBe(0x20 | (1 << bit))
    expect(keyResult(0x7f, key)).toBe(0x7f & ~(1 << bit))
    const s = run(mosaicAt(0x20), key)
    expect(s.col).toBe(1) // the sixel keys do not move the cursor
  })

  test.each(['q', 'w', 'a', 's', 'z', 'x'])('%s on a blast-through cell starts from a space', (key) => {
    const bit = [0, 1, 2, 3, 4, 6]['qwaszx'.indexOf(key)]
    expect(keyResult(0x41, key)).toBe(0x20 | (1 << bit)) // 'A'
  })

  test('r inverts the six sixels, f fills and c clears, always keeping bit 5', () => {
    expect(keyResult(0x21, 'r')).toBe(0x7e)
    expect(keyResult(0x7f, 'r')).toBe(0x20)
    expect(keyResult(0x41, 'r')).toBe(0x7f) // blast-through: inverts a space
    expect(keyResult(0x41, 'f')).toBe(0x7f)
    expect(keyResult(0x33, 'f')).toBe(0x7f)
    expect(keyResult(0x7f, 'c')).toBe(0x20)
    expect(keyResult(0x41, 'c')).toBe(0x20)
    for (const v of [0x20, 0x33, 0x41, 0x5f, 0x7f]) for (const k of 'qwaszxrfc') expect(keyResult(v, k) & 0x20, `${v} ${k}`).toBe(0x20)
  })

  test('other keys type normally in a mosaic cell, capitals as blast-through text', () => {
    const s = run(mosaicAt(0x20), 'Q', 'e')
    expect([cell(s, 1, 1), cell(s, 1, 2)]).toEqual([0x51, 0x65])
    expect(s.col).toBe(3)
  })

  test('the sixel keys are plain letters in an alpha cell', () => {
    const s = run(at(1, 0), 'q')
    expect(cell(s, 1, 0)).toBe(0x71)
    expect(s.col).toBe(1)
  })
})

describe('keys left to the browser', () => {
  test.each([
    [{ key: 'Dead' }],
    [{ key: 'Process' }],
    [{ key: 'Unidentified' }],
    [{ key: 'Tab' }],
    [{ key: 'Tab', shiftKey: true }],
    [{ key: 'a', ctrlKey: true }],
    [{ key: 'v', metaKey: true }],
    [{ key: 'é' }],
  ])('%o returns null', (key) => {
    expect(apply(at(1, 0), key)).toBeNull()
  })
})

describe('the Esc layer', () => {
  const mosaicRow = at(1, 2, { rows: page({ 1: '\x17\x7f\x7f' }) })
  test.each(PALETTE)('Esc then $key writes $name', (p) => {
    const s = run(at(1, 3), 'Escape', p.key)
    expect(cell(s, 1, 3)).toBe(p.code)
    expect([s.col, s.pending]).toEqual([4, false])
  })

  test.each([1, 2, 3, 4, 5, 6, 7])('Esc then Digit%i writes the alpha colour, and with Shift the mosaic colour', (d) => {
    expect(cell(run(at(1, 0), 'Escape', { key: String(d), code: `Digit${d}` }), 1, 0)).toBe(d)
    expect(cell(run(at(1, 0), 'Escape', { key: '!', code: `Digit${d}`, shiftKey: true }), 1, 0)).toBe(0x10 + d)
  })

  test('a lone modifier keeps pending and is not handled, so Shift+1 works', () => {
    const s = run(at(1, 0), 'Escape')
    expect(apply(s, { key: 'Shift' })).toBeNull()
    expect(s.pending).toBe(true)
  })

  test('Esc Esc cancels and writes nothing', () => {
    const s = run(at(1, 0), 'Escape', 'Escape')
    expect([s.pending, s.rows]).toEqual([false, at(1, 0).rows])
  })

  test('an unmapped key cancels, writes nothing and does not move', () => {
    const s = run(at(1, 5), 'Escape', 'ArrowLeft')
    expect([s.pending, s.col, s.rows]).toEqual([false, 5, at(1, 5).rows])
  })

  test('in a mosaic cell, Esc q is unmapped and toggles nothing', () => {
    const s = run(mosaicRow, 'Escape', 'q')
    expect([cell(s, 1, 2), s.pending]).toEqual([0x7f, false])
  })

  test('in a mosaic cell, Esc r writes alpha red, not an inversion', () => {
    expect(cell(run(mosaicRow, 'Escape', 'r'), 1, 2)).toBe(0x01)
  })

  test('a written code inserts in insert mode', () => {
    const s = run(at(1, 0, { rows: page({ 1: 'AB' }), insert: true }), 'Escape', 'r')
    expect(s.rows[1]).toBe('\x01AB'.padEnd(40, ' '))
  })
})

describe('modifiers', () => {
  test('Ctrl+Alt is AltGr and types the character', () => {
    expect(cell(run(at(1, 0), { key: '@', ctrlKey: true, altKey: true }), 1, 0)).toBe(0x40)
  })
  test.each([[{ key: 'a', altKey: true }], [{ key: 'a', ctrlKey: true }]])('%o returns null', (key) => {
    expect(apply(at(1, 0), key)).toBeNull()
  })
})

test('q on a control code inside a mosaic run gives 0x21', () => {
  const s = run(at(1, 1, { rows: page({ 1: '\x17\x11' }) }), 'q')
  expect(cell(s, 1, 1)).toBe(0x21)
})

describe('Insert with modifiers', () => {
  test('Shift+Insert and Ctrl+Insert are left to the browser, plain Insert toggles', () => {
    expect(apply(at(1, 0), { key: 'Insert', shiftKey: true })).toBeNull()
    expect(apply(at(1, 0), { key: 'Insert', ctrlKey: true })).toBeNull()
    expect(apply(at(1, 0), { key: 'Insert' })?.insert).toBe(true)
  })
})

describe('paste', () => {
  test('a line is written at the cursor and the cursor follows', () => {
    const s = paste(at(3, 5), 'AB')
    expect(s.rows[3].slice(5, 7)).toBe('AB')
    expect([s.row, s.col, s.pending]).toEqual([3, 7, false])
  })

  test('later lines start at the same column on the next row; one trailing newline is dropped', () => {
    const s = paste(at(3, 5), 'AB\r\nCD')
    expect(s.rows[3].slice(5, 7)).toBe('AB')
    expect(s.rows[4].slice(5, 7)).toBe('CD')
    expect([s.row, s.col]).toEqual([4, 7])
    expect(paste(at(3, 5), 'AB\n')).toEqual(paste(at(3, 5), 'AB'))
    expect(paste(at(3, 5), 'A\rB').rows[4][5]).toBe('B')
  })

  test('a long line fills to column 39 and drops the rest, keeping the character that reached it', () => {
    const s = paste(at(2, 36), 'ABCDEFG')
    expect(s.rows[2].slice(36)).toBe('ABCD')
    expect([s.row, s.col]).toEqual([2, 39])
  })

  test('a character with no cell is dropped and takes no column', () => {
    expect(paste(at(1, 0), 'AéB\t').rows[1].slice(0, 3)).toBe('AB ')
  })

  test('rows past 24 are dropped', () => {
    const s = paste(at(23, 0), 'A\nB\nC')
    expect(s.rows[23][0]).toBe('A')
    expect(s.rows[24][0]).toBe('B')
    expect([s.row, s.col]).toEqual([24, 1])
  })

  test('the hidden lower half of a double-height row is skipped', () => {
    const s = paste(at(3, 1, { rows: page({ 3: '\x0dA' }) }), 'Z\nY')
    expect(s.rows[4]).toBe(SPACES)
    expect(s.rows[5][1]).toBe('Y')
    expect(s.row).toBe(5)
  })

  test('insert mode shifts existing cells right', () => {
    const s = paste(at(1, 1, { rows: page({ 1: 'abcd' }), insert: true }), 'XY')
    expect(s.rows[1]).toBe('aXYbcd'.padEnd(40, ' '))
  })

  test('nothing written returns the same state', () => {
    const s = at(1, 1)
    expect(paste(s, '')).toBe(s)
    expect(paste(s, 'é')).toBe(s)
  })
})

describe('mouse painting', () => {
  // 0x17 (mosaic white) at column 0, then the cell under test at column 1.
  const MOSAIC = String.fromCharCode(0x17)
  const DOUBLE = String.fromCharCode(0x0d)
  const mosaicAt = (v: number) => at(3, 5, { rows: page({ 1: MOSAIC + String.fromCharCode(v) }), pending: true })
  const set = (v: number, sixel: number, on: boolean) => paint(mosaicAt(v), 1, 1, sixel, on)

  test.each([
    [0.1, 0.1, 0],
    [0.9, 0.1, 1],
    [0.1, 0.5, 2],
    [0.9, 0.5, 3],
    [0.1, 0.9, 4],
    [0.9, 0.9, 5],
    [0.5, 0.5, 3], // x = 0.5 is the right half
    [0.1, 1, 4], // y = 1 is the bottom row
    [-1, -1, 0],
    [2, 2, 5],
  ])('sixelAt(%d, %d) is %d', (fx, fy, want) => {
    expect(sixelAt(fx, fy)).toBe(want)
  })

  test('sixelOn reads the bit, and a blast-through capital as all off', () => {
    expect(sixelOn(mosaicAt(0x21).rows, 1, 1, 0)).toBe(true)
    expect(sixelOn(mosaicAt(0x21).rows, 1, 1, 1)).toBe(false)
    expect(sixelOn(mosaicAt(0x60).rows, 1, 1, 5)).toBe(true)
    expect(sixelOn(mosaicAt(0x5f).rows, 1, 1, 0)).toBe(false)
  })

  test('sixelOn is undefined for an alpha cell, a control code, row 0 and a lower-half row', () => {
    expect(sixelOn(page({ 1: 'AB' }), 1, 1, 0)).toBeUndefined()
    expect(sixelOn(mosaicAt(0x20).rows, 1, 0, 0)).toBeUndefined() // the control code itself
    expect(sixelOn(page({ 1: MOSAIC + ' ' }), 0, 1, 0)).toBeUndefined()
    expect(sixelOn(page({ 3: DOUBLE + MOSAIC + ' ', 4: MOSAIC + ' ' }), 4, 1, 0)).toBeUndefined()
  })

  test('paint sets and clears one sixel, keeps bit 5, moves the cursor and clears pending', () => {
    const on = set(0x20, 3, true)
    expect(cell(on, 1, 1)).toBe(0x28)
    expect(set(0x28, 3, false).rows[1].charCodeAt(1)).toBe(0x20)
    expect(set(0x7f, 5, false).rows[1].charCodeAt(1)).toBe(0x3f)
    expect([on.row, on.col, on.pending, on.insert]).toEqual([1, 1, false, false])
  })

  test('paint starts from a space on a blast-through capital', () => {
    expect(cell(set(0x41, 0, true), 1, 1)).toBe(0x21)
    const capital = mosaicAt(0x41)
    expect(paint(capital, 1, 1, 1, false)).toBe(capital) // it reads as all off, so there is nothing to clear
  })

  test('paint returns the same state when nothing changes or the cell cannot be painted', () => {
    const s = mosaicAt(0x21)
    expect(paint(s, 1, 1, 0, true)).toBe(s)
    expect(paint(s, 1, 1, 1, false)).toBe(s)
    expect(paint(s, 1, 0, 0, true)).toBe(s) // a control code is never written
    expect(paint(s, 0, 1, 0, true)).toBe(s)
    const alpha = at(1, 0)
    expect(paint(alpha, 1, 1, 0, true)).toBe(alpha)
  })
})

describe('block selection', () => {
  const shift = (key: string): Key => ({ key, shiftKey: true })
  const picked = () => run(at(3, 5), shift('ArrowRight'), shift('ArrowRight'))

  test('Shift+arrows extend a selection from where it began', () => {
    const s = picked()
    expect(s.anchor).toEqual({ row: 3, col: 5 })
    expect([s.row, s.col]).toEqual([3, 7])
    expect(selection(s)).toEqual({ top: 3, left: 5, bottom: 3, right: 7 })
    expect(block(s)).toHaveLength(1)
    const two = run(s, shift('ArrowUp'))
    expect(selection(two)).toEqual({ top: 2, left: 5, bottom: 3, right: 7 })
    expect(block(two)).toHaveLength(2)
    expect(selection(run(at(3, 5), shift('ArrowLeft'), shift('ArrowDown')))).toEqual({ top: 3, left: 4, bottom: 4, right: 5 })
  })

  test('block returns the stored cells of the rectangle', () => {
    const s = run(at(3, 5, { rows: page({ 3: '01234\x01AB89' }) }), shift('ArrowRight'), shift('ArrowRight'))
    expect(block(s)).toEqual(['\x01AB'])
    expect(block(at(1, 1))).toBeUndefined()
    expect(selection(at(1, 1))).toBeUndefined()
  })

  test('any other handled key clears it; Ctrl+letter and Tab leave it', () => {
    for (const k of ['ArrowLeft', 'x', 'Backspace', 'Enter', 'Home']) expect(run(picked(), k).anchor).toBeUndefined()
    expect(apply(picked(), { key: 'c', ctrlKey: true })).toBeNull()
    expect(apply(picked(), { key: 'Tab' })).toBeNull()
    expect(picked().anchor).toBeDefined()
  })

  test('Esc drops the selection first, then opens the code layer', () => {
    const s = run(picked(), 'Escape')
    expect([s.anchor, s.pending]).toEqual([undefined, false])
    expect(run(at(3, 5), 'Escape').pending).toBe(true)
    expect(run(picked(), 'Escape', 'Escape').pending).toBe(true)
  })

  test('block and cut cover a hidden lower-half row as stored', () => {
    const rows = page({ 3: '\x0dABC', 4: 'xyz' })
    const s = at(3, 1, { rows, anchor: { row: 3, col: 1 } })
    const t = { ...s, row: 4, col: 2 } // a rectangle reaches the hidden row 4 even though the cursor cannot sit on it
    expect(block(t)).toEqual(['AB', 'yz'])
    const c = cut(t)
    expect(c.rows[3].slice(0, 4)).toBe('\x0d  C')
    expect(c.rows[4].slice(0, 3)).toBe('x  ')
    expect([c.row, c.col, c.anchor, c.pending]).toEqual([4, 2, undefined, false])
    expect(c.rows[0]).toBe('HEADER')
  })

  test('cut with no selection returns the same state', () => {
    const s = at(2, 2)
    expect(cut(s)).toBe(s)
  })

  test('pasteBlock writes consecutive rows, even over a hidden lower half, and keeps codes', () => {
    const s = pasteBlock(at(3, 2, { rows: page({ 3: '\x0d' }), insert: true, pending: true, anchor: { row: 1, col: 1 } }), ['\x01A', 'BC'])
    expect(s.rows[3].slice(0, 4)).toBe('\x0d \x01A')
    expect(s.rows[4].slice(0, 4)).toBe('  BC')
    expect([s.row, s.col, s.anchor, s.pending, s.insert]).toEqual([3, 2, undefined, false, true])
  })

  test('pasteBlock clips at column 39 and drops rows past 24', () => {
    const s = pasteBlock(at(24, 38), ['ABC', 'DEF'])
    expect(s.rows[24].slice(36)).toBe('  AB')
    expect(s.rows[24]).toHaveLength(40)
    expect(s.rows).toHaveLength(25)
  })

  test('pasteBlock returns the same state when nothing is written', () => {
    const s = at(2, 2)
    expect(pasteBlock(s, [])).toBe(s)
    expect(pasteBlock(s, [''])).toBe(s)
    const last = at(24, 0)
    expect(pasteBlock(last, ['', 'X'])).toBe(last)
  })

  test('paste, write and paint clear the anchor', () => {
    const anchor = { row: 1, col: 1 }
    expect(paste(at(1, 0, { anchor }), 'A').anchor).toBeUndefined()
    expect(write(at(1, 0, { anchor }), 0x41).anchor).toBeUndefined()
    const m = at(1, 0, { rows: page({ 1: '\x17 ' }), anchor })
    expect(paint(m, 1, 1, 0, true).anchor).toBeUndefined()
  })
})

describe('lastRow 23 (row 24 written by a fastext group)', () => {
  test('the cursor stops at row 23 and Ctrl+End goes there', () => {
    expect(run(at(23, 4, { lastRow: 23 }), 'ArrowDown', 'Enter').row).toBe(23)
    expect(run(at(1, 4, { lastRow: 23 }), { key: 'End', ctrlKey: true }).row).toBe(23)
  })

  test('paste and pasteBlock drop rows past 23', () => {
    const s = at(23, 0, { lastRow: 23 })
    expect(paste(s, 'AB\nCD').rows[24]).toBe(SPACES)
    expect(pasteBlock(s, ['AB', 'CD']).rows[24]).toBe(SPACES)
    const on24 = at(24, 0, { lastRow: 23 })
    expect(pasteBlock(on24, ['AB'])).toBe(on24)
  })

  test('a sixel on row 24 is not paintable', () => {
    const rows = page({ 24: '\x17\x7f' })
    expect(sixelOn(rows, 24, 1, 0)).toBe(true)
    expect(sixelOn(rows, 24, 1, 0, 23)).toBeUndefined()
  })
})
