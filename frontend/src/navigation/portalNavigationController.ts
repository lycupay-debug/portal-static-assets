/**
 * Responsibility: coordinate browser history, protected-route sequencing, route-data loading and scroll restoration.
 * Dependencies: injected route state, route maps, browser/document objects, and application workflow callbacks.
 * Collaboration: App.vue currently adapts its state/composables into this controller; the controller itself has no
 * Vue imports and can later be mounted by the native application shell without changing navigation semantics.
 * Boundaries: browser history is client navigation only; ensureRouteAccess remains the authentication gate and
 * the Go API remains the authority. Stale navigation IDs may not trigger route data or scroll side effects.
 * Cleanup: dispose removes the owned popstate listener and invalidates any in-flight navigation sequence.
 */
import { resolvePortalLocation, type ResolvedPortalLocation } from './resolvePortalLocation'

export type PortalNavigationOptions = {
  browser: Window
  document: Document
  routeSections: Record<string, string>
  nextNavigationId: () => number
  isNavigationCurrent: (id: number) => boolean
  getRoute: () => string
  setRoute: (route: string) => void
  isProtectedPage: () => boolean
  isAccountPage: () => boolean
  setKnowledgeLocation: (location: ResolvedPortalLocation) => void
  setAccountSection: (section: string) => void
  setActiveSidebarButton: (section: string) => void
  ensureRouteAccess: (navigationId: number) => Promise<boolean>
  saveScrollPosition: () => void
  restoreScrollPosition: () => Promise<void> | void
  closeMenu: () => void
  clearAuthError: () => void
  clearAuthNotice: () => void
  loadPublishedLegalDocuments: () => Promise<unknown> | unknown
  loadContentList: () => Promise<unknown> | unknown
  loadTeachingResources: () => Promise<unknown> | unknown
  loadRouteData: () => void
  loadSession: () => Promise<unknown> | unknown
  observeVirtualTableWidth: () => void
  onScrollCapture: () => void
  afterRender: () => Promise<unknown>
}

export function createPortalNavigationController(options: PortalNavigationOptions) {
  const { browser, document } = options
  let disposed = false

  function setRouteFromLocation(): ResolvedPortalLocation {
    const resolved = resolvePortalLocation(browser.location.pathname, browser.location.search)
    options.setRoute(resolved.route)
    if (resolved.route === '/knowledge') options.setKnowledgeLocation(resolved)
    if (options.isAccountPage()) {
      const section = options.routeSections[resolved.route] || 'overview'
      options.setAccountSection(section)
      options.setActiveSidebarButton(section)
    }
    return resolved
  }

  async function navigate(target: string): Promise<void> {
    if (disposed) return
    const navigationId = options.nextNavigationId()
    options.saveScrollPosition()
    options.closeMenu()
    browser.history.pushState({}, '', target)
    setRouteFromLocation()
    void options.loadPublishedLegalDocuments()
    options.clearAuthError()
    options.clearAuthNotice()
    if (!await options.ensureRouteAccess(navigationId) || !options.isNavigationCurrent(navigationId)) return

    const route = options.getRoute()
    if (route === '/content-management') {
      await options.loadContentList()
      if (!options.isNavigationCurrent(navigationId)) return
    }
    if (route === '/education/teaching-resources') {
      await options.loadTeachingResources()
      if (!options.isNavigationCurrent(navigationId)) return
    }
    if (!options.isNavigationCurrent(navigationId)) return
    options.loadRouteData()

    if (route === '/' && browser.location.hash) {
      await options.afterRender()
      if (!options.isNavigationCurrent(navigationId)) return
      document.querySelector(browser.location.hash)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      return
    }

    browser.scrollTo({ top: 0, behavior: 'auto' })
    if (options.isAccountPage()) {
      await options.afterRender()
      if (!options.isNavigationCurrent(navigationId)) return
      const content = document.querySelector<HTMLElement>('.account-content')
      const table = document.querySelector<HTMLElement>('.virtual-table-scroll')
      if (content) content.scrollTop = 0
      if (table) table.scrollTop = 0
    }
  }

  function onPopState(): void {
    if (disposed) return
    const navigationId = options.nextNavigationId()
    setRouteFromLocation()
    options.closeMenu()
    options.clearAuthError()
    void (async () => {
      if (!await options.ensureRouteAccess(navigationId) || !options.isNavigationCurrent(navigationId)) return
      const route = options.getRoute()
      if (route === '/content-management') void options.loadContentList()
      if (route === '/education/teaching-resources') void options.loadTeachingResources()
      options.loadRouteData()
      await options.restoreScrollPosition()
    })()
  }

  async function start(): Promise<void> {
    if (disposed) return
    const navigationId = options.nextNavigationId()
    setRouteFromLocation()
    await options.afterRender()
    if (!options.isNavigationCurrent(navigationId) || disposed) return
    options.observeVirtualTableWidth()
    browser.addEventListener('popstate', onPopState)
    document.addEventListener('scroll', options.onScrollCapture, true)
    browser.addEventListener('beforeunload', options.saveScrollPosition)
    void options.loadPublishedLegalDocuments()

    if (options.isProtectedPage()) {
      if (!await options.ensureRouteAccess(navigationId) || !options.isNavigationCurrent(navigationId)) return
      const route = options.getRoute()
      if (route === '/content-management') void options.loadContentList()
      if (route === '/education/teaching-resources') void options.loadTeachingResources()
      options.loadRouteData()
      void options.restoreScrollPosition()
    } else {
      options.loadRouteData()
      void options.loadSession()
      void options.restoreScrollPosition()
    }
  }

  function dispose(): void {
    if (disposed) return
    disposed = true
    browser.removeEventListener('popstate', onPopState)
    document.removeEventListener('scroll', options.onScrollCapture, true)
    browser.removeEventListener('beforeunload', options.saveScrollPosition)
    options.nextNavigationId()
  }

  return { navigate, setRouteFromLocation, onPopState, start, dispose }
}
