import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { routeAliases, routeSections, sectionRoutes } from '../src/navigation/portalRoutes.ts'

test('account section routes map every section to one route and reverse-map uniquely', () => {
  assert.equal(sectionRoutes.overview, '/account')
  assert.equal(routeSections['/account'], 'overview')
  assert.equal(Object.keys(routeSections).length, Object.keys(sectionRoutes).length)
  for (const [section, path] of Object.entries(sectionRoutes)) assert.equal(routeSections[path], section)
})

test('legacy localized paths resolve to canonical routes', () => {
  assert.equal(routeAliases['/登录'], '/login')
  assert.equal(routeAliases['/个人中心'], '/account')
  assert.equal(routeAliases['/教学资源'], '/education/teaching-resources')
  assert.equal(routeAliases['/内容管理'], '/content-management')
})


const appSource = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')

test('protected history navigation checks the session before loading route data', () => {
  const start = appSource.indexOf('function onPopState()')
  const end = appSource.indexOf('onMounted(() => {', start)
  assert.ok(start >= 0 && end > start)
  const handler = appSource.slice(start, end)
  assert.ok(handler.indexOf('await loadSession()') < handler.indexOf('loadRouteData()'))
  assert.match(handler, /navigationId !== navigationSequence/)
})

test('async navigation ignores stale completions after a newer navigation begins', () => {
  const start = appSource.indexOf('async function navigate(target: string)')
  const end = appSource.indexOf('async function loadSession()', start)
  assert.ok(start >= 0 && end > start)
  const handler = appSource.slice(start, end)
  assert.match(handler, /const navigationId = \+\+navigationSequence/)
  assert.match(handler, /if \(navigationId !== navigationSequence\) return/)
})

test('initial protected routes wait for session validation before loading route data', () => {
  const start = appSource.indexOf('onMounted(() => {')
  const end = appSource.indexOf('onBeforeUnmount(() => {', start)
  assert.ok(start >= 0 && end > start)
  const lifecycle = appSource.slice(start, end)
  const protectedBranch = lifecycle.slice(lifecycle.indexOf('if (isProtectedPage.value)'), lifecycle.indexOf('} else {', lifecycle.indexOf('if (isProtectedPage.value)')))
  assert.ok(protectedBranch.indexOf('loadSession()') < protectedBranch.indexOf('loadRouteData()'))
})
