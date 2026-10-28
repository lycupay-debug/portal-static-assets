import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useContentManagementActions } from '../src/composables/useContentManagementActions.ts'

const contentItem = (id = 'content-1') => ({
  id,
  title: 'A title',
  slug: 'a-title',
  contentType: 'article',
  tags: ['language'],
  parentCategory: null,
  childCategory: null,
  summary: 'summary',
  body: 'body',
  visibility: 'private',
  status: 'draft',
})

function setup(overrides = {}) {
  const state = {
    routeContentId: ref(null),
    contentItems: ref([contentItem()]),
    contentTrashItems: ref([{ ...contentItem('deleted-1'), title: 'Deleted item' }]),
    contentBusy: ref(false),
    contentError: ref(''),
    contentNotice: ref(''),
    contentEditorOpen: ref(false),
    editingContentId: ref(null),
    contentForm: ref({ title: '', slug: '', contentType: 'article', tags: [], summary: '', body: '', visibility: 'private' }),
    contentVersions: ref([]),
    versionsContentId: ref(null),
    versionsContentTitle: ref(''),
  }
  const calls = []
  const navigations = []
  const listLoads = []
  const options = {
    ...state,
    emptyContentForm: () => ({ title: '', slug: '', contentType: 'article', tags: [], summary: '', body: '', visibility: 'private' }),
    loadContentList: async () => { listLoads.push('load') },
    navigate: async target => { navigations.push(target) },
    request: async (path, init, message) => {
      calls.push({ path, init, message })
      return { item: contentItem(), items: [] }
    },
    ...overrides,
  }
  return { ...state, calls, navigations, listLoads, actions: useContentManagementActions(options) }
}

test('content editor action validates required fields without sending a request', async () => {
  const state = setup()
  await state.actions.saveContent()
  assert.equal(state.contentError.value, '请填写内容标题。')
  assert.equal(state.calls.length, 0)
  assert.equal(state.contentBusy.value, false)
})

test('content editor action creates content, clears editor state and navigates to list', async () => {
  const state = setup()
  state.contentForm.value = {
    title: '  New title ',
    slug: 'new-title',
    contentType: 'article',
    tags: ['language'],
    summary: 'Summary',
    body: 'Body',
    visibility: 'private',
  }
  await state.actions.saveContent()
  assert.equal(state.calls.length, 1)
  assert.equal(state.calls[0].path, '/content')
  assert.equal(state.calls[0].init.method, 'POST')
  assert.equal(JSON.parse(state.calls[0].init.body).title, 'New title')
  assert.equal(state.editingContentId.value, 'content-1')
  assert.equal(state.contentEditorOpen.value, false)
  assert.deepEqual(state.navigations, ['/content-management'])
  assert.equal(state.contentNotice.value, '草稿已创建。')
  assert.equal(state.contentBusy.value, false)
})

test('content editor route resets a new document form when there is no route id', async () => {
  const state = setup()
  state.editingContentId.value = 'old-id'
  state.contentForm.value.title = 'Old content'
  await state.actions.initializeContentEditorRoute()
  assert.equal(state.editingContentId.value, null)
  assert.equal(state.contentForm.value.title, '')
  assert.equal(state.contentEditorOpen.value, false)
  assert.equal(state.calls.length, 0)
})


test('content editor route loads the requested item into the form', async () => {
  const state = setup({ routeContentId: ref('content-1') })
  await state.actions.initializeContentEditorRoute()
  assert.equal(state.calls[0].path, '/content/content-1')
  assert.equal(state.editingContentId.value, 'content-1')
  assert.equal(state.contentForm.value.title, 'A title')
  assert.equal(state.contentForm.value.body, 'body')
  assert.equal(state.contentBusy.value, false)
})

test('content status action reports failed API responses and clears busy state', async () => {
  const state = setup({ request: async () => { throw new Error('publish unavailable') } })
  await state.actions.changeContentStatus('content-1', 'publish')
  assert.equal(state.contentError.value, 'publish unavailable')
  assert.equal(state.contentBusy.value, false)
})

test('opening content versions records the requested content and always clears busy state', async () => {
  const state = setup({
    request: async (path, init, message) => {
      stateCalls.push({ path, init, message })
      return { items: [{ versionId: 'version-1', versionNumber: 3 }] }
    },
  })
  const stateCalls = state.calls
  await state.actions.openContentVersions('content-1')
  assert.equal(stateCalls[0].path, '/content/content-1/versions')
  assert.equal(state.versionsContentId.value, 'content-1')
  assert.equal(state.versionsContentTitle.value, 'A title')
  assert.equal(state.contentVersions.value[0].versionNumber, 3)
  assert.equal(state.contentBusy.value, false)
})

test('content version request errors are surfaced and busy state is released', async () => {
  const state = setup({
    request: async () => { throw new Error('version endpoint unavailable') },
  })
  await state.actions.openContentVersions('content-1')
  assert.equal(state.contentError.value, 'version endpoint unavailable')
  assert.equal(state.contentBusy.value, false)
})
