import { expect, test } from 'vitest'
import { DEFAULT_HEADER, headerRow, vbitHeader } from './header'

const fixed = new Date(2026, 0, 1, 12, 34, 56)

test('substitutes page and time, leaving cells 0-7 as given', () => {
  const h = headerRow(DEFAULT_HEADER, '81a', fixed)
  expect(h).toBe('        CeeMS        81A        12:34:56')
  expect(h.slice(0, 8)).toBe(DEFAULT_HEADER.slice(0, 8))
})

test('%H:%M.%S variant, zero padding and unknown escapes', () => {
  expect(headerRow('mpp %H:%M.%S %x', '100', new Date(2026, 0, 1, 1, 2, 3))).toBe('100 01:02.03 %x')
})

const SAMPLE = 'CeeMS   %%# %%a %e %%b \x03%H:%M:%S'

test('vbitHeader fills the sample template; the cyan cell and its space pass through', () => {
  const h = vbitHeader(SAMPLE, '100', new Date(2026, 9, 24, 12, 34, 56))
  expect(h).toBe('        CeeMS   100 Sat 24 Oct \x0312:34:56')
  expect(h).toHaveLength(40)
})

test('vbitHeader: %e pads day 5 with a space, %d with a zero; %m %y and a lower-case page', () => {
  const d = new Date(2026, 0, 5, 1, 2, 3)
  expect(vbitHeader('%e|%d|%m|%y|%%#|%%b', '8fa', d)).toBe('        ' + ' 5|05|01|26|8FA|Jan')
})
