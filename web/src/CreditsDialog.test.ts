import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, expect, test } from 'vitest'
import CreditsDialog from './CreditsDialog.vue'

const proto = HTMLDialogElement.prototype
const real = { showModal: proto.showModal, close: proto.close }

beforeEach(() => {
  proto.showModal = function (this: HTMLDialogElement) { this.setAttribute('open', '') }
  proto.close = function (this: HTMLDialogElement) { this.removeAttribute('open'); this.dispatchEvent(new Event('close')) }
})
afterEach(() => { proto.showModal = real.showModal; proto.close = real.close })

test('the dialog starts closed and open() shows it', async () => {
  const w = mount(CreditsDialog)
  const d = w.find('dialog')
  expect(d.attributes('open')).toBeUndefined()
  ;(w.vm as unknown as { open: () => void }).open()
  expect(d.attributes('open')).toBeDefined()
})

test('it shows the copyright, the AGPL sentence and the licence links', () => {
  const w = mount(CreditsDialog)
  expect(w.text()).toContain('Copyright © 2026 Conor Morgan and CeeMS contributors')
  expect(w.text()).toContain('CeeMS is free software under the GNU Affero General Public License, version 3 or later. You may redistribute it under the terms of that license. It comes with no warranty.')
  const links = w.findAll('a')
  for (const [text, href] of [['License', '/LICENSE.txt'], ['github.com/Stoolbend/ceems', 'https://github.com/Stoolbend/ceems'], ['Full license texts', '/THIRD_PARTY_NOTICES.txt']]) {
    expect(links.find((a) => a.text() === text)?.attributes('href')).toBe(href)
  }
})

test('it credits both fonts with their links', () => {
  const w = mount(CreditsDialog)
  for (const s of ['VileR', 'Alan Davies', 'Ben Harris']) expect(w.text()).toContain(s)
  expect(w.findAll('a').map((a) => a.attributes('href'))).toEqual(expect.arrayContaining([
    'https://int10h.org/oldschool-pc-fonts/',
    'https://creativecommons.org/licenses/by-sa/4.0/',
    'https://galax.xyz/Teletext50/',
    'http://bjh21.me.uk/bedstead/',
  ]))
  for (const a of w.findAll('a')) {
    expect(a.attributes('target')).toBe('_blank')
    expect(a.attributes('rel')).toBe('noopener')
  }
})

test('the close button is first in focus order and closes the dialog', async () => {
  const w = mount(CreditsDialog)
  expect(w.findAll('button, a')[0].attributes('aria-label')).toBe('Close')
  expect(w.find('button[aria-label="Close"]').text()).toBe('[X]')
  ;(w.vm as unknown as { open: () => void }).open()
  await w.find('button[aria-label="Close"]').trigger('click')
  expect(w.find('dialog').attributes('open')).toBeUndefined()
})

test('the dialog is labelled by its Credits heading', () => {
  const w = mount(CreditsDialog)
  const id = w.find('dialog').attributes('aria-labelledby')!
  const h = w.find(`#${id}`)
  expect(h.element.tagName).toBe('H2')
  expect(h.text()).toBe('Credits')
})
