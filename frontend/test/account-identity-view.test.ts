/** Native account identity tests for the overview/profile/security variants and logout delegation. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountAccountIdentityView, type AccountIdentityState } from '../src/views/accountIdentityView'

test('account overview and profile render user data as text without exposing logout on profile', () => {
  const dom = new JSDOM('<div id="host"></div>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  let state: AccountIdentityState = { section: 'overview', loading: false, busy: false, user: { email: 'user@example.test', displayName: '<script>bad()</script>', createdAt: '2026-01-01T00:00:00Z' } }
  const calls: string[] = []
  const view = mountAccountIdentityView(host, { getState: () => state, logout: () => calls.push('logout') })
  assert.equal(host.querySelector('h1')?.textContent, '账户概览')
  assert.equal(host.querySelector('.account-summary-name')?.textContent, '<script>bad()</script>')
  assert.equal(host.querySelector('.account-summary-name script'), null)
  assert.equal(host.querySelector('.account-status')?.textContent, ' 已登录')
  host.querySelector('button')!.click()
  assert.deepEqual(calls, ['logout'])
  state = { ...state, section: 'profile' }
  view.update(state)
  assert.equal(host.querySelector('h1')?.textContent, '个人资料')
  assert.equal(host.querySelector('button'), null)
  view.dispose(); dom.window.close()
})

test('security view shows session status and loading state remains explicit', () => {
  const dom = new JSDOM('<div id="host"></div>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  let state: AccountIdentityState = { section: 'security', loading: false, busy: true, user: { email: 'user@example.test', createdAt: '2026-01-01T00:00:00Z' } }
  const view = mountAccountIdentityView(host, { getState: () => state, logout: () => undefined })
  assert.equal(host.querySelector('.security-info-row strong')?.textContent, '账户已验证登录')
  assert.equal(host.querySelector('button')?.disabled, true)
  state = { ...state, loading: true, user: null }
  view.update(state)
  assert.equal(host.querySelector('.account-loading')?.textContent, '正在读取账户信息…')
  view.dispose(); dom.window.close()
})
