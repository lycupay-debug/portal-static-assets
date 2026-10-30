/**
 * Native compliance view regression tests.
 * Dependencies: jsdom, Node test APIs, and synthetic same-origin API fixtures.
 * Collaboration: verifies native DOM mounting, safe rendering, explicit mutations, and disposal around the controller.
 * Boundaries: no credentials, live API, persistent database, or Vue runtime is used.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountComplianceCenter } from '../src/components/complianceCenterView'

function setup() {
  const dom = new JSDOM('<!doctype html><main id="host"></main>', { url: 'https://portal.example.test/account/compliance' })
  const originalFetch = globalThis.fetch
  const calls: Array<{ path: string; method: string; body?: string }> = []
  globalThis.window = dom.window as unknown as Window & typeof globalThis
  globalThis.document = dom.window.document
  globalThis.HTMLElement = dom.window.HTMLElement
  globalThis.HTMLFormElement = dom.window.HTMLFormElement
  globalThis.HTMLInputElement = dom.window.HTMLInputElement
  globalThis.Element = dom.window.Element
  globalThis.FormData = dom.window.FormData
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = new URL(String(input), 'https://portal.example.test')
    const method = init.method || 'GET'
    calls.push({ path: url.pathname + url.search, method, body: typeof init.body === 'string' ? init.body : undefined })
    let payload: unknown = {}
    if (url.pathname.endsWith('/compliance/overview')) payload = { assetCount: 1, pendingAssetCount: 1, highRiskCount: 1, licenseCount: 1 }
    else if (url.pathname.endsWith('/compliance/assets') && method === 'GET') payload = { total: 1, items: [{ id: 'asset-1', name: '<img src=x onerror=alert(1)>', path: 'logo.svg', type: 'image', permissionStatus: 'unknown', riskLevel: 'high', reviewStatus: 'pending', active: false }] }
    else if (url.pathname.endsWith('/compliance/assets/asset-1') && method === 'GET') payload = { item: { id: 'asset-1', asset_id: 'asset-1', asset_name: 'Fixture asset', asset_path: 'logo.svg', review_status: 'pending', risk_level: 'high', ownership_basis: 'unknown', permission_status: 'unknown', use_scope: 'unknown', distribution_status: 'not_distributed', published_or_active: false, source_url: '', creator_or_rightsholder: '', notes: '' }, evidence: [], reviews: [] }
    else if (url.pathname.endsWith('/compliance/assets/asset-1') && method === 'PATCH') payload = { ok: true }
    else if (url.pathname.endsWith('/compliance/assets/asset-1/reviews')) payload = { id: 'review-1' }
    else if (url.pathname.endsWith('/compliance/assets/asset-1/evidence')) payload = { id: 'evidence-1' }
    else if (url.pathname.endsWith('/compliance/licenses')) payload = { items: [{ id: 'license-1', name: 'MIT', family: 'permissive', reviewStatus: 'unreviewed', commercialUseAllowed: true }] }
    else if (url.pathname.endsWith('/compliance/licenses/license-1')) payload = { item: { id: 'license-1', name: 'MIT', reviewStatus: 'unreviewed', family: 'permissive', fullText: 'MIT License' } }
    else if (url.pathname.endsWith('/compliance/documents')) payload = { items: [{ id: 'doc-1', title: 'Privacy draft', slug: 'privacy', status: 'draft', version: 1, missingInformation: ['controller'] }] }
    else if (url.pathname.endsWith('/compliance/documents/privacy')) payload = { version: 2 }
    else if (url.pathname.endsWith('/compliance/scans')) payload = { items: [] }
    else if (url.pathname.endsWith('/compliance/audit')) payload = { items: [] }
    else if (url.pathname.endsWith('/compliance/reviewers')) payload = { items: [{ id: 'reviewer-1', email: 'reviewer@example.test', isCopyrightReviewer: false }] }
    else if (url.pathname.endsWith('/compliance/reviewers/reviewer-1')) payload = { item: { id: 'reviewer-1', email: 'reviewer@example.test', isCopyrightReviewer: true } }
    return new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as typeof fetch
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const view = mountComplianceCenter(host, true)
  return { dom, host, calls, view, restore() { view.dispose(); globalThis.fetch = originalFetch; dom.window.close() } }
}

test('native view renders all modules and treats untrusted asset text as text', async () => {
  const ctx = setup()
  try {
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.match(ctx.host.textContent || '', /门户开发资源合规中心/)
    assert.match(ctx.host.textContent || '', /软件许可证目录/)
    assert.equal(ctx.host.querySelector('img'), null)
    assert.match(ctx.host.textContent || '', /<img src=x onerror=alert\(1\)>/)
    for (const label of ['门户资源台账', '软件许可证目录', '内部政策文件', '开发资源扫描', '操作日志', '审核人员权限']) {
      assert.ok(ctx.host.textContent?.includes(label))
    }
  } finally { ctx.restore() }
})

test('native asset detail submits only explicit PATCH and POST actions', async () => {
  const ctx = setup()
  try {
    await new Promise(resolve => setTimeout(resolve, 0))
    ctx.host.querySelector<HTMLButtonElement>('[data-action="asset"]')!.click()
    await new Promise(resolve => setTimeout(resolve, 0))
    const assetForm = ctx.host.querySelector<HTMLFormElement>('[data-form="asset"]')!
    ;(assetForm.elements.namedItem('notes') as HTMLTextAreaElement).value = 'checked by reviewer'
    assetForm.dispatchEvent(new ctx.dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    const patch = ctx.calls.find(call => call.path.endsWith('/compliance/assets/asset-1') && call.method === 'PATCH')
    assert.equal(JSON.parse(patch?.body || '{}').notes, 'checked by reviewer')
    assert.ok(ctx.calls.some(call => call.path.endsWith('/compliance/assets/asset-1') && call.method === 'PATCH'))
    const evidenceForm = ctx.host.querySelector<HTMLFormElement>('[data-form="evidence"]')!
    ;(evidenceForm.elements.namedItem('text') as HTMLTextAreaElement).value = 'MIT text captured'
    evidenceForm.dispatchEvent(new ctx.dom.window.Event('submit', { bubbles: true, cancelable: true }))
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.ok(ctx.calls.some(call => call.path.endsWith('/compliance/assets/asset-1/evidence') && call.method === 'POST'))
  } finally { ctx.restore() }
})
