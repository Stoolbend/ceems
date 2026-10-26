// The row 24 a fastext group writes (the store's rule, T-064): four 10-cell fields, each the colour code
// (red, green, yellow, cyan) then 9 cells. A paged group's Prev and Next slots hold fixed text.
const COLOUR_CODES = [0x01, 0x02, 0x03, 0x06]

/** The row as a middle member of the group shows it. texts are the four link texts in key order. */
export function row24(texts: readonly string[], prevSlot: number | null, nextSlot: number | null): string {
  return COLOUR_CODES.map((code, k) => {
    const t = k === prevSlot ? ' [ Prev  ' : k === nextSlot ? '  Next ] ' : (texts[k] ?? '')
    return String.fromCharCode(code) + t.slice(0, 9).padEnd(9, ' ')
  }).join('')
}
