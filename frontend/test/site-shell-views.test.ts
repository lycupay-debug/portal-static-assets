/** Native public-shell tests for navigation state, menu actions and legal footer links. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountSiteHeaderView } from '../src/views/siteHeaderView'
import { mountSiteFooterView } from '../src/views/siteFooterView'

test('site header renders public links and delegates menu/auth navigation', () => {
  const dom = new JSDOM('<header id="host"></header>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let menuOpen = false
  const view = mountSiteHeaderView(host, { getState: () => ({ menuOpen, loggedIn: true }), navigate: target => calls.push(`navigate:${target}`), setMenuOpen: value => { menuOpen = value; calls.push(`menu:${value}`) } })
  assert.equal(host.querySelector('.brand-copy strong')?.textContent, '资源平台')
  assert.equal(host.querySelector('.main-nav a.nav-auth')?.textContent, '个人中心')
  host.querySelector<HTMLButtonElement>('.menu-toggle')!.click()
  assert.deepEqual(calls, ['menu:true'])
  host.querySelector<HTMLAnchorElement>('.main-nav a[href="/knowledge"]')!.click()
  host.querySelector<HTMLAnchorElement>('.main-nav a.nav-auth')!.click()
  assert.ok(calls.includes('navigate:/knowledge'))
  assert.ok(calls.includes('navigate:/account'))
  view.update({ menuOpen: true, loggedIn: false })
  assert.equal(host.querySelector('.main-nav')?.classList.contains('is-open'), true)
  assert.equal(host.querySelector('.main-nav a.nav-auth')?.textContent, '登录')
  view.dispose(); dom.window.close()
})

test('site footer renders only published legal documents and delegates encoded links', () => {
  const dom = new JSDOM('<footer id="host"></footer>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountSiteFooterView(host, { navigate: target => calls.push(target) })
  view.update([{ slug: 'privacy-policy', title: '隐私政策' }, { slug: 'terms-of-use', title: '服务条款' }])
  assert.equal(host.querySelectorAll('.site-footer-policies a').length, 2)
  host.querySelector<HTMLAnchorElement>('.site-footer-policies a')!.click()
  assert.deepEqual(calls, ['/legal/privacy-policy'])
  view.update([])
  assert.equal(host.querySelector('.site-footer-policies')?.hidden, true)
  view.dispose(); dom.window.close()
})
