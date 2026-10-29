/**
 * Responsibility: centralize the client-side session gate for protected portal routes.
 * Dependencies: caller-provided route classification, session loader, navigation freshness check, and redirect action.
 * Collaboration: App.vue owns History API mutation and the navigation sequence; this guard decides whether route
 * data may proceed after session validation. The Go API remains authoritative for authorization.
 * Boundaries: no DOM/history access, no route mutation except through the injected denial callback, and stale
 * asynchronous checks must never redirect a newer navigation.
 */
export type ProtectedRouteGuardOptions = {
  isProtected: () => boolean
  loadSession: () => Promise<boolean>
  isCurrent: (navigationId: number) => boolean
  redirectToLogin: () => void
}

/** Validate protected-route access and reject stale completions before any redirect side effect. */
export function createProtectedRouteGuard(options: ProtectedRouteGuardOptions) {
  async function ensureRouteAccess(navigationId: number): Promise<boolean> {
    if (!options.isCurrent(navigationId)) return false
    if (!options.isProtected()) return true

    const validSession = await options.loadSession()
    if (!options.isCurrent(navigationId)) return false
    if (!validSession) {
      options.redirectToLogin()
      return false
    }
    return true
  }

  return { ensureRouteAccess }
}
