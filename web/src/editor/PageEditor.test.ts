import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import PageEditor from './PageEditor.vue'
import { decode, encode } from '../zxnet'

const rowsOf = (over: Record<number, string> = {}) => Array.from({ length: 25 }, (_, r) => (r === 0 ? 'HEADER' : (over[r] ?? '').padEnd(40, ' ')))
const mountEditor = (props: { rows?: string[]; disabled?: boolean; draw?: boolean; lastRow?: number } = {}) =>
  mount(PageEditor, { props: { rows: rowsOf({ 1: 'HELLO' }), showCodes: false, showSlots: false, grid: false, draw: false, disabled: false, ...props } })
type W = ReturnType<typeof mountEditor>
const cellAt = (w: W, r: number, c: number) => w.findAll('.tt-row')[r].findAll('.tt-cell')[c]
const status = (w: W) => w.find('p.status').text()
const cursor = (w: W) => w.find<HTMLElement>('.cursor').element.style

test('starts at row 1, column 0 in overwrite, and says so', () => {
  const w = mountEditor()
  expect(status(w)).toBe('Row 1, column 0, Overwrite, H')
  expect(w.find('p.status').attributes('aria-live')).toBe('polite')
  expect(cursor(w).left).toBe('calc(0%)') // jsdom folds calc(col * 2.5%)
  expect(cursor(w).top).toBe('calc(4%)')
})

test('a click on a cell moves the cursor there and the status line follows', async () => {
  const w = mountEditor()
  await cellAt(w, 3, 7).trigger('click')
  expect(status(w)).toBe('Row 3, column 7, Overwrite,') // the glyph of a space; text() trims it
  expect(cursor(w).left).toBe('calc(17.5%)')
  expect(cursor(w).top).toBe('calc(12%)')
  await cellAt(w, 1, 4).find('.g').trigger('click') // a click on the glyph inside the cell
  expect(status(w)).toBe('Row 1, column 4, Overwrite, O')
})

test('a click on row 0 is ignored', async () => {
  const w = mountEditor()
  await cellAt(w, 3, 7).trigger('click')
  await cellAt(w, 0, 2).trigger('click')
  expect(status(w)).toMatch(/^Row 3, column 7,/)
})

test('a click on the hidden lower half of a double-height row moves to the row above', async () => {
  const w = mountEditor({ rows: rowsOf({ 4: '\x0dBIG' }) })
  await cellAt(w, 5, 2).trigger('click')
  expect(status(w)).toBe('Row 4, column 2, Overwrite, I')
})

test('the status names a control code and a mosaic cell, and shows Insert', async () => {
  const w = mountEditor({ rows: rowsOf({ 2: '\x11\x7f' }) })
  await cellAt(w, 2, 0).trigger('click')
  expect(status(w)).toBe('Row 2, column 0, Overwrite, Mosaic red')
  await w.find('[role=application]').trigger('keydown', { key: 'Insert' })
  await w.find('[role=application]').trigger('keydown', { key: 'ArrowRight' })
  expect(status(w)).toBe('Row 2, column 1, Insert, mosaic')
})

test('keys emit update:rows only when the rows change, and handled keys are prevented', async () => {
  const w = mountEditor()
  const app = w.find('[role=application]')
  await app.trigger('keydown', { key: 'ArrowRight' })
  expect(w.emitted('update:rows')).toBeUndefined()
  expect(status(w)).toMatch(/^Row 1, column 1,/)
  await app.trigger('keydown', { key: 'J' })
  const sent = w.emitted<[string[]]>('update:rows')!
  expect(sent).toHaveLength(1)
  expect(sent[0][0][1]).toBe('HJLLO'.padEnd(40, ' '))
  expect(sent[0][0][0]).toBe('HEADER')
  const handled = new KeyboardEvent('keydown', { key: 'ArrowLeft', cancelable: true })
  app.element.dispatchEvent(handled)
  expect(handled.defaultPrevented).toBe(true)
  const tab = new KeyboardEvent('keydown', { key: 'Tab', cancelable: true })
  app.element.dispatchEvent(tab)
  expect(tab.defaultPrevented).toBe(false)
})

