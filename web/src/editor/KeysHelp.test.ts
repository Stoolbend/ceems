import { mount } from '@vue/test-utils'
import { expect, test } from 'vitest'
import KeysHelp from './KeysHelp.vue'
import { PALETTE } from './cells'

test('the Keys panel lists all 27 codes with their keys', () => {
  const w = mount(KeysHelp)
  const rows = w.findAll('details.keys tr')
  expect(rows).toHaveLength(27)
  for (const p of PALETTE) expect(rows.some((r) => r.text().includes(p.name) && r.find('kbd:nth-of-type(2)').text() === p.key)).toBe(true)
})

test('the Keys panel lists undo, redo and paste', () => {
  const t = mount(KeysHelp).find('details.keys').text()
  expect(t).toContain('Ctrl+Z undoes')
  expect(t).toContain('Ctrl+Y or Ctrl+Shift+Z redoes')
  expect(t).toContain('Shift+Insert pastes')
  expect(t).toContain('Ctrl+C')
  expect(t).toContain('Ctrl+X')
})

test('the Keys panel carries no font credit', () => {
  expect(mount(KeysHelp).find('details.keys').text()).not.toContain('VileR')
})
