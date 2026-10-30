import { expect, test } from '@playwright/test'

/**
 * Responsibility: verify real Chromium can load the built SPA and exercise public auth-route UI.
 * Dependencies: Playwright page API and the local Vite preview configured in playwright.config.ts.
 * Collaboration: validates frontend routing/forms only; authentication and authorization remain
 * server-owned and are not simulated here. Protected business workflows require a separate suite.
 * Boundaries: never submits credentials or mutates server data; each test starts from a clean page.
 */
test('direct login URL renders a usable login form', async ({ page }) => {
  await page.goto('/login')

  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '邮箱地址' })).toBeVisible()
  await expect(page.locator('input[name="password"]')).toBeVisible()
  await expect(page.getByRole('button', { name: '登录' })).toBeEnabled()
})

test('login and registration routes navigate in the browser without a full-page reload', async ({ page }) => {
  await page.goto('/login')
  await page.getByRole('link', { name: '立即注册' }).click()

  await expect(page).toHaveURL(/\/register$/)
  await expect(page.getByRole('heading', { name: '创建你的账户' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '昵称' })).toBeVisible()
  await expect(page.locator('input[name="password"]')).toHaveAttribute('minlength', '10')

  await page.getByRole('link', { name: '立即登录' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
})

test('public root route renders the application shell', async ({ page }) => {
  await page.goto('/')

  await expect(page.locator('#app').locator('> *')).toHaveCount(1)
  await expect(page.getByRole('link', { name: /登录/ }).first()).toBeVisible()
})

test('public article route renders native content, interactions and share dialog', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'not signed in' }) }))
  await page.route('**/api/public/content/native-article', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ item: {
      id: 'native-article', title: 'Native article fixture', slug: 'native-article', contentType: 'article',
      tags: ['工程', '测试'], summary: 'Public article summary', body: '# Native body\n\nSanitized Markdown body.',
      publishedAt: '2026-10-01T00:00:00Z', createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
      authorName: 'E2E Author', authorJoinedAt: '2026-01-01T00:00:00Z', likeCount: 2, favoriteCount: 1,
      viewerLiked: false, viewerFavorited: false,
    } }),
  }))
  await page.route('**/api/public/content/native-article/interactions', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ active: true, likeCount: 3, favoriteCount: 1 }),
  }))

  await page.goto('/content/native-article')
  await expect(page.getByRole('heading', { name: 'Native article fixture' })).toBeVisible()
  await expect(page.getByText('Sanitized Markdown body.')).toBeVisible()
  await expect(page.getByText('E2E Author')).toBeVisible()
  await page.getByRole('button', { name: /点赞/ }).click()
  await expect(page.getByRole('button', { name: /已点赞/ })).toBeVisible()
  await page.getByRole('button', { name: '转发 / 分享' }).click()
  await expect(page.getByRole('dialog', { name: '分享这篇文章' })).toBeVisible()
  await expect(page.locator('.share-dialog-url')).toHaveValue(/\/content\/native-article$/)
  await page.getByRole('button', { name: '取消' }).click()
  await expect(page.getByRole('dialog', { name: '分享这篇文章' })).toHaveCount(0)
})