test('when disabled, keys do nothing and there is no tabindex', async () => {
  const w = mountEditor({ disabled: true })
  const app = w.find('[role=application]')
  expect(app.attributes('tabindex')).toBeUndefined()
  expect(app.attributes('aria-label')).toBe('Page editor')
  await app.trigger('keydown', { key: 'X' })
  await app.trigger('keydown', { key: 'ArrowDown' })
  expect(w.emitted('update:rows')).toBeUndefined()
  expect(status(w)).toMatch(/^Row 1, column 0,/)
  expect(mountEditor().find('[role=application]').attributes('tabindex')).toBe('0')
})

test('write(0x01) puts 0x01 at the cursor, moves right and refocuses the page', async () => {
  const w = mount(PageEditor, { props: { rows: rowsOf({ 1: 'HELLO' }), showCodes: false, showSlots: false, grid: false, draw: false, disabled: false }, attachTo: document.body })
  await cellAt(w, 1, 2).trigger('click')
  ;(w.vm as unknown as { write: (c: number) => void }).write(0x01)
  await w.vm.$nextTick()
  expect(w.emitted<[string[]]>('update:rows')![0][0][1]).toBe('HE\x01LO'.padEnd(40, ' '))
  expect(status(w)).toMatch(/^Row 1, column 3,/)
  expect(document.activeElement).toBe(w.find('[role=application]').element)
  w.unmount()
})

test('showCodes and grid reach the page', () => {
  const w = mount(PageEditor, { props: { rows: rowsOf({ 1: '\x01A' }), showCodes: true, showSlots: false, grid: true, draw: false, disabled: false } })
  expect(w.find('.tt-page').classes()).toContain('show-grid')
  expect(w.findAll('.tt-page .ctl').length).toBeGreaterThan(0)
})

test('showSlots reaches the page', () => {
  const w = mount(PageEditor, { props: { rows: rowsOf({ 1: '{ab}' }), showCodes: false, showSlots: true, grid: false, draw: false, disabled: false } })
  expect(w.findAll('.tt-page .slot')).toHaveLength(4)
})

test('Esc then r emits rows with alpha red at the cursor, and the status shows Code… only in between', async () => {
  const w = mountEditor()
  const app = w.find('[role=application]')
  await app.trigger('keydown', { key: 'Escape' })
  expect(status(w)).toContain('Code…')
  await app.trigger('keydown', { key: 'r' })
  expect(status(w)).not.toContain('Code…')
  const rows = (w.emitted('update:rows')![0] as string[][])[0]
  expect(rows[1].charCodeAt(0)).toBe(0x01)
})

// Undo, redo and paste (T-033). Without a parent the rows prop never follows the emits, so these tests feed them back.
type Rows = string[][]
async function press(w: W, init: KeyboardEventInit & { key: string }) {
  const before = (w.emitted<[string[]]>('update:rows') ?? []).length
  const ev = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  w.find('[role=application]').element.dispatchEvent(ev)
  const sent = (w.emitted<[string[]]>('update:rows') ?? []) as Rows[]
  if (sent.length > before) await w.setProps({ rows: sent[sent.length - 1][0] as unknown as string[] })
  await w.vm.$nextTick()
  return ev
}
const row1 = (w: W) => w.props('rows')[1].slice(0, 4).trimEnd()
const ctrl = (key: string, extra: KeyboardEventInit = {}) => ({ key, ctrlKey: true, ...extra })

test('Ctrl+Z undoes one change at a time and restores the cursor; Ctrl+Y and Ctrl+Shift+Z redo', async () => {
  const w = mountEditor({ rows: rowsOf() })
  await press(w, { key: 'A' })
  await press(w, { key: 'B' })
  expect(row1(w)).toBe('AB')
  await press(w, ctrl('z'))
  expect(row1(w)).toBe('A')
  expect(status(w)).toMatch(/^Row 1, column 1,/)
  await press(w, ctrl('z'))
  expect(row1(w)).toBe('')
  expect(status(w)).toMatch(/^Row 1, column 0,/)
  await press(w, ctrl('y'))
  expect(row1(w)).toBe('A')
  await press(w, ctrl('Z', { shiftKey: true }))
  expect(row1(w)).toBe('AB')
})

test('a new change after an undo clears redo', async () => {
  const w = mountEditor({ rows: rowsOf() })
  await press(w, { key: 'A' })
  await press(w, ctrl('z'))
  await press(w, { key: 'C' })
  const n = w.emitted('update:rows')!.length
  await press(w, ctrl('y'))
  expect(w.emitted('update:rows')).toHaveLength(n)
  expect(row1(w)).toBe('C')
})

