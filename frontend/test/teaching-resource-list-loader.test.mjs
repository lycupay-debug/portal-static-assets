import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { createTeachingResourceListLoader } from '../src/composables/loadTeachingResourceList.ts'

function setup(request, onError) {
  const state = { viewMode: ref('active'), activeItems: ref([]), trashItems: ref([]), loading: ref(false), error: ref('') }
  return { ...state, ...createTeachingResourceListLoader({ ...state, request, onError }) }
}
const resource = id => ({ id, title: id, description: '', type: '其他', grade: '', isPublic: false, url: '', createdAt: '', updatedAt: '' })

test('teaching resource loader trims search and fills active records', async () => {
  const calls = []
  const state = setup(async path => { calls.push(path); return { items: [resource('active')] } })
  await state.loadTeachingResources('  math  ')
  assert.deepEqual(calls, ['/resources?q=math'])
  assert.equal(state.activeItems.value[0].id, 'active')
  assert.equal(state.loading.value, false)
})

test('teaching resource loader uses trash endpoint and protects view changes', async () => {
  const resolvers = []
  const state = setup(path => new Promise(resolve => resolvers.push({ path, resolve })))
  const pending = state.loadTeachingResources()
  await new Promise(resolve => setImmediate(resolve))
  state.viewMode.value = 'trash'
  resolvers[0].resolve({ items: [resource('stale-active')] })
  await pending
  assert.equal(state.activeItems.value.length, 0)
  const trashPending = state.loadTeachingResources()
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(resolvers[1].path, '/resources/trash')
  resolvers[1].resolve({ items: [resource('deleted')] })
  await trashPending
  assert.equal(state.trashItems.value[0].id, 'deleted')
})

test('teaching resource loader reports current request failures', async () => {
  const messages = []
  const state = setup(async () => { throw new Error('server unavailable') }, message => messages.push(message))
  await state.loadTeachingResources()
  assert.equal(state.error.value, 'server unavailable')
  assert.deepEqual(messages, ['server unavailable'])
  assert.equal(state.loading.value, false)
})
