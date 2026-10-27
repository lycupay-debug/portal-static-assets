import { nextTick, type Ref } from 'vue'

type ScrollState = {
  windowY?: number
  accountContent?: number
  accountSidebar?: number
  table?: number
}

const SCROLL_TARGETS = [
  ['.account-content', 'accountContent'],
  ['.account-sidebar', 'accountSidebar'],
  ['.virtual-table-scroll', 'table'],
] as const

/** Persists per-route window and nested workspace scroll positions for history navigation. */
export function useScrollRestoration(route: Ref<string>) {
  const storageKey = (path = route.value) => `portal:scroll:${path}`

  function saveScrollPosition() {
    try {
      const state: ScrollState = { windowY: window.scrollY }
      for (const [selector, key] of SCROLL_TARGETS) {
        state[key] = document.querySelector<HTMLElement>(selector)?.scrollTop || 0
      }
      sessionStorage.setItem(storageKey(), JSON.stringify(state))
    } catch { /* Storage may be unavailable. */ }
  }

  async function restoreScrollPosition() {
    await nextTick()
    try {
      const saved = sessionStorage.getItem(storageKey())
      if (!saved) return
      const state = JSON.parse(saved) as ScrollState
      window.scrollTo({ top: state.windowY || 0, behavior: 'auto' })
      for (const [selector, key] of SCROLL_TARGETS) {
        const element = document.querySelector<HTMLElement>(selector)
        const value = state[key]
        if (element && typeof value === 'number') element.scrollTop = value
      }
    } catch { /* Ignore malformed state. */ }
  }

  return { saveScrollPosition, restoreScrollPosition }
}
