// Test and gallery helpers; never imported by main.ts.

// ADR 0002 cell decode: byte >= 0x80 gives b & 0x7F; ESC x gives x - 0x40.
export function decodeRow(latin1: string): string {
  let out = ''
  for (let i = 0; i < latin1.length; i++) {
    const b = latin1.charCodeAt(i)
    if (b === 0x1b && i + 1 < latin1.length) out += String.fromCharCode((latin1.charCodeAt(++i) - 0x40) & 0x7f)
    else out += String.fromCharCode(b & 0x7f)
  }
  return out
}

export interface Subpage {
  mpp: string
  rows: (string | undefined)[]
}

export function subpages(latin1: string): Subpage[] {
  const out: Subpage[] = []
  for (const raw of latin1.split('\n')) {
    const line = raw.endsWith('\r') ? raw.slice(0, -1) : raw
    if (line.startsWith('PN,')) {
      out.push({ mpp: line.slice(3, 6).toUpperCase(), rows: [] })
    } else if (line.startsWith('OL,')) {
      const cur = out[out.length - 1]
      if (!cur) throw new Error('OL line before the first PN line')
      const comma = line.indexOf(',', 3)
      const r = Number(line.slice(3, comma))
      if (r <= 24) cur.rows[r] = decodeRow(line.slice(comma + 1))
    }
  }
  return out
}
