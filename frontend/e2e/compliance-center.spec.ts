/**
 * Real Chromium baseline for the existing compliance center before framework migration.
 * Dependencies: Playwright Chromium and the local Vite preview; all API data is synthetic and isolated.
 * Collaboration: establishes stable route, overview, tab, and read-only table behavior for the native replacement.
 * Boundaries: no real credentials, production API, or persistent data is used.
 */
import { expect, test } from '@playwright/test'

const adminSession = {
  user: { id: 'e2e-admin', email: 'e2e-admin@example.test', displayName: 'E2E Admin', isAdmin: true },
}

test('compliance center loads overview and switches between assets, licenses and policy documents', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(adminSession),
  }))
  await page.route('**/api/compliance/**', async route => {
    const path = new URL(route.request().url()).pathname
    const fixtures: Record<string, unknown> = {
      '/api/compliance/overview': { assetCount: 17, pendingAssetCount: 3, highRiskCount: 2, licenseCount: 5, unreviewedLicenseCount: 1 },
      '/api/compliance/assets': { total: 1, items: [{ id: 'asset-e2e', name: 'Fixture logo', path: 'frontend/public/logo.svg', type: 'image', permissionStatus: 'unknown', riskLevel: 'medium', reviewStatus: 'pending', active: false, distributionStatus: 'not_distributed' }] },
      '/api/compliance/licenses': { items: [{ id: 'license-e2e', name: 'MIT', family: 'permissive', commercialUseAllowed: true, licenseTextCaptured: true, reviewStatus: 'reviewed' }] },
      '/api/compliance/documents': { items: [{ id: 'doc-e2e', title: '隐私政策草稿', slug: 'privacy', jurisdiction: 'unknown', version: 1, status: 'draft', missingInformation: ['controller'] }] },
      '/api/compliance/scans': { items: [] },
      '/api/compliance/audit': { items: [] },
      '/api/compliance/reviewers': { items: [] },
    }
    const result = fixtures[path]
    await route.fulfill({
      status: result ? 200 : 404,
      contentType: 'application/json',
      body: JSON.stringify(result || { error: `Unexpected compliance endpoint: ${path}` }),
    })
  })

  await page.goto('/account/compliance')
  await expect(page.getByRole('heading', { name: '门户开发资源合规中心' })).toBeVisible()
  await expect(page.locator('.cc-metrics article').first().locator('strong')).toHaveText('17')
  await expect(page.getByText('Fixture logo')).toBeVisible()

  await page.getByRole('navigation', { name: '内部资源合规管理模块' }).getByRole('button', { name: '软件许可证目录' }).click()
  await expect(page.getByRole('heading', { name: '许可证目录' })).toBeVisible()
  await expect(page.getByText('MIT', { exact: true })).toBeVisible()

  await page.getByRole('navigation', { name: '内部资源合规管理模块' }).getByRole('button', { name: '内部政策文件' }).click()
  await expect(page.getByRole('heading', { name: '政策文件' })).toBeVisible()
  await expect(page.getByText('隐私政策草稿')).toBeVisible()
})


test('native compliance view preserves asset review and policy mutation workflows', async ({ page }) => {
  const writes: Array<{ method: string; path: string; body: unknown }> = []
  await page.route('**/api/auth/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(adminSession) }))
  await page.route('**/api/compliance/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname
    const method = request.method()
    if (method !== 'GET') writes.push({ method, path, body: request.postDataJSON() })
    const fixtures: Record<string, unknown> = {
      '/api/compliance/overview': { assetCount: 1, pendingAssetCount: 1, highRiskCount: 1, licenseCount: 1, unreviewedLicenseCount: 1 },
      '/api/compliance/assets': { total: 1, items: [{ id: 'asset-native', name: 'Native fixture asset', path: 'frontend/public/native.svg', type: 'image', permissionStatus: 'unknown', riskLevel: 'high', reviewStatus: 'pending', active: false }] },
      '/api/compliance/assets/asset-native': { item: { id: 'asset-native', asset_id: 'asset-native', asset_name: 'Native fixture asset', asset_path: 'frontend/public/native.svg', review_status: 'pending', risk_level: 'high', ownership_basis: 'unknown', permission_status: 'unknown', use_scope: 'unknown', distribution_status: 'not_distributed', published_or_active: false, source_url: '', creator_or_rightsholder: '', notes: '' }, evidence: [], reviews: [] },
      '/api/compliance/licenses': { items: [{ id: 'license-native', name: 'MIT', family: 'permissive', reviewStatus: 'unreviewed', commercialUseAllowed: true, licenseTextCaptured: true }] },
      '/api/compliance/licenses/license-native': { item: { id: 'license-native', name: 'MIT', family: 'permissive', reviewStatus: 'unreviewed', fullText: 'MIT License', commercialUseAllowed: true } },
      '/api/compliance/documents': { items: [{ id: 'doc-native', title: '隐私政策草稿', slug: 'privacy', jurisdiction: 'unknown', version: 1, status: 'draft', missingInformation: ['controller'], factualAssumptions: [] }] },
      '/api/compliance/scans': { items: [] }, '/api/compliance/audit': { items: [] }, '/api/compliance/reviewers': { items: [] },
    }
    const payload = fixtures[path] ?? (method === 'PATCH' || method === 'POST' ? { ok: true, version: 2 } : null)
    await route.fulfill({ status: payload ? 200 : 404, contentType: 'application/json', body: JSON.stringify(payload || { error: `Unexpected endpoint: ${path}` }) })
  })

  await page.goto('/account/compliance')
  await page.getByRole('button', { name: '审核详情' }).click()
  await expect(page.getByRole('dialog', { name: '资产审核详情' })).toBeVisible()
  await page.locator('[data-form="asset"] textarea[name="notes"]').fill('reviewed in Chromium')
  await page.locator('[data-form="asset"] button[type="submit"]').click()
  await expect.poll(() => writes.some(write => write.path.endsWith('/assets/asset-native') && write.method === 'PATCH' && (write.body as any)?.notes === 'reviewed in Chromium')).toBe(true)

  await page.getByRole('button', { name: '关闭' }).click()
  await page.getByRole('button', { name: '内部政策文件' }).click()
  await page.getByRole('button', { name: '编辑/审核' }).click()
  await page.locator('[data-form="document"] textarea[name="changeNote"]').fill('Native view E2E')
  await page.locator('[data-form="document"] button[type="submit"]').click()
  await expect.poll(() => writes.some(write => write.path.endsWith('/documents/privacy') && write.method === 'PATCH' && (write.body as any)?.changeNote === 'Native view E2E')).toBe(true)
})
