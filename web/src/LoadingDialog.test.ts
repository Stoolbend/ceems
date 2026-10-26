import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { nextTick } from 'vue'
import { pending } from './api'
import LoadingDialog from './LoadingDialog.vue'

const proto = HTMLDialogElement.prototype
const real = { showModal: proto.showModal, close: proto.close }

beforeEach(() => {
  vi.useFakeTimers()
  proto.showModal = function (this: HTMLDialogElement) { this.setAttribute('open', '') }
  proto.close = function (this: HTMLDialogElement) { this.removeAttribute('open') }
})
afterEach(async () => {
  vi.useRealTimers()
  pending.value = 0
  await nextTick()
  proto.showModal = real.showModal
  proto.close = real.close
})

const isOpen = (w: ReturnType<typeof mount>) => w.find('dialog').attributes('open') !== undefined

test('opens only after pending has stayed above zero for 300 ms, closes when it returns to 0', async () => {
  const w = mount(LoadingDialog)
  pending.value = 1
  await nextTick()
  vi.advanceTimersByTime(299)
  expect(isOpen(w)).toBe(false)
  vi.advanceTimersByTime(1)
  expect(isOpen(w)).toBe(true)
  pending.value = 0
  await nextTick()
  await nextTick()
  expect(isOpen(w)).toBe(false)
})

test('a request shorter than 300 ms never opens it', async () => {
  const w = mount(LoadingDialog)
  pending.value = 1
  await nextTick()
  vi.advanceTimersByTime(200)
  pending.value = 0
  await nextTick()
  await nextTick()
  vi.advanceTimersByTime(1000)
  expect(isOpen(w)).toBe(false)
})

test('Esc (cancel) does not close it', async () => {
  const w = mount(LoadingDialog)
  pending.value = 1
  await nextTick()
  vi.advanceTimersByTime(300)
  const ev = new Event('cancel', { cancelable: true })
  w.find('dialog').element.dispatchEvent(ev)
  expect(ev.defaultPrevented).toBe(true)
  expect(isOpen(w)).toBe(true)
})

test('the text is a polite status and the bar is hidden from assistive tech', () => {
  const w = mount(LoadingDialog)
  const s = w.find('[role="status"]')
  expect(s.text()).toBe('Loading…')
  expect(s.attributes('aria-live')).toBe('polite')
  expect(w.find('.bar').attributes('aria-hidden')).toBe('true')
})
