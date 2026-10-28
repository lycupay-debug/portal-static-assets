import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { createPublicDirectoryLoader } from '../src/composables/loadPublicDirectoryData.ts'

function setup(request) {
  const state = {
    publicContents: ref([]),
    publicTeachingResources: ref([]),
    publicContentLoading: ref(false),
    publicContentError: ref(''),
    educationResourceLoading: ref(false),
    educationResourceError: ref(''),
    knowledgeSearchQuery: ref(''),
    educationResourceSearch: ref(''),
    knowledgeVisibleCount: ref(30),
    knowledgePageSize: 10,
  }
  const data = createPublicDirectoryLoader({ ...state, request })
  return { ...state, ...data }
}

test('public content loader trims search, updates results, and resets visible page size', async () => {
  const calls = []
  const state = setup(async (path) => { calls.push(path); return { items: [{ id: '1', title: 'A' }] } })
  await state.loadPublicContents('  handbook  ')
  assert.deepEqual(calls, ['/public/content?q=handbook'])
  assert.equal(state.publicContents.value[0].id, '1')
  assert.equal(state.knowledgeVisibleCount.value, 10)
  assert.equal(state.publicContentLoading.value, false)
  assert.equal(state.publicContentError.value, '')
})

test('public content loader ignores stale responses and preserves the latest result', async () => {
  const resolvers = []
  const state = setup(() => new Promise(resolve => resolvers.push(resolve)))
  const oldRequest = state.loadPublicContents('old')
  const latestRequest = state.loadPublicContents('latest')
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(resolvers.length, 2)
  resolvers[1]({ items: [{ id: 'latest' }] })
  await latestRequest
  resolvers[0]({ items: [{ id: 'old' }] })
  await oldRequest
  assert.deepEqual(state.publicContents.value.map(item => item.id), ['latest'])
  assert.equal(state.publicContentLoading.value, false)
})

test('public resource loader reports API errors and clears loading state', async () => {
  const state = setup(async () => { throw new Error('resource API unavailable') })
  await state.loadPublicTeachingResources(' math ')
  assert.equal(state.educationResourceError.value, 'resource API unavailable')
  assert.equal(state.educationResourceLoading.value, false)
})

test('public resource loader trims search terms before sending the request', async () => {
  const calls = []
  const state = setup(async path => { calls.push(path); return { items: [] } })
  await state.loadPublicTeachingResources('  algebra  ')
  assert.deepEqual(calls, ['/public/resources?q=algebra'])
})
