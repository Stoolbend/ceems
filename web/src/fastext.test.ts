import { expect, test } from 'vitest'
import { row24 } from './fastext'

test('the D-066 example: a paged middle member', () => {
  expect(row24(['Index', '', '', 'Art index'], 1, 2)).toBe('\x01Index    \x02 [ Prev  \x03  Next ] \x06Art index')
})

test('a static group is the four texts padded to 9', () => {
  expect(row24(['a', 'bb', '', 'c'], null, null)).toBe('\x01a        \x02bb       \x03         \x06c        ')
  expect(row24(['a', 'b', 'c', 'd'], null, null)).toHaveLength(40)
})

test('a text over 9 cells is cut to 9', () => {
  expect(row24(['0123456789', '', '', ''], null, null).slice(0, 10)).toBe('\x01012345678')
})
