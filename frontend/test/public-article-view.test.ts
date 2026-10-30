/** Native public-article view tests; API metadata is rendered as text and Markdown HTML is pre-sanitized. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountPublicArticleView } from '../src/views/publicArticleView'

const labels = { article: '知识文章', tool: '在线工具', resource: '资源资料', project: '项目展示' }

test('public article view renders loading, unavailable and article states', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/content/example' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const view = mountPublicArticleView(host, { contentTypeLabels: labels, toggleInteraction: () => undefined, shareArticle: () => undefined, copyShareUrl: () => undefined, closeShare: () => undefined, navigate: () => undefined })
  view.update({ article: null, html: '', loading: true, error: '', busy: false, loggedIn: false, shareOpen: false, shareUrl: '' })
  assert.equal(host.textContent?.includes('正在读取内容'), true)
  view.update({ article: null, html: '', loading: false, error: '未公开', busy: false, loggedIn: false, shareOpen: false, shareUrl: '' })
  assert.equal(host.querySelector('h1')?.textContent, '内容暂不可用')
  assert.equal(host.textContent?.includes('未公开'), true)
  view.update({ article: { title: '知识文章标题', slug: 'example', contentType: 'article', tags: ['教育', '教学'], summary: '文章摘要', authorName: '作者甲', publishedAt: '2026-10-01T00:00:00Z', likeCount: 3, favoriteCount: 4, viewerLiked: true, viewerFavorited: false }, html: '<h2>正文</h2>', loading: false, error: '', busy: false, loggedIn: false, shareOpen: false, shareUrl: '' })
  assert.equal(host.querySelector('.public-article h1')?.textContent, '知识文章标题')
  assert.equal(host.querySelector('.public-article-route-type')?.textContent, '教育 · 教学')
  assert.equal(host.querySelector('.public-article-body h2')?.textContent, '正文')
  assert.equal(host.querySelector('.public-author-avatar')?.textContent, '作')
  assert.equal(host.querySelector('.public-article-login-hint')?.hidden, false)
  assert.equal(host.querySelector('.public-article-action[aria-pressed="true"]')?.textContent?.includes('3'), true)
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('article metadata remains text, interaction actions are explicit, and share dialog closes/copies', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/content/example' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountPublicArticleView(host, {
    contentTypeLabels: labels,
    toggleInteraction: type => calls.push(type),
    shareArticle: () => calls.push('share'),
    copyShareUrl: () => calls.push('copy'),
    closeShare: () => calls.push('close-share'),
    navigate: target => calls.push(`navigate:${target}`),
  })
  view.update({ article: { title: '<img src=x onerror=alert(1)>', slug: 'example', contentType: 'tool', authorName: 'author' }, html: '<p>sanitized</p>', loading: false, error: '', busy: false, loggedIn: true, shareOpen: true, shareUrl: 'https://portal.example.test/content/example' })
  assert.equal(host.querySelector('.public-article h1')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(host.querySelector('.public-article h1 img'), null)
  assert.equal(host.querySelector('.public-article-login-hint')?.hidden, true)
  assert.equal(host.querySelector<HTMLInputElement>('.share-dialog-url')?.value, 'https://portal.example.test/content/example')
  const buttons = host.querySelectorAll<HTMLButtonElement>('.public-article-actions button')
  buttons[0]?.click(); buttons[1]?.click(); buttons[2]?.click()
  host.querySelector<HTMLButtonElement>('.share-dialog-copy')!.click()
  host.querySelector<HTMLButtonElement>('.share-dialog-close')!.click()
  assert.deepEqual(calls, ['like', 'favorite', 'share', 'copy', 'close-share'])
  view.update({ article: { title: 'Busy', slug: 'busy', contentType: 'article' }, html: '', loading: false, error: '', busy: true, loggedIn: true, shareOpen: false, shareUrl: '' })
  assert.equal(host.querySelector<HTMLButtonElement>('.public-article-actions button')?.disabled, true)
  view.dispose()
  dom.window.close()
})
