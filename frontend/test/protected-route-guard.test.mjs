/**
 * Responsibility: verify protected-route session gating, denial behavior, and stale-navigation races.
 * Dependencies: Node test runner and the framework-independent route guard factory.
 * Collaboration: protects the shared contract used by App.vue navigation, popstate, and initial protected loads.
 * Boundaries: no real browser history, network, or production credentials; session loading is injected.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { createProtectedRouteGuard } from '../src/composables/useProtectedRouteGuard.ts'

function deferred() {
  let resolve
  const promise = new Promise((res) => { resolve = res })
  return { promise, resolve }
}

function setup({ protectedRoute = true, loadSession = async () => true, isCurrent = () => true } = {}) {
  const redirects = []
  const guard = createProtectedRouteGuard({
    isProtected: () => protectedRoute,
    loadSession,
    isCurrent,
    redirectToLogin: () => redirects.push('/login'),
  })
  return { ...guard, redirects }
}

test('public routes proceed without requesting a session', async () => {
  let requests = 0
  const guard = setup({ protectedRoute: false, loadSession: async () => { requests++; return true } })
  assert.equal(await guard.ensureRouteAccess(1), true)
  assert.equal(requests, 0)
  assert.deepEqual(guard.redirects, [])
})

test('protected routes wait for session validation and redirect only when invalid', async () => {
  const guard = setup({ loadSession: async () => false })
  assert.equal(await guard.ensureRouteAccess(1), false)
  assert.deepEqual(guard.redirects, ['/login'])
})

test('valid protected sessions continue without redirecting', async () => {
  const guard = setup()
  assert.equal(await guard.ensureRouteAccess(1), true)
  assert.deepEqual(guard.redirects, [])
})

test('a stale navigation does not start a session request', async () => {
  let requests = 0
  const guard = setup({ isCurrent: () => false, loadSession: async () => { requests++; return false } })
  assert.equal(await guard.ensureRouteAccess(1), false)
  assert.equal(requests, 0)
  assert.deepEqual(guard.redirects, [])
})

test('an old session response cannot redirect after a newer navigation begins', async () => {
  const session = deferred()
  let currentNavigation = 1
  const guard = setup({ loadSession: () => session.promise, isCurrent: id => id === currentNavigation })
  const pending = guard.ensureRouteAccess(1)
  currentNavigation = 2
  session.resolve(false)
  assert.equal(await pending, false)
  assert.deepEqual(guard.redirects, [])
})
