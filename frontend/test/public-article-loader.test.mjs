import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { createPublicArticleLoader } from '../src/composables/loadPublicArticle.ts'

function setup(request, renderMarkdown = async body => `<p>${body}</p>`) {
  const state = {
    route: ref('/content/first-article'),
    article: ref(null),
    html: ref(''),
    loading: ref(false),
    error: ref(''),
  }
  return { ...state, ...createPublicArticleLoader({ ...state, request, renderMarkdown }) }
}

const article = (slug, body = slug) => ({ item: {
  id: slug, title: slug, slug, contentType: 'article', tags: [], parentCategory: '', childCategory: '',
  summary: '', publishedAt: null, body, createdAt: '', updatedAt: '', authorName: '', authorJoinedAt: '',
  likeCount: 0, favoriteCount: 0, viewerLiked: false, viewerFavorited: false,
} })

test('public article loader requests the selected slug and renders Markdown', async () => {
  const calls = []
  const state = setup(async path => { calls.push(path); return article('first-article', '# Hello') })
  await state.loadPublicArticle('first-article')
  assert.deepEqual(calls, ['/public/content/first-article'])
  assert.equal(state.article.value.slug, 'first-article')
  assert.equal(state.html.value, '<p># Hello</p>')
  assert.equal(state.loading.value, false)
})

test('public article loader does not commit a response after the route changes', async () => {
  let resolveRequest
  const state = setup(() => new Promise(resolve => { resolveRequest = resolve }))
  const pending = state.loadPublicArticle('first-article')
  await new Promise(resolve => setImmediate(resolve))
  state.route.value = '/content/second-article'
  resolveRequest(article('first-article'))
  await pending
  assert.equal(state.article.value, null)
  assert.equal(state.html.value, '')
})

test('public article loader surfaces missing article errors and clears loading', async () => {
  const state = setup(async () => ({ error: 'Content not found' }))
  await state.loadPublicArticle('missing')
  assert.equal(state.error.value, 'Content not found')
  assert.equal(state.loading.value, false)
})

test('public article loader ignores an older response after a newer request', async () => {
  const resolvers = []
  const state = setup(() => new Promise(resolve => resolvers.push(resolve)))
  const first = state.loadPublicArticle('first-article')
  await new Promise(resolve => setImmediate(resolve))
  state.route.value = '/content/second-article'
  const second = state.loadPublicArticle('second-article')
  await new Promise(resolve => setImmediate(resolve))
  resolvers[1](article('second-article'))
  await second
  resolvers[0](article('first-article'))
  await first
  assert.equal(state.article.value.slug, 'second-article')
})
