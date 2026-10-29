import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { dismissNotificationById, notificationState, portalNotify } from '../src/utils/notification.ts'

function clearNotifications() {
  for (const item of [...notificationState.value]) dismissNotificationById(item.id)
}

test('notification manager ignores blank messages and applies default titles', () => {
  clearNotifications()
  portalNotify.info('   ', undefined, 0)
  assert.equal(notificationState.value.length, 0)
  portalNotify.success('Saved', undefined, 0)
  assert.equal(notificationState.value.length, 1)
  assert.equal(notificationState.value[0].title, '操作成功')
  assert.equal(notificationState.value[0].message, 'Saved')
  clearNotifications()
})

test('notification queue is bounded to five and evicts the oldest notification', () => {
  clearNotifications()
  for (let index = 1; index <= 6; index += 1) portalNotify.info(`message-${index}`, undefined, 0)
  assert.equal(notificationState.value.length, 5)
  assert.deepEqual(notificationState.value.map(item => item.message), [
    'message-2', 'message-3', 'message-4', 'message-5', 'message-6',
  ])
  clearNotifications()
})

test('manual dismissal removes a notification from the queue', () => {
  clearNotifications()
  portalNotify.warning('dismiss me', undefined, 0)
  const id = notificationState.value[0].id
  dismissNotificationById(id)
  assert.equal(notificationState.value.some(item => item.id === id), false)
})

test('manual dismissal and queue eviction release their timers, including a falsy timer handle', () => {
  clearNotifications()
  const originalSetTimeout = globalThis.setTimeout
  const originalClearTimeout = globalThis.clearTimeout
  const timers = []
  const cleared = []
  globalThis.setTimeout = (callback, delay) => {
    const handle = timers.length === 0 ? 0 : { callback, delay }
    timers.push({ handle, callback, delay })
    return handle
  }
  globalThis.clearTimeout = (handle) => { cleared.push(handle) }
  try {
    portalNotify.info('manual', undefined, 100)
    const manuallyDismissedId = notificationState.value.at(-1).id
    const manualTimer = timers[0].handle
    dismissNotificationById(manuallyDismissedId)
    assert.ok(cleared.includes(manualTimer), 'manual dismissal should clear its timer even when the handle is 0')

    for (let index = 1; index <= 6; index += 1) portalNotify.info(`queued-${index}`, undefined, 100)
    assert.equal(notificationState.value.length, 5)
    assert.ok(cleared.includes(timers[1].handle), 'evicting the oldest queued notification should clear its timer')
    clearNotifications()
  } finally {
    globalThis.setTimeout = originalSetTimeout
    globalThis.clearTimeout = originalClearTimeout
    clearNotifications()
  }
})

test('timed notifications are automatically removed', async () => {
  clearNotifications()
  portalNotify.success('short-lived', undefined, 15)
  const id = notificationState.value[0].id
  await new Promise(resolve => setTimeout(resolve, 35))
  assert.equal(notificationState.value.some(item => item.id === id), false)
  clearNotifications()
})


test('notification host provides accessible live updates and explicit dismissal', () => {
  const host = readFileSync(new URL('../src/components/NotificationHost.vue', import.meta.url), 'utf8')
  assert.match(host, /<Teleport to="body">/)
  assert.match(host, /aria-live="polite"/)
  assert.match(host, /:key="item\.id"/)
  assert.match(host, /role="status"/)
  assert.match(host, /aria-label="关闭通知"/)
  assert.match(host, /dismissNotificationById\(item\.id\)/)
})
