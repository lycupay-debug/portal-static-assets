import assert from 'node:assert/strict'
import test from 'node:test'
import { buildSearchableTableRecords, filterAccountTableRecords } from '../src/utils/accountTableFiltering.ts'

test('search index excludes UI-only actions and normalizes case', () => {
  const [record] = buildSearchableTableRecords([
    { title: 'FreeBSD Handbook', summary: 'Operating System', actions: 'delete', count: 3 },
  ])
  assert.equal(record._searchText, 'freebsd handbook Operating System 3'.toLowerCase())
  assert.equal(record.actions, 'delete')
})

test('account table search supports normal and whitespace-insensitive matching', () => {
  const records = buildSearchableTableRecords([
    { title: 'Go SSH MCP', status: 'active', actions: 'open' },
    { title: 'Personal Portal', status: 'draft', actions: 'delete' },
  ])
  assert.deepEqual(filterAccountTableRecords(records, ' GO SSH ' ).map(item => item.title), ['Go SSH MCP'])
  assert.deepEqual(filterAccountTableRecords(records, 'personalportal').map(item => item.title), ['Personal Portal'])
})

test('empty search preserves records and the original record array is not mutated', () => {
  const source = [{ title: 'One', actions: 'view' }, { title: 'Two', actions: 'edit' }]
  const before = source.map(item => ({ ...item }))
  const indexed = buildSearchableTableRecords(source)
  assert.deepEqual(filterAccountTableRecords(indexed, '  '), indexed)
  assert.deepEqual(source, before)
})
