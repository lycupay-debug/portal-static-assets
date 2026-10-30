/**
 * Responsibility: persist and restore per-route window and nested account scroll positions.
 * Dependencies: structural route value, browser/document/storage adapters and a render-completion scheduler.
 * Collaboration: route navigation invokes save before history changes and restore after destination rendering.
 * Boundaries: storage is best-effort, malformed state is ignored, and no route or page data is mutated.
 */
import type { ReadonlyValue } from './valueContracts'

type ScrollState = { windowY?: number; accountContent?: number; accountSidebar?: number; table?: number }
type ScrollBrowser = Pick<Window, 'scrollY' | 'scrollTo'>
type ScrollDocument = Pick<Document, 'querySelector'>
type ScrollStorage = Pick<Storage, 'getItem' | 'setItem'>
const SCROLL_TARGETS = [
  ['.account-content', 'accountContent'],
  ['.account-sidebar', 'accountSidebar'],
  ['.virtual-table-scroll', 'table'],
] as const

export function createScrollRestoration(
  route: ReadonlyValue<string>,
  dependencies: { browser: ScrollBrowser; document: ScrollDocument; storage: ScrollStorage; afterRender: () => Promise<unknown> },
) {
  const { browser, document, storage, afterRender } = dependencies
  const storageKey = (path = route.value) => `portal:scroll:${path}`

  function saveScrollPosition() {
    try {
      const state: ScrollState = { windowY: browser.scrollY }
      for (const [selector, key] of SCROLL_TARGETS) state[key] = document.querySelector<HTMLElement>(selector)?.scrollTop || 0
      storage.setItem(storageKey(), JSON.stringify(state))
    } catch { /* Storage may be unavailable. */ }
  }

  async function restoreScrollPosition() {
    await afterRender()
    try {
      const saved = storage.getItem(storageKey())
      if (!saved) return
      const state = JSON.parse(saved) as ScrollState
      browser.scrollTo({ top: state.windowY || 0, behavior: 'auto' })
      for (const [selector, key] of SCROLL_TARGETS) {
        const element = document.querySelector<HTMLElement>(selector)
        const value = state[key]
        if (element && typeof value === 'number') element.scrollTop = value
      }
    } catch { /* Ignore malformed state. */ }
  }

  return { saveScrollPosition, restoreScrollPosition }
}
