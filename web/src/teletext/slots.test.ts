import { expect, test } from 'vitest'
import { slots } from './slots'

test('finds every slot token with its width, in row then column order', () => {
  const rows: string[] = Array(26).fill('')
  rows[1] = 'x{p1name.}y {^title..} {>p1score.}'
  rows[2] = '{gname' + '.'.repeat(25) + '} {a}{A}'
  rows[3] = '{1x}{b {_x} {}'
  expect(slots(rows)).toEqual([
    { row: 1, col: 1, width: 9 },
    { row: 1, col: 12, width: 10 },
    { row: 1, col: 23, width: 11 },
    { row: 2, col: 0, width: 32 },
    { row: 2, col: 33, width: 3 },
  ])
})

test('only rows 1-24 are scanned', () => {
  const rows: string[] = Array(26).fill('')
  rows[0] = '{head}'
  rows[25] = '{tail}'
  rows[3] = '{mid}'
  expect(slots(rows)).toEqual([{ row: 3, col: 0, width: 5 }])
})

test('an empty or unclosed token, or a missing row, gives none', () => {
  expect(slots(['', '{}', '{ab'])).toEqual([])
  expect(slots([])).toEqual([])
})
