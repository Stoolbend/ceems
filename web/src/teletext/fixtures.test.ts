import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, test } from 'vitest'
import { DEFAULT_HEADER, headerRow } from './header'
import { gridText, renderPage } from './render'
import { testPages } from './testpages'
import { subpages } from './tti'

const dir = join(__dirname, '../../../testdata/sample/pages')
const files = readdirSync(dir).filter((f) => f.endsWith('.tti')).sort()
const fixed = new Date(2026, 0, 1, 12, 34, 56)
const snap = (name: string) => `__snapshots__/${name}.txt`

test('finds 30 files and 41 subpages', () => {
  expect(files).toHaveLength(30)
  expect(files.flatMap((f) => subpages(readFileSync(join(dir, f), 'latin1')))).toHaveLength(41)
})

describe.each(files)('%s', (file) => {
  const subs = subpages(readFileSync(join(dir, file), 'latin1'))
  test.each(subs.map((s, i) => [i + 1, s] as const))('subpage %i', async (n, sub) => {
    const rows = sub.rows.map((r) => r ?? '')
    rows[0] = headerRow(DEFAULT_HEADER, sub.mpp, fixed)
    await expect(gridText(renderPage(rows))).toMatchFileSnapshot(snap(`sample/${file.replace('.tti', '')}-${n}`))
  })
})

describe('control-code test pages', () => {
  test.each(Object.entries(testPages))('%s', async (name, rows) => {
    await expect(gridText(renderPage(rows))).toMatchFileSnapshot(snap(`case-${name}`))
  })
})
