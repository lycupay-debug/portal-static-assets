/**
 * Responsibility: render the shared notification queue with native DOM APIs.
 * Dependencies: notification-store subscription and the Bootstrap Icons CSS classes.
 * Collaboration: portalApplication.ts mounts this host and owns the disposer; portalNotify publishes changes.
 * Boundaries: no framework runtime, network access, or HTML-string parsing; all user text uses textContent.
 * Cleanup: unsubscribe, remove delegated click handling, and detach the host when the owner unmounts.
 */
import { dismissNotificationById, subscribeNotifications } from '../utils/notification'

const icons = { success: 'bi-check-lg', error: 'bi-x-lg', warning: 'bi-exclamation-lg', info: 'bi-info-lg' } as const

export function mountNotificationHost(container: HTMLElement): () => void {
  const doc = container.ownerDocument
  const stack = doc.createElement('div')
  stack.className = 'portal-notification-stack'
  stack.setAttribute('aria-label', '系统通知')
  stack.setAttribute('aria-live', 'polite')
  stack.setAttribute('aria-relevant', 'additions')
  container.append(stack)
  const unsubscribe = subscribeNotifications((items) => {
    const fragment = doc.createDocumentFragment()
    for (const item of items) {
      const section = doc.createElement('section')
      section.className = 'portal-notification is-' + item.type
      section.setAttribute('role', 'status')
      const icon = doc.createElement('span')
      icon.className = 'portal-notification-icon'
      icon.setAttribute('aria-hidden', 'true')
      const iconNode = doc.createElement('i')
      iconNode.className = 'bi ' + icons[item.type]
      icon.append(iconNode)
      const copy = doc.createElement('div')
      copy.className = 'portal-notification-copy'
      const title = doc.createElement('strong')
      title.textContent = item.title
      const message = doc.createElement('p')
      message.textContent = item.message
      copy.append(title, message)
      const close = doc.createElement('button')
      close.type = 'button'
      close.className = 'portal-notification-close'
      close.setAttribute('aria-label', '关闭通知')
      close.dataset.notificationId = String(item.id)
      const closeIcon = doc.createElement('i')
      closeIcon.className = 'bi bi-x-lg'
      closeIcon.setAttribute('aria-hidden', 'true')
      close.append(closeIcon)
      section.append(icon, copy, close)
      fragment.append(section)
    }
    stack.replaceChildren(fragment)
  })
  const onClick = (event: Event) => {
    const target = event.target
    if (!(target instanceof doc.defaultView!.Element)) return
    const button = target.closest<HTMLButtonElement>('button[data-notification-id]')
    if (!button || !stack.contains(button)) return
    const id = Number(button.dataset.notificationId)
    if (Number.isSafeInteger(id) && id > 0) dismissNotificationById(id)
  }
  stack.addEventListener('click', onClick)
  let mounted = true
  return () => {
    if (!mounted) return
    mounted = false
    unsubscribe()
    stack.removeEventListener('click', onClick)
    stack.remove()
  }
}
