import { expect, test } from '@playwright/test'

const adminSession = {
  user: {
    id: 'e2e-admin',
    email: 'e2e-admin@example.test',
    displayName: 'E2E Admin',
    isAdmin: true,
  },
}

test.beforeEach(async ({ page }) => {
  // Isolated browser-only session fixture. No real account, password, cookie or production API is used.
  await page.route('**/api/auth/me', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(adminSession),
  }))
})

test('protected system warnings route mounts the native warnings view', async ({ page }) => {
  await page.route('**/api/system-ops/warnings', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      items: [{ id: 11, source: 'server', level: 'warning', message: 'E2E warning sample', time: '2026-10-30T00:00:00Z' }],
      failedFileCleanupEvents: 0,
      checkedAt: '2026-10-30T00:00:00Z',
    }),
  }))

  await page.goto('/account/system-ops/warnings')

  await expect(page.getByRole('heading', { name: '系统警告' })).toBeVisible()
  await expect(page.getByText('E2E warning sample')).toBeVisible()
  await expect(page.locator('.ops-summary-row article').first().locator('strong')).toHaveText('1')
})

test('protected policy route mounts native execution and permission views and switches between them', async ({ page }) => {
  await page.route('**/api/system-ops/policy', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      mode: 'stable',
      updatedAt: '2026-10-30T00:00:00Z',
      changedBy: 'E2E fixture',
      enforcement: 'executor_connected',
      executorStatus: 'online',
    }),
  }))
  await page.route('**/api/system-ops/tasks', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) }))
  await page.route('**/api/admin/users', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) }))
  await page.route('**/api/admin/roles', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) }))
  await page.route('**/api/admin/permissions', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) }))

  await page.goto('/account/system-ops/policy')
  await expect(page.getByRole('heading', { name: '运行模式与任务策略' })).toBeVisible()

  await page.getByRole('navigation', { name: '运维管理功能' }).getByRole('button', { name: '权限管理' }).click()
  await expect(page.getByRole('heading', { name: '权限管理' })).toBeVisible()

  await page.getByRole('navigation', { name: '运维管理功能' }).getByRole('button', { name: '运行模式与任务策略' }).click()
  await expect(page.getByRole('heading', { name: '运行模式与任务策略' })).toBeVisible()
})
