/**
 * Regression tests for the framework-independent compliance-center controller.
 * Dependencies: Node test/assert APIs and synthetic same-origin fetch responses.
 * Collaboration: protects API contracts and state transitions before the native DOM view is mounted.
 * Boundaries: no real credentials, production API, or persistent data; server authorization remains authoritative.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createComplianceCenterController } from '../src/components/complianceCenterController'

type Call = { url: string; method: string; body?: string }
function setup(options: { isAdmin?: boolean } = {}) {
  const calls: Call[] = []
  const originalFetch = globalThis.fetch
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input)
    const method = init.method || 'GET'
    const body = typeof init.body === 'string' ? init.body : undefined
    calls.push({ url, method, body })
    let payload: unknown = {}
    if (url.endsWith('/compliance/overview')) payload = { assetCount: 17, pendingAssetCount: 3 }
    else if (url.includes('/compliance/assets?')) payload = { total: 1, items: [{ id: 'asset-1', name: 'Logo' }] }
    else if (url.endsWith('/compliance/assets/asset-1') && method === 'GET') {
      payload = {
        item: { asset_id: 'asset-1', review_status: 'pending', risk_level: 'high', ownership_basis: 'unknown', permission_status: 'unknown', use_scope: 'unknown', distribution_status: 'not_distributed', published_or_active: false },
        evidence: [], reviews: [],
      }
    } else if (url.endsWith('/compliance/assets/asset-1') && method === 'PATCH') payload = { ok: true }
    else if (url.endsWith('/compliance/assets/asset-1/reviews') && method === 'POST') payload = { id: 'review-1' }
    else if (url.endsWith('/compliance/assets/asset-1/evidence') && method === 'POST') payload = { id: 'evidence-1' }
    else if (url.endsWith('/compliance/assets/asset-1/evidence/evidence-1') && method === 'PATCH') payload = { verified: true }
    else if (url.endsWith('/compliance/licenses')) payload = { items: [] }
    else if (url.endsWith('/compliance/documents')) payload = { items: [{ slug: 'privacy', status: 'draft' }] }
    else if (url.endsWith('/compliance/scans')) payload = { items: [] }
    else if (url.endsWith('/compliance/audit')) payload = { items: [] }
    else if (url.endsWith('/compliance/reviewers')) payload = { items: [{ id: 'reviewer-1', email: 'reviewer@example.test', isCopyrightReviewer: false }] }
    else if (url.endsWith('/compliance/reviewers/reviewer-1')) payload = { item: { id: 'reviewer-1', email: 'reviewer@example.test', isCopyrightReviewer: true } }
    else if (url.endsWith('/compliance/licenses/license-1')) payload = { item: { id: 'license-1', reviewStatus: 'unreviewed', family: 'permissive' } }
    else if (url.endsWith('/compliance/documents/privacy') && method === 'PATCH') payload = { version: 2 }
    return new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as typeof fetch
  const controller = createComplianceCenterController(options)
  return {
    calls, controller,
    restore() { controller.dispose(); globalThis.fetch = originalFetch },
  }
}

test('refresh loads all compliance ledgers and only requests reviewer identities for administrators', async () => {
  const admin = setup({ isAdmin: true })
  try {
    await admin.controller.ready
    assert.equal(admin.controller.state.overview.assetCount, 17)
    assert.equal(admin.controller.state.assetsTotal, 1)
    assert.equal(admin.controller.state.reviewerUsers.length, 1)
    assert.ok(admin.calls.some(call => call.url.endsWith('/compliance/reviewers')))
  } finally { admin.restore() }

  const reviewer = setup({ isAdmin: false })
  try {
    await reviewer.controller.ready
    assert.equal(reviewer.controller.state.reviewerUsers.length, 0)
    assert.equal(reviewer.calls.some(call => call.url.endsWith('/compliance/reviewers')), false)
  } finally { reviewer.restore() }
})

test('asset update, review creation and evidence verification preserve their API contracts', async () => {
  const context = setup()
  try {
    await context.controller.ready
    await context.controller.openAsset({ id: 'asset-1' })
    assert.equal(context.controller.state.selectedAsset?.id, 'asset-1')
    context.controller.state.assetForm.notes = 'verified source'
    await context.controller.saveAsset()
    const assetPatch = context.calls.find(call => call.url.endsWith('/compliance/assets/asset-1') && call.method === 'PATCH')
    assert.equal(JSON.parse(assetPatch?.body || '{}').notes, 'verified source')

    context.controller.state.reviewForm.findings = 'License text checked'
    context.controller.state.reviewForm.actions = 'Archive source\nConfirm redistribution'
    await context.controller.saveReview()
    const review = context.calls.find(call => call.url.endsWith('/reviews') && call.method === 'POST')
    assert.deepEqual(JSON.parse(review?.body || '{}').requiredActions, ['Archive source', 'Confirm redistribution'])

    context.controller.state.evidenceForm.text = 'MIT license captured'
    await context.controller.saveEvidence()
    const evidence = context.calls.find(call => call.url.endsWith('/evidence') && call.method === 'POST')
    assert.equal(JSON.parse(evidence?.body || '{}').text, 'MIT license captured')

    await context.controller.verifyEvidence({ id: 'evidence-1' }, true)
    const verification = context.calls.find(call => call.url.endsWith('/evidence/evidence-1') && call.method === 'PATCH')
    assert.deepEqual(JSON.parse(verification?.body || '{}'), { verified: true })
  } finally { context.restore() }
})

test('reviewer role updates are admin-only and explicit', async () => {
  const context = setup({ isAdmin: true })
  try {
    await context.controller.ready
    await context.controller.setReviewerRole(context.controller.state.reviewerUsers[0], true)
    const update = context.calls.find(call => call.url.endsWith('/reviewers/reviewer-1') && call.method === 'PATCH')
    assert.deepEqual(JSON.parse(update?.body || '{}'), { isCopyrightReviewer: true })
    assert.equal(context.controller.state.reviewerUsers[0].isCopyrightReviewer, true)
  } finally { context.restore() }
})

test('policy document changes are sent as explicit PATCH with normalized multiline fields', async () => {
  const context = setup()
  try {
    await context.controller.ready
    context.controller.openDocument({
      slug: 'privacy', status: 'draft', bodyMarkdown: '# Privacy',
      missingInformation: ['controller'], factualAssumptions: ['retention pending'],
    })
    context.controller.state.documentForm.missingInformation = 'contact\ncontroller'
    context.controller.state.documentForm.factualAssumptions = 'retention confirmed'
    context.controller.state.documentForm.changeNote = 'Clarify contact and retention'
    await context.controller.saveDocument()
    const update = context.calls.find(call => call.url.endsWith('/compliance/documents/privacy') && call.method === 'PATCH')
    const payload = JSON.parse(update?.body || '{}')
    assert.deepEqual(payload.missingInformation, ['contact', 'controller'])
    assert.deepEqual(payload.factualAssumptions, ['retention confirmed'])
    assert.equal(payload.changeNote, 'Clarify contact and retention')
    assert.equal(context.controller.state.feedback, '政策已保存，版本 2。')
  } finally { context.restore() }
})
