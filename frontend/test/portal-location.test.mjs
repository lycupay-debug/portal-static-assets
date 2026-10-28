/**
 * Responsibility: verify canonical route and knowledge-query normalization independently of Vue lifecycle behavior.
 * Dependencies: Node test runner, the route resolver, and the configured knowledge taxonomy.
 * Collaboration: protects App.vue navigation from invalid aliases and stale/unsupported filter parameters.
 * Boundaries: tests pure URL resolution; history, authentication, and API loading remain covered by navigation tests.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { resolvePortalLocation } from '../src/navigation/resolvePortalLocation.ts'

test('resolves localized aliases and trailing slashes to canonical routes', () => {
  assert.equal(resolvePortalLocation('/个人中心/', '').route, '/account')
  assert.equal(resolvePortalLocation('/登录', '').route, '/login')
  assert.equal(resolvePortalLocation('/education-resources///', '').route, '/education-resources')
  assert.equal(resolvePortalLocation('/', '').route, '/')
})

test('knowledge route defaults to recent when query filters are absent or unsupported', () => {
  assert.deepEqual(resolvePortalLocation('/knowledge', ''), {
    route: '/knowledge', knowledgeCategory: 'recent', knowledgeSubcategory: '',
  })
  assert.deepEqual(resolvePortalLocation('/knowledge', '?category=not-configured&subcategory=bad'), {
    route: '/knowledge', knowledgeCategory: 'recent', knowledgeSubcategory: '',
  })
})

test('knowledge route keeps only a subcategory belonging to the selected category', () => {
  assert.deepEqual(resolvePortalLocation('/knowledge', '?category=education&subcategory=小学教育'), {
    route: '/knowledge', knowledgeCategory: 'education', knowledgeSubcategory: '小学教育',
  })
  assert.deepEqual(resolvePortalLocation('/knowledge', '?category=education&subcategory=事业编'), {
    route: '/knowledge', knowledgeCategory: 'education', knowledgeSubcategory: '',
  })
})

test('non-knowledge routes ignore knowledge-only query parameters', () => {
  assert.deepEqual(resolvePortalLocation('/login', '?category=education&subcategory=小学教育'), {
    route: '/login', knowledgeCategory: 'recent', knowledgeSubcategory: '',
  })
})
