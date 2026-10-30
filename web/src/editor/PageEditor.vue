<script setup lang="ts">
import { computed, ref } from 'vue'
import TeletextPage from '../teletext/TeletextPage.vue'
import { glyph, renderPage } from '../teletext/render'
import { fromEditable, PALETTE, toEditable } from './cells'
import { apply, block, cut, lowerHalf, paint, paste, pasteBlock, selection, sixelAt, sixelOn, write, type State } from './edit'
import { decode } from '../zxnet'

const props = defineProps<{ rows: string[]; showCodes: boolean; showSlots: boolean; grid: boolean; draw: boolean; disabled: boolean; lastRow?: number }>()
const emit = defineEmits<{ 'update:rows': [rows: string[]]; error: [message: string] }>()

// Marks clipboard text as a rectangle of cells, so a paste can tell it from plain text.
const BLOCK_TYPE = 'application/x-ceems-block'

const el = ref<HTMLElement | null>(null)
const anchorText = ref<HTMLElement | null>(null)
// The parent remounts the editor (a :key) when the subpage changes, which resets the cursor.
const row = ref(1)
const col = ref(0)
const insert = ref(false)
const pending = ref(false)
const anchor = ref<State['anchor']>()

const state = (): State => ({ rows: props.rows, row: row.value, col: col.value, insert: insert.value, pending: pending.value, lastRow: props.lastRow, anchor: anchor.value })
const sel = computed(() => selection(state()))

// Each entry is the page and cursor before one change. They live as long as the editor, which the parent remounts per subpage.
type Snapshot = { rows: string[]; row: number; col: number }
const MAX_UNDO = 200
const undo: Snapshot[] = []
let redo: Snapshot[] = []

// record is false for the later changes of one drag, so the drag is one undo step.
function commit(next: State, record = true) {
  if (record && next.rows !== props.rows) {
    undo.push({ rows: props.rows, row: row.value, col: col.value })
    if (undo.length > MAX_UNDO) undo.shift()
    redo = []
  }
  row.value = next.row
  col.value = next.col
  insert.value = next.insert
  pending.value = next.pending
  anchor.value = next.anchor
  if (next.rows !== props.rows) emit('update:rows', next.rows)
}

// The current page goes onto `to`, and the last entry of `from` is restored.
function step(from: Snapshot[], to: Snapshot[]) {
  pending.value = false
  anchor.value = undefined
  const prev = from.pop()
  if (!prev) return
  to.push({ rows: props.rows, row: row.value, col: col.value })
  row.value = prev.row
  col.value = prev.col
  emit('update:rows', prev.rows)
}

// The page is not text and mousedown stops text selection, so the browser has nothing to copy and may not fire copy, cut or paste.
// A selection on one hidden character gives it something; the handlers prevent the default, so that character never reaches the clipboard.
function holdSelection() {
  const s = getSelection()
  if (props.disabled || !s || !anchorText.value) return
  const r = document.createRange()
  r.selectNodeContents(anchorText.value)
  s.removeAllRanges()
  s.addRange(r)
}

function keydown(e: KeyboardEvent) {
  if (props.disabled) return
  const k = e.key.toLowerCase()
  if (((e.ctrlKey || e.metaKey) && ['c', 'x', 'v', 'insert'].includes(k)) || (e.shiftKey && ['insert', 'delete'].includes(k))) holdSelection()
  if (e.ctrlKey && !e.altKey && !e.metaKey && (k === 'z' || k === 'y')) {
    e.preventDefault()
    if (k === 'z' && !e.shiftKey) step(undo, redo)
    else step(redo, undo)
    return
  }
  const next = apply(state(), e)
  if (!next) return
  e.preventDefault()
  commit(next)
}

function pasted(e: ClipboardEvent) {
  if (props.disabled) return
  // types is checked as well because a stub that answers every getData with the same text must not look like a block
  const blockText = e.clipboardData?.types?.includes(BLOCK_TYPE) ? e.clipboardData.getData(BLOCK_TYPE) : ''
  if (blockText) {
    e.preventDefault()
    const lines = blockText.split('\n').map(fromEditable).filter((l): l is string => typeof l === 'string')
    return commit(pasteBlock(state(), lines))
  }
  const text = e.clipboardData?.getData('text/plain') ?? ''
  e.preventDefault()
  const url = text.trim()
  if (!/[\r\n]/.test(url) && /#\d:/.test(url)) {
    const d = decode(url)
    if (d instanceof Error) return emit('error', d.message)
    commit({ ...state(), rows: props.rows.map((r, i) => (i === 0 ? r : d.rows[i])) })
    emit('error', '')
  } else commit(paste(state(), text))
}

// Copy and cut put the selected block on the clipboard: Control Pictures text, once as plain text and once marked as a block.
function copied(e: ClipboardEvent, cutting = false) {
  if (props.disabled) return
  const lines = block(state())
  // nothing selected: write nothing, and do not let the browser copy the hidden character
  if (!lines || !e.clipboardData) return e.preventDefault()
  const text = lines.map(toEditable).join('\n')
  e.clipboardData.setData('text/plain', text)
  e.clipboardData.setData(BLOCK_TYPE, text)
  e.preventDefault()
  if (cutting) commit(cut(state()))
}

