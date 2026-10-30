/** Framework-neutral tests for account table derivation and virtualized-window calculations. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createAccountTableModel } from '../src/state/accountTableModel'
import type { MutableValue } from '../src/state/valueContracts'
function cell<T>(value: T): MutableValue<T> { return { value } }

function createModel() {
  const state = {
    activeAccountSection: cell('content-management'), contentViewMode: cell<'active' | 'trash'>('active'),
    contentItems: cell<any[]>([
      { id: '1', title: 'First article', slug: 'first-article', contentType: 'article', status: 'draft', visibility: 'private', updatedAt: '2026-10-01T00:00:00Z', summary: 'alpha', body: 'body', tags: ['education'], likeCount: 2, favoriteCount: 1 },
      { id: '2', title: 'Second article', slug: 'second-article', contentType: 'tool', status: 'published', visibility: 'public', updatedAt: '2026-10-02T00:00:00Z', summary: 'beta', body: 'body', tags: ['tools'], likeCount: 3, favoriteCount: 2 },
    ]),
    contentTrashItems: cell<any[]>([]), resourceViewMode: cell<'active' | 'trash'>('active'),
    teachingResources: cell<any[]>([]), teachingResourceTrash: cell<any[]>([]), debouncedTableQuery: cell(''),
    tableScrollTop: cell(0), virtualTableViewportWidth: cell(800),
  }
  const model = createAccountTableModel({ ...state, screenWidth: () => 1200 })
  return { state, model }
}

test('account table model derives the content schema, rows and responsive column template', () => {
  const { model } = createModel()
  assert.equal(model.activeTableDefinition.value?.title, '内容列表')
  assert.equal(model.filteredTableRecords.value.length, 2)
  assert.equal(model.filteredTableRecords.value[0]?.title, 'First article')
  assert.match(model.tableColumnTemplate.value, /minmax\(330px, max-content\)/)
  assert.ok(model.tableColumnTemplate.value.split(' ').length >= 8)
})

test('account table filtering uses normalized search data and resets the virtual window', () => {
  const { state, model } = createModel()
  state.debouncedTableQuery.value = 'second'
  assert.equal(model.filteredTableRecords.value.length, 1)
  assert.equal(model.filteredTableRecords.value[0]?.id, '2')
  state.debouncedTableQuery.value = ''
  state.tableScrollTop.value = 10000
  assert.equal(model.virtualTableStart.value, 0)
  assert.equal(model.visibleTableRecords.value.length, 2)
  assert.equal(model.virtualTableTopSpace.value, 0)
  assert.equal(model.virtualTableBottomSpace.value, 0)
})

test('account table model switches to resource records and unknown sections have no table', () => {
  const { state, model } = createModel()
  state.activeAccountSection.value = 'teaching-resources'
  state.teachingResources.value = [{ id: 'r1', title: 'Lesson', filename: 'lesson.pdf', type: '课件', grade: '小学', isPublic: true, description: 'slides', createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-02T00:00:00Z', url: '' }]
  assert.equal(model.activeTableDefinition.value?.title, '教学资源')
  assert.equal(model.filteredTableRecords.value[0]?.filename, 'lesson.pdf')
  state.activeAccountSection.value = 'not-a-table'
  assert.equal(model.activeTableDefinition.value, null)
  assert.equal(model.filteredTableRecords.value.length, 0)
})
