/**
 * Real Chromium coverage for the native teaching-resource create dialog.
 * Dependencies: Playwright Chromium and the local Vite preview; API responses are isolated fixtures.
 * Collaboration: verifies App.vue's route/list state mounts the native dialog and preserves its API contract.
 * Boundaries: no real account, credential, database, or production endpoint is used.
 */
import { expect, test } from '@playwright/test'

const adminSession = {
  user: {
    id: 'e2e-admin',
    email: 'e2e-admin@example.test',
    displayName: 'E2E Admin',
    isAdmin: true,
  },
}

test('native teaching-resource dialog creates a URL resource and completes the upload flow', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(adminSession),
  }))
  await page.route('**/api/resources', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [] }) })
      return
    }
    if (route.request().method() === 'POST') {
      const payload = route.request().postDataJSON()
      expect(payload).toMatchObject({
        title: 'Chromium E2E teaching resource',
        type: '听力材料',
        grade: null,
        isPublic: false,
        url: 'https://files.example.test/e2e.pdf',
      })
      expect(payload).not.toHaveProperty('filename')
      expect(payload).not.toHaveProperty('data')
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ item: { id: 'e2e-resource', title: payload.title, type: payload.type, grade: null, isPublic: false, url: payload.url } }),
      })
      return
    }
    await route.continue()
  })

  await page.goto('/education/teaching-resources')
  await expect(page.getByRole('heading', { name: '教学资源' })).toBeVisible()
  await page.getByRole('button', { name: '创建资源' }).click()
  await expect(page.getByRole('dialog', { name: '创建教学资源' })).toBeVisible()

  await page.getByLabel('资源名称').fill('Chromium E2E teaching resource')
  await page.getByLabel('URL 链接（选填）').fill('https://files.example.test/e2e.pdf')
  await page.getByRole('button', { name: '保存资源' }).click()

  await expect(page.getByText('资源已保存。请点击下方“完成”关闭窗口。')).toBeVisible()
  const finish = page.getByRole('button', { name: '完成' })
  await expect(finish).toBeEnabled()
  await finish.click()
  await expect(page.getByRole('dialog', { name: '创建教学资源' })).toHaveCount(0)
})
