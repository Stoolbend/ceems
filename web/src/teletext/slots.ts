// Mirrors internal/tmpl/tmpl.go slotRE: a slot's width is its match length, braces included (ADR 0002).
const SLOT_RE = /\{[\^>]?[a-z][a-z0-9_-]*\.*\}/g

export type Slot = { row: number; col: number; width: number }

/** Slot tokens on rows 1-24 of a page of decoded cells, in row then column order. */
export function slots(rows: readonly string[]): Slot[] {
  const out: Slot[] = []
  for (let r = 1; r <= 24; r++) for (const m of (rows[r] ?? '').matchAll(SLOT_RE)) out.push({ row: r, col: m.index, width: m[0].length })
  return out
}
