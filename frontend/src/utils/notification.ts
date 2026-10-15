import { ref } from 'vue'
export type NotificationType = 'success' | 'error' | 'warning' | 'info'
export type PortalNotification = { id: number; type: NotificationType; title: string; message: string }
const notifications = ref<PortalNotification[]>([])
let nextId = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()
function dismiss(id: number) {
  notifications.value = notifications.value.filter((item) => item.id !== id)
  const timer = timers.get(id)
  if (timer) clearTimeout(timer)
  timers.delete(id)
}
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
