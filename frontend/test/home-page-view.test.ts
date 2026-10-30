/** Native landing-page view tests for local search, empty state, anchors and safe category rendering. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountHomePageView } from '../src/views/homePageView'
import { homeSections } from '../src/config/homeSections'

test('home view renders the portal sections and filters category cards by search', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const view = mountHomePageView(host, homeSections, () => undefined)
  assert.equal(host.querySelector('main#top') !== null, true)
  assert.equal(host.querySelectorAll('.section-card').length, 4)
  const search = host.querySelector<HTMLInputElement>('input[name="portalSearch"]')!
  search.value = '工具'
  search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.equal(host.querySelectorAll('.section-card').length, 1)
  assert.equal(host.querySelector('.section-card h3')?.textContent, '在线工具')
  search.value = 'not-found'
  search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.equal(host.querySelector('.empty-search')?.hidden, false)
  host.querySelector<HTMLButtonElement>('.empty-search button')!.click()
  assert.equal(search.value, '')
  assert.equal(host.querySelectorAll('.section-card').length, 4)
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('home anchor links delegate navigation without forcing a document reload', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountHomePageView(host, homeSections, target => calls.push(target))
  host.querySelector<HTMLAnchorElement>('.hero-actions a[href="/#explore"]')!.click()
  host.querySelector<HTMLAnchorElement>('.hero-actions a[href="/#about"]')!.click()
  assert.deepEqual(calls, ['/#explore', '/#about'])
  view.dispose()
  dom.window.close()
})
