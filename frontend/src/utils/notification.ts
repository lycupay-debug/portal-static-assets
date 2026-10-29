/**
 * 职责：维护与 UI 框架无关的门户通知队列、订阅接口和自动关闭定时器。
 * 依赖：浏览器定时器；不依赖 Vue、DOM、网络或应用页面。
 * 协作：portalNotify 创建通知；subscribeNotifications 为 DOM 宿主订阅快照；
 * dismissNotificationById 关闭通知并释放计时器。队列最多保留五条。
 * 边界与副作用：仅管理客户端内存和有界定时器；duration <= 0 表示不自动关闭；
 * 订阅者必须调用返回的取消函数释放引用。
 */
export type NotificationType = 'success' | 'error' | 'warning' | 'info'
export type PortalNotification = { id: number; type: NotificationType; title: string; message: string }
export type NotificationListener = (items: readonly PortalNotification[]) => void

let notifications: PortalNotification[] = []
let nextId = 1
const listeners = new Set<NotificationListener>()
const timers = new Map<number, ReturnType<typeof setTimeout>>()

/** 发布不可变快照，避免订阅者直接修改内部队列。 */
function publish() {
  const snapshot = notifications.map((item) => ({ ...item }))
  for (const listener of [...listeners]) listener(snapshot)
}

/** 订阅队列变化并立即收到当前状态；返回幂等的取消订阅函数。 */
export function subscribeNotifications(listener: NotificationListener): () => void {
  listeners.add(listener)
  listener(notifications.map((item) => ({ ...item })))
  let subscribed = true
  return () => {
    if (!subscribed) return
    subscribed = false
    listeners.delete(listener)
  }
}

/** 关闭指定通知并释放其定时器；重复关闭是安全操作。 */
export function dismissNotificationById(id: number) {
  const exists = notifications.some((item) => item.id === id)
  notifications = notifications.filter((item) => item.id !== id)
  const timer = timers.get(id)
  if (timer !== undefined) clearTimeout(timer)
  timers.delete(id)
  if (exists) publish()
}

/** 创建非空通知；队列超限时先移除最旧项，确保旧定时器被清理。 */
function notify(type: NotificationType, message: string, title?: string, duration = 4200) {
  const text = message.trim()
  if (!text) return
  const id = nextId++
  const titles: Record<NotificationType, string> = {
    success: '操作成功',
    error: '操作失败',
    warning: '请注意',
    info: '提示',
  }
  notifications = [...notifications, { id, type, title: title || titles[type], message: text }]
  if (notifications.length > 5 && notifications[0]) dismissNotificationById(notifications[0].id)
  if (duration > 0) timers.set(id, setTimeout(() => dismissNotificationById(id), duration))
  publish()
}

export const portalNotify = {
  success: (message: string, title?: string, duration?: number) => notify('success', message, title, duration),
  error: (message: string, title?: string, duration?: number) => notify('error', message, title, duration),
  warning: (message: string, title?: string, duration?: number) => notify('warning', message, title, duration),
  info: (message: string, title?: string, duration?: number) => notify('info', message, title, duration),
}

/** Test seam and diagnostics: expose only a copied snapshot, never the mutable internal queue. */
export function getNotificationSnapshot(): readonly PortalNotification[] {
  return notifications.map((item) => ({ ...item }))
}
