import assert from 'node:assert/strict'
import test from 'node:test'
import { filterKnowledgeItems } from '../src/utils/knowledgeFiltering.ts'

const items = [
  { title: 'FreeBSD 系统维护', summary: '系统配置与部署', tags: ['系统', '系统维护'], publishedAt: '2026-10-25' },
  { title: 'Python 入门', summary: '编程实践', tags: ['编程', 'Python'], publishedAt: '2026-10-26' },
  { title: '无标签文章', summary: '系统维护经验', publishedAt: '2026-10-27' },
]

test('recent knowledge results are copied and sorted newest first', () => {
  const result = filterKnowledgeItems(items, 'recent', '', '')
  assert.deepEqual(result.map(item => item.title), ['无标签文章', 'Python 入门', 'FreeBSD 系统维护'])
  assert.notEqual(result, items)
})

test('category filters use taxonomy and fallback text only for untagged legacy items', () => {
  assert.deepEqual(filterKnowledgeItems(items, 'programming', '', '').map(item => item.title), ['Python 入门'])
  assert.deepEqual(filterKnowledgeItems(items, 'system', '', '').map(item => item.title), ['无标签文章', 'FreeBSD 系统维护'])
  assert.deepEqual(filterKnowledgeItems(items, 'system', '系统维护', '').map(item => item.title), ['无标签文章', 'FreeBSD 系统维护'])
})

test('search term narrows category results with the shared fuzzy matcher', () => {
  assert.deepEqual(filterKnowledgeItems(items, 'recent', '', 'python').map(item => item.title), ['Python 入门'])
  assert.deepEqual(filterKnowledgeItems(items, 'recent', '', 'zzzz'), [])
})
