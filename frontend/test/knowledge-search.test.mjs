import assert from 'node:assert/strict'
import test from 'node:test'
import { buildKnowledgeSearchSuggestions, knowledgeEditDistance, knowledgeFuzzyMatch } from '../src/utils/knowledgeSearch.ts'

test('edit distance counts insertions, deletions and substitutions', () => {
  assert.equal(knowledgeEditDistance('kitten', 'sitting'), 3)
  assert.equal(knowledgeEditDistance('portal', 'portal'), 0)
  assert.equal(knowledgeEditDistance('猫🐈', '猫狗'), 1)
})

test('knowledge fuzzy matching normalizes Unicode width, case and whitespace', () => {
  assert.equal(knowledgeFuzzyMatch('ＦｒｅｅＢＳＤ', 'free bsd'), true)
  assert.equal(knowledgeFuzzyMatch('PostgreSQL database', 'POSTGRESQL'), true)
})

test('knowledge fuzzy matching supports ordered Chinese partial phrases and small typos', () => {
  assert.equal(knowledgeFuzzyMatch('系统维护与部署', '系统护'), true)
  assert.equal(knowledgeFuzzyMatch('database migration', 'databse'), true)
  assert.equal(knowledgeFuzzyMatch('portal security', 'zzzz'), false)
})

test('an empty fuzzy query matches without filtering', () => {
  assert.equal(knowledgeFuzzyMatch('anything', ''), true)
})

test('bounded typo matching finds late matches in long source text', () => {
  const source = '无关内容'.repeat(5000) + 'the database migration guide'
  assert.equal(knowledgeFuzzyMatch(source, 'databse migration'), true)
  assert.equal(knowledgeFuzzyMatch(source, 'zzzz'), false)
})

test('knowledge suggestions merge themes and articles, deduplicate labels and cap results', () => {
  const categories = [{ label: '编程', children: ['Python', '数据库'] }]
  const articles = [
    { label: 'Python 入门', slug: 'python-intro', summary: '编程实践' },
    { label: 'Python', slug: 'python', summary: '语言介绍' },
  ]
  assert.deepEqual(buildKnowledgeSearchSuggestions(categories, articles, ''), [])
  const result = buildKnowledgeSearchSuggestions(categories, articles, 'python')
  assert.equal(result[0]?.label, 'Python')
  assert.equal(result.filter(item => item.label === 'Python').length, 1)
  assert.ok(result.some(item => item.kind === 'article' && item.slug === 'python-intro'))
  assert.ok(result.length <= 8)
})
