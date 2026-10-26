import { expect, test } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// vitest empties css imports (even ?raw), so the file is read directly.
const css = readFileSync(join(__dirname, 'theme.css'), 'utf8')
// The teletext preview inherits from every element above it, so the theme must not touch it (D-049, T-044).
const rules = css
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .split('}')
  .map((r) => r.split('{'))
  .filter((p) => p.length === 2)
  .map(([sel, body]) => ({ selectors: sel.split(',').map((s) => s.trim()), body: body.trim() }))
  .filter((r) => !r.selectors[0].startsWith('@'))
const selectorText = rules.flatMap((r) => r.selectors).join('\n')

test('no selector targets bare i, b or span', () => {
  expect(selectorText).not.toMatch(/(^|[\s,>+~])(i|b|span)(?=$|[\s,{.:[#>+~])/m)
})

test('no selector or property names a TeletextPage class or variable', () => {
  const classes = ['tt-page', 'tt-row', 'tt-cell', 'g', 'code', 'mosaic', 'sep', 'on', 'flash', 'conceal', 'ctl', 'slot', 'show-grid', 'dh-top', 'dh-bottom', 'aspect-4-3', 'aspect-16-9', ...[...Array(8).keys()].flatMap((n) => [`fg-${n}`, `bg-${n}`])]
  for (const c of classes) expect(selectorText, c).not.toMatch(new RegExp(String.raw`\.${c}(?![\w-])`))
  expect(css).not.toMatch(/--(rows|cols|sx)\b/)
})

const allowedStart = /^(h[1-6]|th|td|legend|kbd|code|a|small|summary|input|select|textarea|::placeholder|::file-selector-button)(?![\w-])/
const inheritable = /(^|;)\s*(text-transform|letter-spacing|word-spacing|text-shadow|font-weight|font-style|font-variant[\w-]*|font-feature-settings|font-synthesis|-webkit-font-smoothing|-moz-osx-font-smoothing|font)\s*:\s*([^;]*)/g

test('inheritable text properties appear only on selectors that cannot sit above .tt-page', () => {
  for (const r of rules) {
    for (const m of r.body.matchAll(inheritable)) {
      if (m[2] === 'font' && m[3].trim() === 'inherit') continue
      for (const s of r.selectors) expect(s, `${m[2]} in "${r.selectors.join(', ')}"`).toMatch(allowedStart)
    }
  }
})

test('the box-sizing reset is present and there is no prefers-color-scheme query', () => {
  expect(css).toMatch(/\*, \*::before, \*::after \{ box-sizing: border-box; \}/)
  expect(css).not.toMatch(/prefers-color-scheme/)
})

test('the only at-rule is @font-face, so no rule hides inside a block the checks above skip', () => {
  expect(css.replace(/\/\*[\s\S]*?\*\//g, '').match(/@[a-z-]+/g)).toEqual(['@font-face'])
})

test('the field is its own well, distinct from buttons and the desk, with an edge and a footer height', () => {
  const root = css.match(/:root \{([^}]*)\}/)![1]
  const token = (n: string) => root.match(new RegExp(String.raw`${n}\s*:\s*([^;]+);`))?.[1].trim()
  expect(token('--field-bg')).toBeDefined()
  expect(token('--field-bg')).not.toBe(token('--bar'))
  expect(token('--field-bg')).not.toBe(token('--desk'))
  expect(token('--field-edge')).toBeDefined()
  expect(token('--app-footer')).toBeDefined()
  expect(token('--warning-bg')).toBe('#AAAA00')
  expect(token('--warning-text')).toBeDefined()
  expect(root).not.toMatch(/--dialog-field/)
})
