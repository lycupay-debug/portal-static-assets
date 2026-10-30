/** Native account-sidebar tests for active states, group expansion and delegated navigation. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountAccountSidebarView } from '../src/views/accountSidebarView'
import { accountGroups } from '../src/navigation/accountNavigation'

test('account sidebar renders permission-filtered groups and delegates navigation', () => {
  const dom = new JSDOM('<aside id="host"></aside>', { url: 'https://portal.example.test/account' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let expanded = ['personal']
  const groups = accountGroups.filter(group => group.id !== 'compliance' && group.id !== 'system-ops')
  const view = mountAccountSidebarView(host, {
    getState: () => ({ groups, active: 'overview', expanded }),
    toggleGroup: id => { calls.push(`toggle:${id}`); expanded = expanded.includes(id) ? expanded.filter(value => value !== id) : [...expanded, id] },
    selectSection: id => calls.push(`select:${id}`),
    navigate: target => calls.push(`navigate:${target}`),
  })
  assert.equal(host.querySelectorAll('.account-nav-group').length, groups.length)
  assert.equal(host.querySelector('.account-sidebar-item.is-active')?.textContent, '账户概览')
  assert.equal(host.querySelector('.account-sidebar-collapse')?.style.display, '')
  host.querySelector<HTMLButtonElement>('.account-group-toggle')!.click()
  host.querySelectorAll<HTMLButtonElement>('.account-sidebar-subitem')[3]!.click()
  host.querySelector<HTMLAnchorElement>('.account-sidebar-home')!.click()
  assert.deepEqual(calls, ['toggle:personal', 'select:content-management', 'navigate:/'])
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})
