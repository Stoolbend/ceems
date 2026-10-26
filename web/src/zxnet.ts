// Codec for the zxnet teletext editor URL fragment (evidence Q-018, D-033, D-034).
export const EDITOR = 'https://zxnet.co.uk/teletext/editor/#'

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
const PAGE_NUMBER = /^[1-8][0-9A-Fa-f]{2}$/

/** rows: 25 strings of exactly 40 cells (code units 0-127). mpp: three hex digits. links: six FL entries ('' or invalid = none). */
export function encode(rows: string[], mpp: string, links: string[]): string {
  let data = ''
  let acc = 0
  let n = 0 // bits waiting in acc
  for (const row of rows) {
    for (let col = 0; col < 40; col++) {
      acc = (acc << 7) | (row.charCodeAt(col) & 0x7f)
      n += 7
      while (n >= 6) {
        data += ALPHABET[(acc >> (n - 6)) & 63]
        n -= 6
      }
      acc &= (1 << n) - 1
    }
  }
  if (n > 0) data += ALPHABET[(acc << (6 - n)) & 63]
  const x270 = links.map((l) => (PAGE_NUMBER.test(l) ? l.toUpperCase() : '8FF') + '3F7F').join('') + 'F'
  return `${EDITOR}0:${data}:PN=${mpp.toUpperCase()}:X270=${x270}`
}

/** Accepts a full URL or a bare fragment, with or without '#'. */
export function decode(text: string): { rows: string[]; pn: string | null } | Error {
  let frag = text.trim()
  const hash = frag.indexOf('#')
  if (hash >= 0) frag = frag.slice(hash + 1)
  const parts = frag.split(':')
  if (parts.length < 2 || !/^[0-9]$/.test(parts[0])) return new Error('not a zxnet editor URL (expected meta:pagedata)')
  const data = parts[1].replace(/=+$/, '')
  if (/[^A-Za-z0-9_-]/.test(data)) return new Error('page data has a character outside A-Z a-z 0-9 - _')
  if (data.length !== 1120 && data.length !== 1167) return new Error(`page data must be 1120 or 1167 characters, got ${data.length}`)

  const cells: number[] = []
  let acc = 0
  let n = 0
  for (const ch of data) {
    acc = (acc << 6) | ALPHABET.indexOf(ch)
    n += 6
    if (n >= 7) {
      cells.push((acc >> (n - 7)) & 0x7f)
      n -= 7
      acc &= (1 << n) - 1
    }
  }
  const rows = Array.from({ length: 25 }, (_, r) =>
    r * 40 < cells.length ? String.fromCharCode(...cells.slice(r * 40, r * 40 + 40)) : ' '.repeat(40),
  )
  const pn = parts.slice(2).find((p) => p.startsWith('PN='))?.slice(3) ?? ''
  return { rows, pn: PAGE_NUMBER.test(pn) ? pn.toUpperCase() : null }
}
