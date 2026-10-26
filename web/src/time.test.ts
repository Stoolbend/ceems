import { expect, test } from 'vitest'
import { localTime } from './time'

test('valid timestamp matches toLocaleString', () => {
  const x = '2026-01-01T12:34:56Z'
  expect(localTime(x)).toBe(new Date(x).toLocaleString())
})

test.each([null, undefined, '', 'not a date'])('%s gives the fallback', (v) => {
  expect(localTime(v)).toBe('')
  expect(localTime(v, 'never')).toBe('never')
})