// The cell is found from the DOM, not from coordinates: its index in its row, and the row's index in the page.
function cellAt(e: MouseEvent): { row: number; col: number; el: Element } | undefined {
  const el = (e.target as Element).closest('.tt-cell')
  const line = el?.parentElement
  if (!el || !line?.parentElement) return undefined
  let r = Array.from(line.parentElement.children).indexOf(line)
  if (lowerHalf(props.rows, r)) r-- // the hidden lower half shows the row above
  return r < 1 || r > (props.lastRow ?? 24) ? undefined : { row: r, col: Array.from(line.children).indexOf(el), el }
}

function click(e: MouseEvent) {
  const c = cellAt(e)
  if (!c) return
  row.value = c.row
  col.value = c.col
}

// A drag paints with one mode, set by the first paintable sixel it meets; pushed is whether its undo entry exists.
let drag: { mode?: boolean; pushed: boolean } | null = null

// shortcut: a fast drag can skip cells, interpolate the line if it matters
function paintAt(e: MouseEvent) {
  const c = drag && cellAt(e)
  if (!drag || !c) return
  const r = c.el.getBoundingClientRect()
  const sixel = sixelAt((e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height)
  const was = sixelOn(props.rows, c.row, c.col, sixel, props.lastRow)
  if (was === undefined) return
  drag.mode ??= !was
  const next = paint(state(), c.row, c.col, sixel, drag.mode)
  if (next.rows === props.rows) return
  commit(next, !drag.pushed)
  drag.pushed = true
}

// With Draw off, a press and drag selects: the cell where the button went down, until the drag leaves it.
let press: { row: number; col: number } | null = null

function mousedown(e: MouseEvent) {
  if (props.disabled || e.button !== 0) return
  e.preventDefault() // no text selection; this also suppresses the native focus
  el.value?.focus()
  holdSelection() // focus() does nothing, so no focus event, when the editor is already focused
  pending.value = false
  if (props.draw) {
    drag = { pushed: false }
    paintAt(e)
    return
  }
  anchor.value = undefined
  const c = cellAt(e)
  press = c ? { row: c.row, col: c.col } : null
  if (c) {
    row.value = c.row
    col.value = c.col
  }
}

function mousemove(e: MouseEvent) {
  if (props.disabled) return
  if (props.draw) {
    if (drag && !(e.buttons & 1)) drag = null // released outside the editor
    else paintAt(e)
    return
  }
  if (!(e.buttons & 1)) press = null
  const c = press && cellAt(e)
  if (!press || !c || (!anchor.value && c.row === press.row && c.col === press.col)) return
  anchor.value ??= press
  row.value = c.row
  col.value = c.col
}

function mouseup() {
  drag = null
  press = null
}

const status = computed(() => {
  const v = props.rows[row.value].charCodeAt(col.value)
  const name = PALETTE.find((p) => p.code === v)?.name
  const what = v < 0x20 ? (name ?? `code 0x${v.toString(16).padStart(2, '0')}`) : renderPage(props.rows)[row.value][col.value].mosaic ? 'mosaic' : glyph(v)
  const r = sel.value
  const picked = r ? `, ${r.bottom - r.top + 1} rows by ${r.right - r.left + 1} columns selected` : ''
  return `Row ${row.value}, column ${col.value}, ${insert.value ? 'Insert' : 'Overwrite'}, ${what}${pending.value ? ', Code…' : ''}${picked}`
})

defineExpose({
  /** Writes one cell value at the cursor, as typing does, and returns focus to the page. */
  write(cell: number) {
    commit(write(state(), cell))
    el.value?.focus()
  },
})
</script>

<template>
  <div ref="el" class="page-editor" :tabindex="disabled ? undefined : 0" role="application" aria-label="Page editor" @focus="holdSelection" @keydown="keydown" @paste="pasted" @copy="copied($event)" @cut="copied($event, true)" @click="click" @mousedown="mousedown" @mousemove="mousemove" @mouseup="mouseup">
    <TeletextPage :rows="rows" :show-codes="showCodes" :show-slots="showSlots" :grid="grid" />
    <div v-if="sel" class="selection" :style="{ left: `calc(${sel.left} * 2.5%)`, top: `calc(${sel.top} * 4%)`, width: `calc(${sel.right - sel.left + 1} * 2.5%)`, height: `calc(${sel.bottom - sel.top + 1} * 4%)` }" />
    <div class="cursor" :style="{ left: `calc(${col} * 2.5%)`, top: `calc(${row} * 4%)` }" />
    <span ref="anchorText" class="clip-anchor" aria-hidden="true">c</span>
  </div>
  <p class="status" aria-live="polite">{{ status }}</p>
</template>

<style scoped>
.page-editor { position: relative; }
/* the cursor is placed in percentages of .page-editor, so the page must fill it exactly: lift the page's own 960px cap here */
.page-editor :deep(.tt-page) { max-width: none; }
.page-editor:focus { outline: 2px solid var(--focus); outline-offset: 2px; }
/* white under difference inverts whatever is beneath, so the block shows on all 8 colours */
/* selectable but invisible and out of flow: it only gives the browser a selection to copy */
.clip-anchor { position: absolute; left: 0; top: 0; width: 1px; height: 1px; overflow: hidden; opacity: 0; pointer-events: none; }
.cursor { position: absolute; width: 2.5%; height: 4%; background: #fff; mix-blend-mode: difference; pointer-events: none; }
.selection { position: absolute; background: rgb(255 255 85 / 0.3); outline: 1px solid var(--focus); box-shadow: inset 0 0 0 1px #000; pointer-events: none; }
/* a one-row status bar in the theme's bar colours; PageView sizes it on wide screens */
.status { color: var(--bar-text); background: var(--bar); padding: 0 1ch; margin: 1em 0 0; }
</style>
