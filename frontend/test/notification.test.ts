/**
 * Notification-store and native-host regression tests.
 * Dependencies: Node's built-in test/assert modules and jsdom already present in frontend devDependencies.
 * Collaboration: verifies the framework-independent notification store and DOM host used by App.vue.
 * Boundaries: no network calls, service access, or real browser assumptions.
 */
import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { JSDOM } from 'jsdom'
import { dismissNotificationById, getNotificationSnapshot, portalNotify, subscribeNotifications } from '../src/utils/notification'
import { mountNotificationHost } from '../src/components/notificationHost'

afterEach(() => {
  for (const item of getNotificationSnapshot()) dismissNotificationById(item.id)
})

test('notification subscribers receive immutable snapshots and can unsubscribe', () => {
  const observed: number[] = []
  const unsubscribe = subscribeNotifications((items) => observed.push(items.length))
  portalNotify.success('saved', undefined, 0)
  unsubscribe()
  portalNotify.info('later', undefined, 0)
  assert.deepEqual(observed, [0, 1])
  assert.equal(getNotificationSnapshot().length, 2)
})

test('notification queue is capped at five and removes the oldest entry', () => {
  for (let index = 1; index <= 6; index += 1) portalNotify.info('message ' + index, undefined, 0)
  const items = getNotificationSnapshot()
  assert.equal(items.length, 5)
  assert.equal(items[0]?.message, 'message 2')
  assert.equal(items[4]?.message, 'message 6')
})

test('native host renders text safely and closes notifications through delegated clicks', () => {
  const dom = new JSDOM('<!doctype html><html><body></body></html>')
  const { document, MouseEvent } = dom.window
  const dispose = mountNotificationHost(document.body)
  portalNotify.error('<img src=x onerror=alert(1)>', 'Test', 0)

  const stack = document.querySelector('.portal-notification-stack')
  assert.ok(stack)
  assert.equal(stack.querySelectorAll('section[role="status"]').length, 1)
  assert.equal(stack.querySelector('strong')?.textContent, 'Test')
  assert.equal(stack.querySelector('p')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(stack.querySelector('img'), null)

  const close = stack.querySelector<HTMLButtonElement>('button[aria-label="关闭通知"]')
  assert.ok(close)
  close.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  assert.equal(getNotificationSnapshot().length, 0)
  assert.equal(stack.querySelectorAll('section').length, 0)

  dispose()
  dispose()
  assert.equal(document.querySelector('.portal-notification-stack'), null)
  dom.window.close()
})
