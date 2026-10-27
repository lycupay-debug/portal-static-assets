import assert from 'node:assert/strict'
import test from 'node:test'
import { filterEducationResources } from '../src/utils/resourceFiltering.ts'

const resources = [
  { id: 'old', title: 'FreeBSD reference', description: 'System notes', type: '电子书', grade: '大学', filename: 'handbook.pdf', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
  { id: 'match', title: 'FreeBSD Handbook', description: 'Operating system', type: '电子书', grade: '大学', filename: 'FreeBSD Handbook.pdf', createdAt: '2026-02-01T00:00:00Z', updatedAt: '2026-02-01T00:00:00Z' },
  { id: 'audio', title: 'FreeBSD listening', description: 'Audio practice', type: '听力材料', grade: '高中', filename: 'freebsd.mp3', createdAt: '2026-03-01T00:00:00Z', updatedAt: '2026-03-01T00:00:00Z' },
  { id: 'newest', title: 'Reference', description: 'No search hit', type: '电子书', grade: '大学', filename: 'reference.pdf', createdAt: '2026-04-01T00:00:00Z', updatedAt: '2026-04-01T00:00:00Z' },
]

test('resource filtering applies category and grade filters together', () => {
  assert.deepEqual(
    filterEducationResources(resources, '电子书', '大学', '').map(item => item.id),
    ['newest', 'match', 'old'],
  )
  assert.deepEqual(
    filterEducationResources(resources, '听力材料', '大学', '').map(item => item.id),
    [],
  )
})

test('resource search ranks exact filenames before title and fallback matches', () => {
  assert.deepEqual(
    filterEducationResources(resources, '全部资源', '全部年级', 'FreeBSD Handbook.pdf').map(item => item.id),
    ['match', 'old'],
  )
})

test('resource results default to newest updated first and do not mutate the source array', () => {
  const before = resources.map(item => item.id)
  const result = filterEducationResources(resources, '全部资源', '全部年级', '')
  assert.deepEqual(result.map(item => item.id), ['newest', 'audio', 'match', 'old'])
  assert.deepEqual(resources.map(item => item.id), before)
})
