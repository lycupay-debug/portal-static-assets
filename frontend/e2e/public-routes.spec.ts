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
  await expect(page.locator('.hero h1')).toContainText('把想法整理好，')
  await expect(page.locator('.hero h1')).toContainText('让行动有迹可循。')
  await expect(page.locator('.section-card')).toHaveCount(4)
  const search = page.getByRole('searchbox', { name: '搜索门户分类' })
  await search.fill('工具')
  await expect(page.locator('.section-card')).toHaveCount(1)
  await expect(page.locator('.section-card h3')).toHaveText('在线工具')
  await search.fill('no matching section')
  await expect(page.locator('.empty-search')).toBeVisible()
  await page.getByRole('button', { name: '清除搜索' }).click()
  await expect(page.locator('.section-card')).toHaveCount(4)
  await page.getByRole('link', { name: '开始探索' }).click()
  await expect(page).toHaveURL(/\/#explore$/)
  await expect(page.locator('#explore')).toBeInViewport()
})

test('knowledge directory filters categories and searches published articles in native DOM', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'not signed in' }) }))
  await page.route('**/api/public/content*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [
      { id: 'article-1', title: 'FreeBSD 系统维护', slug: 'freebsd-notes', contentType: 'article', tags: ['网络维护'], parentCategory: '网络', childCategory: '网络维护', summary: 'FreeBSD 网络与服务维护', publishedAt: '2026-10-01T00:00:00Z' },
      { id: 'article-2', title: 'Python 编程实践', slug: 'python-notes', contentType: 'article', tags: ['Python'], parentCategory: '编程', childCategory: 'Python', summary: 'Python 开发经验', publishedAt: '2026-09-01T00:00:00Z' },
    ] }),
  }))
  await page.goto('/knowledge')
  await expect(page.getByRole('heading', { name: '文章列表' })).toBeVisible()
  await expect(page.locator('.knowledge-list-item')).toHaveCount(2)

  const search = page.getByRole('searchbox', { name: '模糊搜索知识文章' })
  await search.fill('FreeBSD')
  await expect(page.locator('.knowledge-list-item')).toHaveCount(1)
  await expect(page.locator('.knowledge-list-item strong')).toHaveText('FreeBSD 系统维护')
  await expect(page.getByRole('option', { name: /FreeBSD 系统维护/ })).toBeVisible()
  await page.getByRole('button', { name: '清除搜索' }).click()
  await expect(page.locator('.knowledge-list-item')).toHaveCount(2)

  await page.getByRole('button', { name: '编程', exact: true }).click()
  await expect(page).toHaveURL(/category=programming/)
  await expect(page.locator('.knowledge-list-item')).toHaveCount(1)
  await expect(page.locator('.knowledge-list-item strong')).toHaveText('Python 编程实践')
  await page.getByRole('button', { name: 'Python', exact: true }).click()
  await expect(page).toHaveURL(/subcategory=Python/)
  await expect(page.locator('.knowledge-list-item strong')).toHaveText('Python 编程实践')
})

test('software tools and homestay directory overviews render native content', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'not signed in' }) }))
  await page.route('**/api/public/content', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ items: [{
      id: 'tool-fixture', title: 'Native tool fixture', slug: 'native-tool-fixture', contentType: 'tool',
      tags: ['效率工具'], summary: 'Tool directory summary', publishedAt: '2026-10-01T00:00:00Z',
    }] }),
  }))
  await page.goto('/software-tools')
  await expect(page.getByRole('heading', { name: '软件工具.' })).toBeVisible()
  await expect(page.locator('.directory-tile')).toHaveCount(4)
  await expect(page.getByRole('heading', { name: 'Native tool fixture' })).toBeVisible()
  await expect(page.getByText('Tool directory summary')).toBeVisible()

  await page.goto('/homestays')
  await expect(page.getByRole('heading', { name: '民宿房源.' })).toBeVisible()
  await expect(page.locator('.directory-tile')).toHaveCount(4)
  await expect(page.getByRole('heading', { name: '房源内容正在整理中' })).toBeVisible()
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
