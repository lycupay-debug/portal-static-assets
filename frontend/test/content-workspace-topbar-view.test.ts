/** Native DOM tests for content workspace topbar navigation and status rendering. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountContentWorkspaceTopbarView } from '../src/views/contentWorkspaceTopbarView'

test('renders a safe document title, updates busy state and delegates return navigation', () => {
  const dom = new JSDOM('<!doctype html><header id="host"></header>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let state = { title: '<script>alert(1)</script>', busy: false }
  const view = mountContentWorkspaceTopbarView(host, { getState: () => state, returnToList: () => calls.push('return'), cancel: () => calls.push('cancel') })
  try {
    assert.equal(host.querySelector('.workspace-document-name > span:nth-child(2)')?.textContent, '<script>alert(1)</script>')
    assert.equal(host.querySelectorAll('script').length, 0)
    state = { title: 'Updated draft', busy: true }; view.update(state)
    assert.equal(host.querySelector('.workspace-document-name > span:nth-child(2)')?.textContent, 'Updated draft')
    assert.equal(host.querySelector<HTMLButtonElement>('.workspace-return')?.disabled, true)
    host.querySelector<HTMLAnchorElement>('.workspace-brand')!.click()
    state = { title: 'Updated draft', busy: false }; view.update(state)
    host.querySelector<HTMLButtonElement>('.workspace-return')!.click()
    assert.deepEqual(calls, ['return', 'cancel'])
  } finally { view.dispose(); assert.equal(host.childElementCount, 0); dom.window.close() }
})
