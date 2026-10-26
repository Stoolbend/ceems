import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import PreviewDialog from './PreviewDialog.vue'
import TeletextPlayer from './teletext/TeletextPlayer.vue'
import { getPreview } from './api'

vi.mock('./api', async (importOriginal) => ({ ...(await importOriginal<typeof import('./api')>()), getPreview: vi.fn() }))

const proto = HTMLDialogElement.prototype
const real = { showModal: proto.showModal, close: proto.close }
beforeEach(() => {
  proto.showModal = function (this: HTMLDialogElement) { this.setAttribute('open', '') }
  proto.close = function (this: HTMLDialogElement) { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }
})
afterEach(() => { proto.showModal = real.showModal; proto.close = real.close })

type Open = { open: (m: string) => Promise<void> }
const preview = { mpp: '100', description: 'Index', headerTemplate: '', subpages: [{ rows: Array(25).fill(null), seconds: 8 }] }

test('it loads the page, shows its description and plays it', async () => {
  vi.mocked(getPreview).mockResolvedValue(preview)
  const w = mount(PreviewDialog)
  await (w.vm as unknown as Open).open('100')
  await flushPromises()
  expect(getPreview).toHaveBeenCalledWith('100')
  expect(w.find('dialog').attributes('open')).toBeDefined()
  expect(w.find('h2').text()).toBe('Preview 100 Index')
  expect(w.findComponent(TeletextPlayer).exists()).toBe(true)
})

test('it shows the error text when the load fails', async () => {
  vi.mocked(getPreview).mockRejectedValue(new Error('page not found'))
  const w = mount(PreviewDialog)
  await (w.vm as unknown as Open).open('100')
  await flushPromises()
  expect(w.find('[role="alert"]').text()).toBe('page not found')
  expect(w.findComponent(TeletextPlayer).exists()).toBe(false)
})

test('Close closes the dialog and unmounts the player', async () => {
  vi.mocked(getPreview).mockResolvedValue(preview)
  const w = mount(PreviewDialog)
  await (w.vm as unknown as Open).open('100')
  await flushPromises()
  await w.find('button[aria-label="Close"]').trigger('click')
  expect(w.find('dialog').attributes('open')).toBeUndefined()
  expect(w.findComponent(TeletextPlayer).exists()).toBe(false)
})