test('undo keeps 200 steps', async () => {
  const w = mountEditor({ rows: rowsOf() })
  for (let i = 0; i < 201; i++) await press(w, { key: i % 2 ? 'A' : 'B' })
  for (let i = 0; i < 201; i++) await press(w, ctrl('z'))
  expect(w.emitted('update:rows')).toHaveLength(401) // 201 changes and 200 undos
  expect(w.props('rows')[1].slice(0, 1)).toBe('B') // the first change could not be undone
}, 20000) // about 400 key events; over the 5 s default under full-suite load (KI-004)

test('Ctrl+Z on an empty stack is consumed and emits nothing; Ctrl+Alt+Z is not undo', async () => {
  const w = mountEditor({ rows: rowsOf() })
  expect((await press(w, ctrl('z'))).defaultPrevented).toBe(true)
  expect(w.emitted('update:rows')).toBeUndefined()
  await press(w, { key: 'A' })
  await press(w, ctrl('z', { altKey: true })) // AltGr types its character
  expect(row1(w)).toBe('Az')
})

function pasteInto(w: W, text: string) {
  const ev = new Event('paste', { cancelable: true, bubbles: true }) as Event & { clipboardData: unknown }
  ev.clipboardData = { getData: (t: string) => (t === 'text/plain' ? text : '') }
  w.find('[role=application]').element.dispatchEvent(ev)
  return ev
}

test('a text paste writes at the cursor and is one undo step', async () => {
  const w = mountEditor({ rows: rowsOf() })
  const ev = pasteInto(w, 'AB\nCD')
  expect(ev.defaultPrevented).toBe(true)
  const sent = w.emitted<[string[]]>('update:rows')!
  expect(sent).toHaveLength(1)
  expect(sent[0][0][1].slice(0, 2)).toBe('AB')
  expect(sent[0][0][2].slice(0, 2)).toBe('CD')
  await w.setProps({ rows: sent[0][0] })
  await press(w, ctrl('z'))
  expect(w.emitted<[string[]]>('update:rows')![1][0][1]).toBe(rowsOf()[1])
})

test('pasting a zxnet URL replaces rows 1-24 and clears the error', () => {
  const w = mountEditor({ rows: rowsOf() })
  const src = Array.from({ length: 25 }, (_, r) => `R${r}`.padEnd(40, ' '))
  pasteInto(w, `  ${encode(src, '1A0', [])}\n`)
  const out = w.emitted<[string[]]>('update:rows')![0][0]
  expect(out[0]).toBe('HEADER')
  expect(out.slice(1)).toEqual(src.slice(1))
  expect(w.emitted('error')).toEqual([['']])
})

test('pasting a bad zxnet URL emits the decode message and no rows', () => {
  const w = mountEditor()
  pasteInto(w, 'https://x/#0:abc')
  expect(w.emitted('update:rows')).toBeUndefined()
  expect(w.emitted('error')).toEqual([[(decode('https://x/#0:abc') as Error).message]])
})

test('a paste on a disabled editor does nothing', () => {
  const w = mountEditor({ disabled: true })
  const ev = pasteInto(w, 'AB')
  expect(w.emitted('update:rows')).toBeUndefined()
  expect(w.emitted('error')).toBeUndefined()
  expect(ev.defaultPrevented).toBe(false)
})

