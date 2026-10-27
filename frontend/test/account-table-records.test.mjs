import assert from 'node:assert/strict'
import test from 'node:test'
import { buildAccountTableRecords } from '../src/utils/accountTableRecords.ts'

const content = {
  id: 'content-1', title: 'A guide', slug: 'a-guide', summary: 'Summary', body: 'Body',
  contentType: 'article', status: 'published', visibility: 'public', tags: ['教育', '英语'],
  parentCategory: '教育', childCategory: '英语', updatedAt: '2026-10-27T12:00:00Z',
  likeCount: 2, favoriteCount: 3,
}
const resource = {
  id: 'resource-1', title: 'Lesson', filename: 'lesson.pdf', type: '课件', grade: '高中',
  isPublic: true, description: 'Teaching notes', url: '/resource.pdf',
  createdAt: '2026-10-26T12:00:00Z', updatedAt: '2026-10-27T12:00:00Z',
}
const base = {
  section: 'content-management',
  contentViewMode: 'active',
  contentItems: [content],
  contentTrashItems: [{ ...content, deletedAt: '2026-10-28T00:00:00Z' }],
  resourceViewMode: 'active',
  teachingResources: [resource],
  teachingResourceTrash: [{ ...resource, deletedAt: '2026-10-28T00:00:00Z' }],
  contentTypeLabels: { article: '文章' },
  contentStatusLabels: { published: '已发布' },
}

test('content records preserve labels, searchable fields, counters and visibility', () => {
  const [record] = buildAccountTableRecords(base)
  assert.equal(record.title, 'A guide')
  assert.equal(record.type, '教育 · 英语')
  assert.equal(record.status, '已发布')
  assert.equal(record.visibility, '公开')
  assert.equal(record.likeCount, 2)
  assert.equal(record.favoriteCount, 3)
  assert.match(String(record.searchText), /a-guide.*Summary.*Body/)
  assert.equal(record.actions, '已发布')
})

test('content trash uses deleted timestamp and marks action as deleted', () => {
  const [record] = buildAccountTableRecords({ ...base, contentViewMode: 'trash' })
  assert.equal(record.actions, '已删除')
  assert.equal(record.updatedAt, new Date('2026-10-28T00:00:00Z').toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' }))
})

test('teaching resource records preserve visibility, metadata and trash state', () => {
  const [record] = buildAccountTableRecords({ ...base, section: 'teaching-resources' })
  assert.equal(record.filename, 'lesson.pdf')
  assert.equal(record.grade, '高中')
  assert.equal(record.visibility, '公开')
  assert.equal(record.actions, '可用')
  const [trashed] = buildAccountTableRecords({ ...base, section: 'teaching-resources', resourceViewMode: 'trash' })
  assert.equal(trashed.actions, '已删除')
})

test('unrelated account sections do not generate management records', () => {
  assert.deepEqual(buildAccountTableRecords({ ...base, section: 'overview' }), [])
})
