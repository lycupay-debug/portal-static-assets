/** Chromium coverage for the native DOM hierarchical knowledge-tag picker. */
import { expect, test } from '@playwright/test'

const adminSession = { user: { id: 'tags-e2e-admin', email: 'tags-e2e@example.test', displayName: 'Tags E2E', isAdmin: true } }

test('content settings open, commit and remove taxonomy tags through native DOM', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(adminSession) }))
  await page.goto('/content-management/new', { waitUntil: 'domcontentloaded', timeout: 10000 })
  await expect(page.locator('.workspace-save-draft')).toBeVisible({ timeout: 10000 })

  await page.getByRole('button', { name: /点击选择标签/ }).click({ timeout: 5000 })
  const dialog = page.getByRole('dialog', { name: '选择知识广场标签' })
  await expect(dialog).toBeVisible({ timeout: 5000 })
  await dialog.locator('label.knowledge-tag-tree-option').filter({ hasText: '小学教育' }).click({ timeout: 5000 })
  await expect(dialog.locator('.knowledge-tag-dialog-footer strong')).toHaveText('1')
  await dialog.getByRole('button', { name: '完成' }).click({ timeout: 5000 })
  await expect(page.getByRole('button', { name: '移除标签 小学教育' })).toBeVisible()
  await page.getByRole('button', { name: '移除标签 小学教育' }).click({ timeout: 5000 })
  await expect(page.getByText('尚未选择标签')).toBeVisible()
})