// Mouse painting (T-038). One rect for every cell is enough, because the cell comes from the event target.
describe('Draw mode', () => {
  const W = 20
  const H = 30
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ left: 0, top: 0, width: W, height: H, right: W, bottom: H, x: 0, y: 0, toJSON: () => ({}) })
  })
  afterEach(() => vi.restoreAllMocks())

  // 0x17 (mosaic white) at column 0, mosaic cells after it, and 0x01 (alpha red) at column 5.
  const mosaicRows = () => rowsOf({ 1: '\x17\x20\x20\x20\x20\x01 ', 2: 'ALPHA' })
  const mouse = async (w: W, type: string, target: ReturnType<typeof cellAt>, x: number, y: number, buttons = 1) => {
    const before = (w.emitted<[string[]]>('update:rows') ?? []).length
    target.element.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, buttons, clientX: x, clientY: y }))
    const sent = (w.emitted<[string[]]>('update:rows') ?? []) as Rows[]
    if (sent.length > before) await w.setProps({ rows: sent[sent.length - 1][0] as unknown as string[] })
    await w.vm.$nextTick()
  }
  const TL = [2, 3] as const // top-left sixel of a 20 by 30 cell
  const BR = [15, 25] as const // bottom-right sixel
  const v = (w: W, c: number) => w.props('rows')[1].charCodeAt(c)

  test('a drag across two mosaic cells paints both, and one Ctrl+Z restores rows and cursor', async () => {
    const w = mountEditor({ rows: mosaicRows(), draw: true })
    const before = w.props('rows')
    await mouse(w, 'mousedown', cellAt(w, 1, 1), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...BR)
    await mouse(w, 'mouseup', cellAt(w, 1, 2), ...BR, 0)
    expect([v(w, 1), v(w, 2)]).toEqual([0x21, 0x60])
    expect(status(w)).toMatch(/^Row 1, column 2,/)
    expect(w.emitted('update:rows')).toHaveLength(2)
    await press(w, ctrl('z'))
    expect(w.props('rows')).toBe(before)
    expect(status(w)).toMatch(/^Row 1, column 0,/)
    await press(w, ctrl('z'))
    expect(w.props('rows')).toBe(before) // nothing more to undo
  })

  test('pressing a set sixel clears for the whole drag', async () => {
    const w = mountEditor({ rows: rowsOf({ 1: '\x17\x21\x7f\x20' }), draw: true })
    await mouse(w, 'mousedown', cellAt(w, 1, 1), ...TL)
    expect(v(w, 1)).toBe(0x20)
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 3), ...TL) // already clear: stays clear
    expect([v(w, 2), v(w, 3)]).toEqual([0x7e, 0x20])
  })

  test('the first paintable sixel decides when the press is on an alpha cell', async () => {
    const w = mountEditor({ rows: mosaicRows(), draw: true })
    await mouse(w, 'mousedown', cellAt(w, 2, 0), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 1), ...TL)
    expect(v(w, 1)).toBe(0x21)
    expect(w.props('rows')[2]).toBe('ALPHA'.padEnd(40, ' '))
  })

  test('a drag over a control code, an alpha cell or row 0 changes nothing', async () => {
    const w = mountEditor({ rows: mosaicRows(), draw: true })
    await mouse(w, 'mousedown', cellAt(w, 1, 0), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 5), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 2, 1), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 0, 1), ...TL)
    expect(w.emitted('update:rows')).toBeUndefined()
  })

  test('a mousemove with no button down ends the drag', async () => {
    const w = mountEditor({ rows: mosaicRows(), draw: true })
    await mouse(w, 'mousedown', cellAt(w, 1, 1), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...TL, 0)
    await mouse(w, 'mousemove', cellAt(w, 1, 3), ...TL) // the button reads as down again, but no drag is running
    expect([v(w, 1), v(w, 2), v(w, 3)]).toEqual([0x21, 0x20, 0x20])
  })

  test('only the main button starts a drag', async () => {
    const w = mountEditor({ rows: mosaicRows(), draw: true })
    cellAt(w, 1, 1).element.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 2, buttons: 2, clientX: 2, clientY: 3 }))
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...TL)
    expect(w.emitted('update:rows')).toBeUndefined()
  })

  test('with Draw off the mouse only moves the cursor', async () => {
    const w = mountEditor({ rows: mosaicRows() })
    const down = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0, buttons: 1, clientX: 2, clientY: 3 })
    cellAt(w, 1, 1).element.dispatchEvent(down)
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...TL)
    await cellAt(w, 1, 2).trigger('click')
    expect(down.defaultPrevented).toBe(true) // Draw off selects by drag (T-039)
    expect(w.emitted('update:rows')).toBeUndefined()
    expect(status(w)).toMatch(/^Row 1, column 2,/)
  })

  test('when disabled, nothing is emitted and nothing is focused', async () => {
    const w = mount(PageEditor, { props: { rows: mosaicRows(), showCodes: false, showSlots: false, grid: false, draw: true, disabled: true }, attachTo: document.body })
    await mouse(w, 'mousedown', cellAt(w, 1, 1), ...TL)
    await mouse(w, 'mousemove', cellAt(w, 1, 2), ...TL)
    expect(w.emitted('update:rows')).toBeUndefined()
    expect(document.activeElement).toBe(document.body)
    w.unmount()
  })

  test('a press focuses the page and drops a pending Esc', async () => {
    const w = mount(PageEditor, { props: { rows: mosaicRows(), showCodes: false, showSlots: false, grid: false, draw: true, disabled: false }, attachTo: document.body })
    await press(w, { key: 'Escape' })
    expect(status(w)).toContain('Code…')
    await mouse(w, 'mousedown', cellAt(w, 1, 1), ...TL)
    expect(status(w)).not.toContain('Code…')
    expect(document.activeElement).toBe(w.find('[role=application]').element)
    w.unmount()
  })

  test('pressing the lower half of a double-height mosaic paints the row above', async () => {
    const w = mountEditor({ rows: rowsOf({ 3: '\x0d\x17\x20' }), draw: true })
    await mouse(w, 'mousedown', cellAt(w, 4, 2), ...BR)
    expect(w.props('rows')[3].charCodeAt(2)).toBe(0x60)
    expect(status(w)).toMatch(/^Row 3, column 2,/)
  })
})

