/** Framework-neutral regression tests for knowledge filtering, URL state and incremental loading. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { createKnowledgeDirectoryController } from '../src/state/knowledgeDirectory'
import type { MutableValue } from '../src/state/valueContracts'

function cell<T>(value: T): MutableValue<T> { return { value } }
const items = Array.from({ length: 14 }, (_, index) => ({
  id: `id-${index}`, slug: `article-${index}`, title: index % 2 ? `教育实践 ${index}` : `服务器维护 ${index}`,
  summary: index % 2 ? '教育教学与课程' : 'FreeBSD 系统运维', tags: [], publishedAt: new Date(2026, 0, index + 1).toISOString(),
}))

test('knowledge category selection filters articles and synchronizes URL query state', () => {
  const dom = new JSDOM('<div class="knowledge-infinite-list"></div>', { url: 'https://portal.example.test/knowledge?x=1' })
  const controller = createKnowledgeDirectoryController(cell('/knowledge'), cell(items), cell([]), () => undefined, {
    createValue: cell, browser: dom.window, document: dom.window.document,
  })
  controller.selectKnowledgeCategory('education')
  assert.equal(controller.knowledgeCategory.value, 'education')
  assert.equal(controller.selectedKnowledgeCategoryLabel.value, '教育')
  assert.match(dom.window.location.search, /category=education/)
  assert.equal(controller.knowledgeFilteredItems.value.length, 7)
  controller.selectKnowledgeCategory('education', '小学教育')
  assert.match(dom.window.location.search, /subcategory=%E5%B0%8F%E5%AD%A6%E6%95%99%E8%82%B2/)
  dom.window.close()
})

test('knowledge autocomplete navigates to articles and resets search state after category choice', () => {
  const dom = new JSDOM('<div></div>', { url: 'https://portal.example.test/knowledge' })
  const navigations: string[] = []
  const controller = createKnowledgeDirectoryController(cell('/knowledge'), cell(items), cell([]), path => navigations.push(path), {
    createValue: cell, browser: dom.window, document: dom.window.document,
  })
  controller.knowledgeSearchQuery.value = '教育实践 1'
  const article = controller.knowledgeSearchSuggestions.value.find(item => item.kind === 'article' && item.slug === 'article-1')
  assert.ok(article)
  controller.chooseKnowledgeSuggestion(article!)
  assert.deepEqual(navigations, ['/content/article-1'])
  controller.knowledgeSearchQuery.value = '网络'
  controller.chooseKnowledgeSuggestion({ kind: 'theme', label: '网络' })
  assert.equal(controller.knowledgeSearchQuery.value, '')
  assert.equal(controller.knowledgeCategory.value, 'network')
  dom.window.close()
})

test('knowledge list paging increases the visible window only near the list end', () => {
  const dom = new JSDOM('<div class="knowledge-infinite-list"></div>', { url: 'https://portal.example.test/knowledge' })
  const list = dom.window.document.querySelector<HTMLElement>('.knowledge-infinite-list')!
  list.getBoundingClientRect = () => ({ bottom: 100, top: 0, left: 0, right: 100, width: 100, height: 100, x: 0, y: 0, toJSON: () => ({}) })
  const controller = createKnowledgeDirectoryController(cell('/knowledge'), cell(items), cell([]), () => undefined, {
    createValue: cell, browser: dom.window, document: dom.window.document,
  })
  assert.equal(controller.knowledgeVisibleCount.value, 10)
  controller.onKnowledgePageScroll()
  assert.equal(controller.knowledgeVisibleCount.value, 14)
  controller.loadMoreKnowledge()
  assert.equal(controller.knowledgeVisibleCount.value, 14)
  dom.window.close()
})
