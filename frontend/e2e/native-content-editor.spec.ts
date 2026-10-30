/**
 * Chromium coverage for the native Tiptap workspace mounted from App.vue.
 * Dependencies: Playwright Chromium and synthetic same-origin API fixtures.
 * Collaboration: verifies the Vue-to-DOM bridge while keeping document persistence and uploads isolated.
 * Boundaries: no production account, real API mutation, or server-side content is used.
 */
import { expect, test } from '@playwright/test'

const adminSession = { user: { id: 'editor-e2e-admin', email: 'editor-e2e@example.test', displayName: 'Editor E2E', isAdmin: true } }

test('native editor mounts workspace, edits Markdown, filters slash tools and inserts an image URL', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(adminSession) }))

  await page.goto('/content-management/new')
  const editor = page.locator('.ProseMirror[aria-label="文档正文编辑器"]')
  await expect(editor).toBeVisible()
  await expect(page.getByRole('complementary').filter({ has: page.getByRole('navigation', { name: '文档目录' }) })).toBeVisible()
  await editor.click()
  await editor.pressSequentially('Native editor content')
  await expect(editor).toContainText('Native editor content')

  await editor.press('Enter')
  await editor.pressSequentially('/')
  const slashMenu = page.getByRole('menu', { name: '插入内容块' })
  await expect(slashMenu).toBeVisible()
  await slashMenu.getByRole('searchbox', { name: '搜索插入工具' }).fill('一级标题')
  await slashMenu.getByRole('menuitem', { name: /一级标题/ }).click()
  await expect(editor.locator('h1')).toBeVisible()

  await editor.press('Enter')
  await editor.pressSequentially('/')
  await slashMenu.getByRole('searchbox', { name: '搜索插入工具' }).fill('图片')
  await slashMenu.getByRole('menuitem', { name: /图片/ }).click()
  const imageDialog = page.getByRole('dialog', { name: '插入图片' })
  await expect(imageDialog).toBeVisible()
  await imageDialog.locator('input[name="imageUrl"]').fill('https://assets.example.test/demo.png')
  await imageDialog.getByRole('button', { name: '插入 URL' }).click()
  await expect(editor.locator('img[src="https://assets.example.test/demo.png"]')).toBeVisible()

  await expect(page.getByText('创建内容')).toBeVisible()
  await expect(page.getByRole('button', { name: '保存草稿' })).toBeVisible()
})
