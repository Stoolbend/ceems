<script setup lang="ts">
import { computed } from 'vue'
import { glyph, renderPage, SEXTANT_BITS } from './render'
import { slots } from './slots'

const props = withDefaults(defineProps<{ rows: string[]; aspect?: '4:3' | '16:9'; rowCount?: number; colCount?: number; showCodes?: boolean; grid?: boolean; showSlots?: boolean }>(), { aspect: '4:3', rowCount: 25, colCount: 40, showCodes: false, grid: false, showSlots: false })
// Crops from row 0, column 0, so a small sample renders exactly as it would at the top of a page.
const grid = computed(() => renderPage(props.rows).slice(0, props.rowCount).map((r) => r.slice(0, props.colCount)))

type Cell = ReturnType<typeof renderPage>[number][number]

// The row below one holding 0x0d is drawn from it, so its own source is not shown (mirrors renderPage).
const belowDouble = computed(() => {
  const below: boolean[] = []
  for (let r = 1; r < props.rowCount; r++) below[r] = !below[r - 1] && !!props.rows[r - 1]?.includes('\x0d')
  return below
})

// A control cell is one whose source code is below 0x20 and which the page draws from that source.
const ctlCode = (c: Cell, r: number, i: number) => {
  const v = props.showCodes && c.height !== 'bottom' && !belowDouble.value[r] ? props.rows[r]?.charCodeAt(i) : undefined
  return v !== undefined && v < 0x20 ? String.fromCharCode(0x2400 + v) : ''
}

// Slot cells by source row; the bottom half of a double-height row reads the row above, as renderPage draws it.
const slotCells = computed(() => {
  const m: boolean[][] = []
  if (props.showSlots) for (const s of slots(props.rows)) for (let i = 0; i < s.width; i++) (m[s.row] ??= [])[s.col + i] = true
  return m
})
const inSlot = (r: number, i: number) => !!slotCells.value[belowDouble.value[r] ? r - 1 : r]?.[i]

function cellClass(c: Cell, r: number, i: number) {
  return [
    `fg-${c.fg}`,
    `bg-${c.bg}`,
    { mosaic: c.mosaic, sep: c.separated, 'dh-top': c.height === 'top', 'dh-bottom': c.height === 'bottom', conceal: c.conceal, flash: c.flash, ctl: ctlCode(c, r, i) !== '', slot: inSlot(r, i) },
  ]
}
</script>

<template>
  <div class="tt-page" :class="[aspect === '16:9' ? 'aspect-16-9' : 'aspect-4-3', { 'show-grid': props.grid }]" :style="{ '--rows': rowCount, '--cols': colCount }">
    <div v-for="(row, r) in grid" :key="r" class="tt-row">
      <span v-for="(c, i) in row" :key="i" class="tt-cell" :class="cellClass(c, r, i)">
        <template v-if="c.mosaic"><i v-for="b in SEXTANT_BITS" :key="b" :class="{ on: (c.ch & (1 << b)) !== 0 }" /></template>
        <span v-else class="g">{{ inSlot(r, i) ? String.fromCharCode(c.ch) : glyph(c.ch) }}</span>
        <b v-if="ctlCode(c, r, i)" class="code">{{ ctlCode(c, r, i) }}</b>
      </span>
    </div>
  </div>
</template>

<style scoped>
@font-face { font-family: Teletext50; src: url("../assets/fonts/Teletext50.otf") format("opentype"); font-display: block; }
.tt-page { display: grid; grid-template-rows: repeat(var(--rows), 1fr); width: 100%; max-width: 960px; container-type: size; background: #000; font-family: Teletext50, monospace; line-height: 1; }
.aspect-4-3 { aspect-ratio: calc(var(--cols) / (1.2 * var(--rows))); --sx: calc(25 / 18); }
.aspect-16-9 { aspect-ratio: calc(var(--cols) / (0.9 * var(--rows))); --sx: calc(50 / 27); }
.tt-row { display: grid; grid-template-columns: repeat(var(--cols), 1fr); min-height: 0; }
/* row height is 100cqh / rows; glyph height tracks it, and scaleX stretches the glyph to the wider cell like a TV */
.tt-cell { min-width: 0; font-size: calc(100cqh / var(--rows)); line-height: calc(100cqh / var(--rows)); text-align: center; overflow: hidden; position: relative; }
.mosaic { display: grid; grid-template: repeat(3, 1fr) / repeat(2, 1fr); }
.mosaic i { background: transparent; }
.mosaic.fg-0 i.on { background: #000; } .mosaic.fg-1 i.on { background: #f00; } .mosaic.fg-2 i.on { background: #0f0; }
.mosaic.fg-3 i.on { background: #ff0; } .mosaic.fg-4 i.on { background: #00f; } .mosaic.fg-5 i.on { background: #f0f; }
.mosaic.fg-6 i.on { background: #0ff; } .mosaic.fg-7 i.on { background: #fff; }
.sep i.on { box-shadow: inset 0 0 0 1px #000; }
.fg-0 { color: #000; } .fg-1 { color: #f00; } .fg-2 { color: #0f0; } .fg-3 { color: #ff0; }
.fg-4 { color: #00f; } .fg-5 { color: #f0f; } .fg-6 { color: #0ff; } .fg-7 { color: #fff; }
.bg-0 { background-color: #000; } .bg-1 { background-color: #f00; } .bg-2 { background-color: #0f0; } .bg-3 { background-color: #ff0; }
.bg-4 { background-color: #00f; } .bg-5 { background-color: #f0f; } .bg-6 { background-color: #0ff; } .bg-7 { background-color: #fff; }
/* double height: stretch the glyph to two rows; the cell shows its top or bottom half */
.g { display: inline-block; transform: scaleX(var(--sx)); }
.dh-top .g { transform: scale(var(--sx), 2); transform-origin: top; }
.dh-bottom .g { transform: scale(var(--sx), 2); transform-origin: bottom; }
/* flash is on for 3/4 of each second; the foreground only, so the background stays */
@keyframes tt-flash { from { visibility: visible } 75% { visibility: hidden } to { visibility: visible } }
.flash .g, .flash i { animation: tt-flash 1s step-end infinite; }
@media (prefers-reduced-motion: reduce) { .flash .g, .flash i { animation: none; } }
.conceal .g, .conceal i.on { opacity: 0.35; }
/* control-code overlay: readable on any of the 8 backgrounds */
.ctl { outline: 1px dotted #888; outline-offset: -1px; }
.show-grid .tt-cell { box-shadow: inset 0 0 0 1px rgba(128,128,128,.5); }
/* slot highlight: a two-colour bar along the token's bottom edge, one line of which passes on every background */
.tt-cell.slot { box-shadow: inset 0 -2px #000, inset 0 -4px #ff0; }
.code { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; font-size: 60%; font-weight: normal; color: #fff; text-shadow: 0 0 1px #000, 0 0 1px #000, 1px 1px 0 #000, -1px -1px 0 #000; pointer-events: none; }
</style>
