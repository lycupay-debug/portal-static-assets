import assert from 'node:assert/strict'
import test from 'node:test'
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

test('timed notifications are automatically removed', async () => {
  clearNotifications()
  portalNotify.success('short-lived', undefined, 15)
  const id = notificationState.value[0].id
  await new Promise(resolve => setTimeout(resolve, 35))
  assert.equal(notificationState.value.some(item => item.id === id), false)
  clearNotifications()
})
