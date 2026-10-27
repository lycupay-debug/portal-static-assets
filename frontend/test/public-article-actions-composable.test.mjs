import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { ApiRequestError } from '../src/api/client.ts'
import { usePublicArticleActions } from '../src/composables/usePublicArticleActions.ts'

const makeArticle = () => ({ slug: 'guide-1', likeCount: 2, favoriteCount: 3, viewerLiked: false, viewerFavorited: false })
function setup(overrides = {}) {
  const article = ref(makeArticle())
  const busy = ref(false)
  const authNotice = ref('')
  const shareDialogOpen = ref(false)
  const shareUrl = ref('')
  const calls = []
  const messages = { success: [], error: [] }
  const options = {
    article, busy, authNotice, shareDialogOpen, shareUrl,
    navigate: async target => calls.push(['navigate', target]),
    request: async (path, init) => { calls.push([path, init]); return { active: true, likeCount: 7, favoriteCount: 8 } },
    notify: { success: message => messages.success.push(message), error: message => messages.error.push(message) },
    ...overrides,
  }
  return { article, busy, authNotice, shareDialogOpen, shareUrl, calls, messages, actions: usePublicArticleActions(options) }
}

test('public article interaction updates counts and viewer state', async () => {
  const state = setup()
  await state.actions.toggleInteraction('like')
  assert.equal(state.article.value.likeCount, 7)
  assert.equal(state.article.value.favoriteCount, 8)
  assert.equal(state.article.value.viewerLiked, true)
  assert.equal(state.calls[0][1].method, 'POST')
  assert.deepEqual(state.messages.success, ['已点赞'])
  assert.equal(state.busy.value, false)
})

test('public article interaction redirects unauthenticated users to login', async () => {
  const state = setup({ request: async () => { throw new ApiRequestError('Unauthorized', 401, null) } })
  await state.actions.toggleInteraction('favorite')
  assert.equal(state.authNotice.value, '请先登录，再进行点赞或收藏。')
  assert.deepEqual(state.calls, [['navigate', '/login']])
  assert.equal(state.busy.value, false)
})

test('public article interaction ignores duplicate calls while busy', async () => {
  const state = setup()
  state.busy.value = true
  await state.actions.toggleInteraction('like')
  assert.deepEqual(state.calls, [])
})
