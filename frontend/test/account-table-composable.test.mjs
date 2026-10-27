import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useAccountTable } from '../src/composables/useAccountTable.ts'

const content = (id, title) => ({
  id, title, slug: id, summary: 'Summary', body: 'Body', contentType: 'article',
  status: 'published', visibility: 'public', tags: ['教育', '英语'], parentCategory: '教育',
  childCategory: '英语', updatedAt: '2026-10-27T12:00:00Z', likeCount: 0, favoriteCount: 0,
})

function createTable(count = 3) {
  return useAccountTable({
    activeAccountSection: ref('content-management'),
    contentViewMode: ref('active'),
    contentItems: ref(Array.from({ length: count }, (_, index) => content(`content-${index}`, `Guide ${index}`))),
    contentTrashItems: ref([]),
    resourceViewMode: ref('active'),
    teachingResources: ref([]),
    teachingResourceTrash: ref([]),
    debouncedTableQuery: ref(''),
    tableScrollTop: ref(0),
    virtualTableViewportWidth: ref(800),
  })
}

test('account table composable derives records and a bounded visible window', () => {
  const table = createTable(3)
  assert.equal(table.activeTableDefinition.value.title.length > 0, true)
  assert.equal(table.filteredTableRecords.value.length, 3)
  assert.equal(table.visibleTableRecords.value.length, 3)
  assert.equal(table.virtualTableTopSpace.value, 0)
  assert.equal(table.virtualTableBottomSpace.value, 0)
})

test('account table composable reacts to the debounced search query', () => {
  const query = ref('')
  const table = useAccountTable({
    activeAccountSection: ref('content-management'), contentViewMode: ref('active'),
    contentItems: ref([content('one', 'Guide one'), content('two', 'Reference two')]),
    contentTrashItems: ref([]), resourceViewMode: ref('active'), teachingResources: ref([]),
    teachingResourceTrash: ref([]), debouncedTableQuery: query, tableScrollTop: ref(0),
    virtualTableViewportWidth: ref(800),
  })
  query.value = 'reference'
  assert.equal(table.filteredTableRecords.value.length, 1)
  assert.equal(table.visibleTableRecords.value[0].title, 'Reference two')
  query.value = 'no match'
  assert.equal(table.filteredTableRecords.value.length, 0)
  assert.equal(table.virtualTableBottomSpace.value, 0)
})
