import assert from 'node:assert/strict'
import test from 'node:test'
import { knowledgeEditDistance, knowledgeFuzzyMatch } from '../src/utils/knowledgeSearch.ts'

test('edit distance counts insertions, deletions and substitutions', () => {
  assert.equal(knowledgeEditDistance('kitten', 'sitting'), 3)
  assert.equal(knowledgeEditDistance('portal', 'portal'), 0)
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
