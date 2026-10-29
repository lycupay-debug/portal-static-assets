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
