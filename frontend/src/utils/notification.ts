/**
 * 职责：维护门户全局通知队列，并为限时通知管理自动关闭定时器。
 * 依赖：Vue ref；由 NotificationHost.vue 订阅状态并提供手动关闭入口。
 * 协作：portalNotify 创建通知，dismissNotificationById 关闭通知；队列最多保留五条。
 * 边界：仅管理客户端内存状态和定时器，不执行网络请求；duration <= 0 表示不自动关闭。
 */
import { ref } from 'vue'
export type NotificationType = 'success' | 'error' | 'warning' | 'info'
type PortalNotification = { id: number; type: NotificationType; title: string; message: string }
const notifications = ref<PortalNotification[]>([])
let nextId = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()

/**
 * 关闭指定通知并释放其定时器。对已不存在的 ID 重复调用保持安全。
 */
function dismiss(id: number) {
  notifications.value = notifications.value.filter((item) => item.id !== id)
  const timer = timers.get(id)
  // Do not use truthiness: timer handles are opaque values and cleanup must depend on presence.
  if (timer !== undefined) clearTimeout(timer)
  timers.delete(id)
}

/**
 * 创建一条非空通知；队列超限时先关闭最旧项，确保其定时器同步释放。
 */
function notify(type: NotificationType, message: string, title?: string, duration = 4200) {
  const text = message.trim()
  if (!text) return
  const id = nextId++
  const titles = { success: '操作成功', error: '操作失败', warning: '请注意', info: '提示' }
  notifications.value.push({ id, type, title: title || titles[type], message: text })
  if (notifications.value.length > 5 && notifications.value[0]) dismiss(notifications.value[0].id)
  if (duration > 0) timers.set(id, setTimeout(() => dismiss(id), duration))
}
export const notificationState = notifications
export const dismissNotificationById = dismiss
export const portalNotify = {
  success: (message: string, title?: string, duration?: number) => notify('success', message, title, duration),
  error: (message: string, title?: string, duration?: number) => notify('error', message, title, duration),
  warning: (message: string, title?: string, duration?: number) => notify('warning', message, title, duration),
  info: (message: string, title?: string, duration?: number) => notify('info', message, title, duration),
}
