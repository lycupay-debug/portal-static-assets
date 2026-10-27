import assert from 'node:assert/strict'
import test from 'node:test'
import { knowledgeCategoryMenus, knowledgeCategorySearchTerms } from '../src/config/knowledgeCategories.ts'

test('knowledge category ids and labels are unique and child categories do not overlap', () => {
  assert.equal(new Set(knowledgeCategoryMenus.map(category => category.value)).size, knowledgeCategoryMenus.length)
  assert.equal(new Set(knowledgeCategoryMenus.map(category => category.label)).size, knowledgeCategoryMenus.length)
  for (const category of knowledgeCategoryMenus) {
    assert.ok(Array.isArray(category.children))
    assert.equal(new Set(category.children).size, category.children.length)
  }
})

test('category search terms only reference configured categories', () => {
  const categoryIds = new Set(knowledgeCategoryMenus.map(category => category.value))
  for (const [category, terms] of Object.entries(knowledgeCategorySearchTerms)) {
    assert.ok(categoryIds.has(category))
    assert.ok(terms.length > 0)
  }
})
