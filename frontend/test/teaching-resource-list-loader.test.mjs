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


test('teaching resource loader keeps the newest search result and suppresses stale failures', async () => {
  const pending = []
  const messages = []
  const state = setup((path) => new Promise((resolve, reject) => pending.push({ path, resolve, reject })), message => messages.push(message))
  const older = state.loadTeachingResources('algebra')
  const newer = state.loadTeachingResources('biology')
  await new Promise(resolve => setImmediate(resolve))
  assert.deepEqual(pending.map(item => item.path), ['/resources?q=algebra', '/resources?q=biology'])

  pending[1].resolve({ items: [resource('biology-current')] })
  await newer
  assert.equal(state.activeItems.value[0].id, 'biology-current')
  assert.equal(state.loading.value, false)

  pending[0].reject(new Error('stale algebra failure'))
  await older
  assert.equal(state.activeItems.value[0].id, 'biology-current')
  assert.equal(state.error.value, '')
  assert.deepEqual(messages, [])
  assert.equal(state.loading.value, false)
})

test('teaching resource loader never lets a stale active request overwrite trash view data', async () => {
  const pending = []
  const state = setup(path => new Promise(resolve => pending.push({ path, resolve })))
  const activeRequest = state.loadTeachingResources('old query')
  await new Promise(resolve => setImmediate(resolve))
  state.viewMode.value = 'trash'
  const trashRequest = state.loadTeachingResources()
  await new Promise(resolve => setImmediate(resolve))

  pending[1].resolve({ items: [resource('trash-current')] })
  await trashRequest
  pending[0].resolve({ items: [resource('stale-active')] })
  await activeRequest

  assert.equal(state.trashItems.value[0].id, 'trash-current')
  assert.equal(state.activeItems.value.length, 0)
  assert.equal(state.loading.value, false)
})
