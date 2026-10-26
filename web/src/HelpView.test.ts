import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { expect, test } from 'vitest'
import HelpView from './HelpView.vue'
import { routes } from './router'

const IDS = ['overview', 'walkthrough', 'templates', 'bindings', 'errors', 'limits']

async function mountView() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: HelpView }] })
  router.push('/')
  await router.isReady()
  return mount(HelpView, { global: { plugins: [router] } })
}

test('the h1 is Help', async () => {
  expect((await mountView()).find('h1').text()).toBe('Help')
})

test('the six sections exist and the contents list links to each', async () => {
  const wrapper = await mountView()
  expect(wrapper.findAll('h2').map((h) => h.attributes('id'))).toEqual(IDS)
  const links = wrapper.find('ul').findAll('a').map((a) => a.attributes('href'))
  expect(links).toEqual(IDS.map((id) => `#${id}`))
})

test('the walkthrough gives the example columns and range, and the slot toggle is named', async () => {
  const wrapper = await mountView()
  const walk = wrapper.find('ol').text() // the walkthrough is the page's only ordered list
  expect(walk).toContain('p#name, p#score')
  expect(walk).toContain('A2:B')
  expect(wrapper.text()).toContain('Show template slots')
})

test('/help resolves to HelpView', () => {
  expect(routes.find((r) => r.path === '/help')?.component).toBe(HelpView)
})
