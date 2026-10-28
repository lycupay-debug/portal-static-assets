import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { createContentListLoader } from '../src/composables/loadContentList.ts'

function setup(request) {
  const state = {
    viewMode: ref('active'),
    activeItems: ref([]),
    trashItems: ref([]),
    loading: ref(false),
    error: ref(''),
  }
  return { ...state, ...createContentListLoader({ ...state, request }) }
}
const item = id => ({ id, title: id, slug: id, contentType: 'article', tags: [], parentCategory: '', childCategory: '', summary: '', body: '', visibility: 'private', publishedAt: null, createdAt: '', updatedAt: '' })

test('content list loader trims search and updates active records', async () => {
  const calls = []
  const state = setup(async path => { calls.push(path); return { items: [item('one')] } })
  await state.loadContentList('  handbook  ')
  assert.deepEqual(calls, ['/content?q=handbook'])
  assert.equal(state.activeItems.value[0].id, 'one')
  assert.equal(state.loading.value, false)
})

test('content list loader routes trash view to the trash collection', async () => {
  const calls = []
  const state = setup(async path => { calls.push(path); return { items: [item('deleted')] } })
  state.viewMode.value = 'trash'
  await state.loadContentList()
  assert.deepEqual(calls, ['/content/trash'])
  assert.equal(state.trashItems.value[0].id, 'deleted')
})

test('content list loader ignores stale responses and exposes current request errors', async () => {
  const resolvers = []
  const state = setup(() => new Promise(resolve => resolvers.push(resolve)))
  const older = state.loadContentList('old')
  const newer = state.loadContentList('new')
  await new Promise(resolve => setImmediate(resolve))
  resolvers[1]({ items: [item('new')] })
  await newer
  resolvers[0]({ items: [item('old')] })
  await older
  assert.equal(state.activeItems.value[0].id, 'new')
  const failed = setup(async () => { throw new Error('permission denied') })
  await failed.loadContentList()
  assert.equal(failed.error.value, 'permission denied')
  assert.equal(failed.loading.value, false)
})
