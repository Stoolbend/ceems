import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { expect, test } from 'vitest'

const here = (p: string) => new URL(p, import.meta.url)
const css = readFileSync(here('./TeletextPage.vue'), 'utf8').split('<style scoped>')[1]

test('TeletextPage declares one Teletext50 @font-face and uses it', () => {
  expect(css.match(/@font-face/g)).toHaveLength(1)
  expect(css).toContain('font-family: Teletext50;')
  expect(css).toContain('src: url("../assets/fonts/Teletext50.otf")')
  expect(css).toContain('font-display: block')
  expect(css).toMatch(/\.tt-page \{[^}]*font-family: Teletext50, monospace/)
})

test('the vendored Teletext50.otf is unmodified', () => {
  const bytes = readFileSync(here('../assets/fonts/Teletext50.otf'))
  expect(bytes.length).toBe(128692)
  expect(createHash('sha256').update(bytes).digest('hex')).toBe('edea9c498be65310b0a59e805b7131893eef0e41d9d81d6e82d7c0c5787ea1cb')
})
