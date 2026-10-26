import { mount } from '@vue/test-utils'
import { expect, test } from 'vitest'
import CellInput from './CellInput.vue'

const mountBox = (modelValue = '') => mount(CellInput, { props: { modelValue, maxlength: 9, label: 'Red text' }, attachTo: document.body })
const press = (w: ReturnType<typeof mountBox>, key: string, code?: string) => w.find('input').trigger('keydown', { key, code })

test('Esc then r inserts the red code at the caret', async () => {
  const w = mountBox('ab')
  const el = w.find<HTMLInputElement>('input').element
  el.setSelectionRange(1, 1)
  await press(w, 'Escape')
  await press(w, 'r')
  expect(el.value).toBe('a␁b') // Control Picture for 0x01
  expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['a\x01b'])
})

test('Esc then a digit is alpha, with Shift mosaic', async () => {
  const w = mountBox()
  await press(w, 'Escape')
  await press(w, '2', 'Digit2')
  await press(w, 'Escape')
  await w.find('input').trigger('keydown', { key: '!', code: 'Digit1', shiftKey: true })
  expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['\x02\x11'])
})

test('a full box refuses a code', async () => {
  const w = mountBox('123456789')
  expect(w.find('input').attributes('maxlength')).toBe('9')
  const el = w.find<HTMLInputElement>('input').element
  el.setSelectionRange(9, 9)
  await press(w, 'Escape')
  await press(w, 'r')
  expect(el.value).toBe('123456789')
})

test('a character that is not a cell reports invalid and keeps the last value', async () => {
  const w = mountBox('ab')
  await w.find('input').setValue('aé')
  expect(w.emitted('invalid')!.at(-1)).toEqual([true])
  expect(w.emitted('update:modelValue')).toBeUndefined()
  await w.find('input').setValue('ab')
  expect(w.emitted('invalid')!.at(-1)).toEqual([false])
})

test('an unknown key after Esc inserts nothing and clears the pending state', async () => {
  const w = mountBox()
  await press(w, 'Escape')
  await press(w, '?')
  await press(w, 'r')
  expect(w.find<HTMLInputElement>('input').element.value).toBe('')
})
