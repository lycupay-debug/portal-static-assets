import assert from 'node:assert/strict'
import test from 'node:test'
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
