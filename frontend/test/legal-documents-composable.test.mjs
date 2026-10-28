import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useLegalDocuments } from '../src/composables/useLegalDocuments.ts'

const document = (slug, bodyMarkdown = `# ${slug}`) => ({
  slug,
  title: `Policy ${slug}`,
  bodyMarkdown,
  effectiveAt: null,
})

test('legal document loader derives valid route slug and loads rendered content', async () => {
  const route = ref('/legal/privacy-policy')
  const calls = []
  const legal = useLegalDocuments(route, async (path) => {
    calls.push(path)
    return { item: document('privacy-policy') }
  }, markdown => `<article>${markdown}</article>`)

  assert.equal(legal.legalSlug.value, 'privacy-policy')
  route.value = '/legal/INVALID_slug'
  assert.equal(legal.legalSlug.value, '')
  route.value = '/legal/privacy-policy'
  await legal.loadLegalDocument(legal.legalSlug.value)

  assert.deepEqual(calls, ['/public/legal/privacy-policy'])
  assert.equal(legal.legalDocument.value.slug, 'privacy-policy')
  assert.equal(legal.legalDocumentHtml.value, '<article># privacy-policy</article>')
  assert.equal(legal.legalDocumentLoading.value, false)
  assert.equal(legal.legalDocumentError.value, '')
})

test('legal document loader ignores a response after route changes', async () => {
  const route = ref('/legal/old-policy')
  let resolveRequest
  const legal = useLegalDocuments(route, () => new Promise(resolve => { resolveRequest = resolve }), markdown => markdown)
  const pending = legal.loadLegalDocument('old-policy')
  await new Promise(resolve => setImmediate(resolve))
  route.value = '/legal/new-policy'
  resolveRequest({ item: document('old-policy') })
  await pending

  assert.equal(legal.legalDocument.value, null)
  assert.equal(legal.legalDocumentHtml.value, '')
  assert.equal(legal.legalDocumentError.value, '')
  assert.equal(legal.legalDocumentLoading.value, false)
})

test('legal document loader reports API and missing-document failures', async () => {
  const route = ref('/legal/unknown-policy')
  const apiFailure = useLegalDocuments(route, async () => { throw new Error('API unavailable') }, markdown => markdown)
  await apiFailure.loadLegalDocument('unknown-policy')
  assert.equal(apiFailure.legalDocumentError.value, 'API unavailable')
  assert.equal(apiFailure.legalDocumentLoading.value, false)

  const missing = useLegalDocuments(route, async () => ({ error: 'not published' }), markdown => markdown)
  await missing.loadLegalDocument('unknown-policy')
  assert.equal(missing.legalDocumentError.value, 'not published')
})

test('published policy list failures stay non-fatal and successful lists replace state', async () => {
  const route = ref('/')
  const legal = useLegalDocuments(route, async () => ({ items: [{ slug: 'privacy', title: 'Privacy', documentType: 'privacy' }] }), markdown => markdown)
  await legal.loadPublishedLegalDocuments()
  assert.deepEqual(legal.publishedLegalDocuments.value, [{ slug: 'privacy', title: 'Privacy', documentType: 'privacy' }])

  const unavailable = useLegalDocuments(route, async () => { throw new Error('offline') }, markdown => markdown)
  await assert.doesNotReject(unavailable.loadPublishedLegalDocuments())
  assert.deepEqual(unavailable.publishedLegalDocuments.value, [])
})
