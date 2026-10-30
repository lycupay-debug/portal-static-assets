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

test('account sidebar and identity pages render through native DOM', async ({ page }) => {
  await page.goto('/account')
  await expect(page.getByRole('heading', { name: '账户概览' })).toBeVisible()
  await expect(page.locator('.account-summary-name')).toHaveText('E2E Admin')
  await expect(page.locator('.account-summary-email')).toHaveText('e2e-admin@example.test')
  await expect(page.getByRole('button', { name: '退出登录' })).toBeVisible()

  await page.getByRole('button', { name: '个人资料' }).click()
  await expect(page.getByRole('heading', { name: '个人资料' })).toBeVisible()
  await expect(page.getByRole('button', { name: '退出登录' })).toHaveCount(0)

  await page.getByRole('button', { name: '登录与安全' }).click()
  await expect(page.getByRole('heading', { name: '登录与安全' })).toBeVisible()
  await expect(page.getByText('账户已验证登录')).toBeVisible()
  await expect(page.getByRole('button', { name: '退出当前登录' })).toBeEnabled()
})

test('content management table renders native rows, details and trash navigation', async ({ page }) => {
  await page.route('**/api/content**', route => {
    const pathname = new URL(route.request().url()).pathname
    if (pathname.endsWith('/trash')) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) })
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [{
        id: 'content-e2e', title: 'Native DOM table content for E2E', slug: 'native-dom-table-e2e',
        contentType: 'article', status: 'published', visibility: 'public', tags: ['E2E', 'Native DOM'],
        summary: 'Account table native view fixture', body: 'A long body is not rendered in this table cell.',
        updatedAt: '2026-10-30T00:00:00Z', likeCount: 7, favoriteCount: 3,
      }] }),
    })
  })
  await page.goto('/account/content-management')
  await expect(page.getByRole('heading', { name: '内容列表' })).toBeVisible()
  await expect(page.locator('.virtual-table-row')).toHaveCount(1)
  await expect(page.getByRole('button', { name: '下架' })).toBeVisible()

  await page.locator('.table-cell-preview').first().click()
  await expect(page.getByRole('dialog', { name: '内容标题' })).toBeVisible()
  await expect(page.getByRole('dialog').locator('p')).toHaveText('Native DOM table content for E2E')
  await page.getByRole('button', { name: '关闭' }).click()

  await page.getByRole('button', { name: '回收站' }).click()
  await expect(page.getByRole('button', { name: '返回内容列表' })).toBeVisible()
  await expect(page.getByText('回收站为空。已删除内容可以恢复，也可以永久删除。')).toBeVisible()
  await page.getByRole('button', { name: '返回内容列表' }).click()
  await expect(page.locator('.virtual-table-row')).toHaveCount(1)
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
