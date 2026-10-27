import assert from 'node:assert/strict'
import test from 'node:test'
import { knowledgeTagOptions, mergeLegacyTaxonomyIntoTags, normalizeKnowledgeTags, removedGradeTags, taxonomyParentNames } from '../src/utils/knowledgeTags.ts'

test('knowledge editor tag options omit grade taxonomy and keep parent-child hierarchy', () => {
  assert.equal(knowledgeTagOptions.some(category => category.value === '年级'), false)
  assert.equal(knowledgeTagOptions.some(category => category.children.some(child => removedGradeTags.has(child))), false)
  assert.ok(taxonomyParentNames.has('教育'))
})

test('normalization deduplicates children and derives their parent categories', () => {
  assert.deepEqual(normalizeKnowledgeTags(['Python', 'Python', '数据库', '教育']), ['编程', 'Python', '数据库'])
  assert.deepEqual(normalizeKnowledgeTags(['初一', '本科', '系统维护']), ['系统', '系统维护'])
})

test('legacy taxonomy fields merge into the canonical tag representation', () => {
  assert.deepEqual(mergeLegacyTaxonomyIntoTags(['Python'], '编程', '数据库'), ['编程', 'Python', '数据库'])
  assert.deepEqual(mergeLegacyTaxonomyIntoTags([], null, null), [])
})
