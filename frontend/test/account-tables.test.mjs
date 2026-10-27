import assert from 'node:assert/strict'
import test from 'node:test'
import { contentStatusLabels, contentTypeLabels, tableDefinitions } from '../src/config/accountTables.ts'

test('account table definitions have unique columns and stable required metadata', () => {
  assert.ok(Object.keys(tableDefinitions).includes('content-management'))
  assert.ok(Object.keys(tableDefinitions).includes('teaching-resources'))
  for (const [section, definition] of Object.entries(tableDefinitions)) {
    assert.ok(definition.title.length > 0, 'table has a title')
    assert.ok(definition.columns.length > 0, 'table has columns')
    assert.equal(new Set(definition.columns.map(column => column.key)).size, definition.columns.length, 'column keys are unique')
    assert.ok(definition.note.length > 0, 'table has an explanatory note')
    assert.ok(section.length > 0)
  }
})

test('content type and status labels cover supported API values', () => {
  assert.deepEqual(Object.keys(contentTypeLabels).sort(), ['article', 'project', 'resource', 'tool'])
  assert.deepEqual(Object.keys(contentStatusLabels).sort(), ['draft', 'published', 'unpublished'])
})