// Block selection, copy, cut and block paste (T-039).
describe('block selection', () => {
  const BLOCK = 'application/x-ceems-block'
  const shift = (key: string) => ({ key, shiftKey: true })
  const overlay = (w: W) => w.find<HTMLElement>('.selection')
  // Row 1 holds a colour code at column 2.
  const mount0 = (props: { rows?: string[]; draw?: boolean; lastRow?: number } = {}) =>
    mount(PageEditor, { props: { rows: rowsOf({ 1: 'AB\x01C', 2: 'DEFG' }), showCodes: false, showSlots: false, grid: false, draw: false, disabled: false, ...props }, attachTo: document.body })
  const clip = (w: W, type: 'copy' | 'cut' | 'paste', data: Record<string, string> = {}) => {
    const set = vi.fn()
    const ev = new Event(type, { cancelable: true, bubbles: true }) as Event & { clipboardData: unknown }
    ev.clipboardData = { types: Object.keys(data), getData: (t: string) => data[t] ?? '', setData: set }
    w.find('[role=application]').element.dispatchEvent(ev)
    return { ev, set }
  }
  // Columns 0-3 of rows 1-2.
  const select = async (w: W) => {
    for (const k of ['ArrowRight', 'ArrowRight', 'ArrowRight', 'ArrowDown']) await press(w, shift(k))
  }

  test('Shift+arrows show the overlay and the status; a plain arrow hides them', async () => {
    const w = mountEditor()
    await press(w, shift('ArrowRight'))
    await press(w, shift('ArrowRight'))
    const style = overlay(w).element.style
    expect([style.left, style.top, style.width, style.height]).toEqual(['calc(0%)', 'calc(4%)', 'calc(7.5%)', 'calc(4%)'])
    expect(status(w)).toMatch(/, 1 rows by 3 columns selected$/)
    await press(w, { key: 'ArrowLeft' })
    expect(overlay(w).exists()).toBe(false)
    expect(status(w)).not.toContain('selected')
  })

  test('Ctrl+Z clears the overlay', async () => {
    const w = mountEditor()
    await press(w, shift('ArrowRight'))
    expect(overlay(w).exists()).toBe(true)
    await press(w, ctrl('z'))
    expect(overlay(w).exists()).toBe(false)
  })

  describe('mouse, Draw off', () => {
    const mouse = async (w: W, type: string, target: ReturnType<typeof cellAt>, buttons = 1) => {
      const ev = new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, buttons })
      target.element.dispatchEvent(ev)
      await w.vm.$nextTick()
      return ev
    }

    test('a drag selects a rectangle without editing; a later press clears it', async () => {
      const w = mount0()
      const down = await mouse(w, 'mousedown', cellAt(w, 1, 1))
      expect(down.defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(w.find('[role=application]').element)
      await mouse(w, 'mousemove', cellAt(w, 2, 3))
      await mouse(w, 'mouseup', cellAt(w, 2, 3), 0)
      expect(status(w)).toMatch(/^Row 2, column 3,.*, 2 rows by 3 columns selected$/)
      expect(overlay(w).element.style.left).toBe('calc(2.5%)')
      expect(overlay(w).element.style.top).toBe('calc(4%)')
      expect(w.emitted('update:rows')).toBeUndefined()
      await mouse(w, 'mousedown', cellAt(w, 5, 5))
      expect(overlay(w).exists()).toBe(false)
      w.unmount()
    })

    test('a press and release on one cell makes no selection', async () => {
      const w = mount0()
      await mouse(w, 'mousedown', cellAt(w, 1, 1))
      await mouse(w, 'mousemove', cellAt(w, 1, 1))
      await mouse(w, 'mouseup', cellAt(w, 1, 1), 0)
      expect(overlay(w).exists()).toBe(false)
      expect(status(w)).toMatch(/^Row 1, column 1,/)
      w.unmount()
    })

    test('with Draw on a drag never selects', async () => {
      const w = mount0({ draw: true })
      await mouse(w, 'mousedown', cellAt(w, 1, 1))
      await mouse(w, 'mousemove', cellAt(w, 2, 3))
      expect(overlay(w).exists()).toBe(false)
      w.unmount()
    })
  })

  test('copy puts the Control Pictures text on the clipboard twice, prevents the default and keeps the selection', async () => {
    const w = mount0()
    await select(w)
    const { ev, set } = clip(w, 'copy')
    expect(set.mock.calls).toEqual([
      ['text/plain', 'AB␁C\nDEFG'],
      [BLOCK, 'AB␁C\nDEFG'],
    ])
    expect(ev.defaultPrevented).toBe(true)
    expect(w.emitted('update:rows')).toBeUndefined()
    expect(overlay(w).exists()).toBe(true)
    w.unmount()
  })

  test('copy without a selection is left to the browser', () => {
    const w = mount0()
    const { ev, set } = clip(w, 'copy')
    expect(ev.defaultPrevented).toBe(false)
    expect(set).not.toHaveBeenCalled()
    w.unmount()
  })

  test('cut writes the clipboard, blanks the block in one change and one Ctrl+Z restores it', async () => {
    const w = mount0()
    const before = w.props('rows')
    await select(w)
    const { ev, set } = clip(w, 'cut')
    expect(ev.defaultPrevented).toBe(true)
    expect(set).toHaveBeenCalledTimes(2)
    const sent = w.emitted<[string[]]>('update:rows')!
    expect(sent).toHaveLength(1)
    expect(sent[0][0][1]).toBe(' '.repeat(40))
    expect(sent[0][0][2]).toBe(' '.repeat(40))
    await w.setProps({ rows: sent[0][0] })
    expect(overlay(w).exists()).toBe(false)
    await press(w, ctrl('z'))
    expect(w.props('rows')).toBe(before)
    w.unmount()
  })

  test('a block paste writes a rectangle at the cursor with its codes, ignoring text/plain, in one undo step', async () => {
    const w = mount0({ rows: rowsOf() })
    const before = w.props('rows')
    const { ev } = clip(w, 'paste', { [BLOCK]: 'A␁B\nCD', 'text/plain': 'ignored' })
    expect(ev.defaultPrevented).toBe(true)
    const sent = w.emitted<[string[]]>('update:rows')!
    expect(sent).toHaveLength(1)
    expect(sent[0][0][1].slice(0, 4)).toBe('A\x01B ')
    expect(sent[0][0][2].slice(0, 3)).toBe('CD ')
    await w.setProps({ rows: sent[0][0] })
    await press(w, ctrl('z'))
    expect(w.props('rows')).toBe(before)
    w.unmount()
  })

  test('copy and cut on a disabled editor do nothing', () => {
    const w = mount(PageEditor, { props: { rows: rowsOf({ 1: 'AB' }), showCodes: false, showSlots: false, grid: false, draw: false, disabled: true } })
    for (const type of ['copy', 'cut'] as const) {
      const { ev, set } = clip(w, type)
      expect(ev.defaultPrevented).toBe(false)
      expect(set).not.toHaveBeenCalled()
    }
    expect(w.emitted('update:rows')).toBeUndefined()
  })
})

test('with lastRow 23 a click on row 24 is ignored', async () => {
  const w = mountEditor({ lastRow: 23 })
  await cellAt(w, 3, 7).trigger('click')
  await cellAt(w, 24, 2).trigger('click')
  expect(status(w)).toMatch(/^Row 3, column 7,/)
})
