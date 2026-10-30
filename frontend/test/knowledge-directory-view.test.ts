/** Native knowledge-directory tests for filtering controls, suggestions, safe article metadata and cleanup. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountKnowledgeDirectoryView, type KnowledgeDirectoryState } from '../src/views/knowledgeDirectoryView'
import { knowledgeCategoryMenus } from '../src/config/knowledgeCategories'

function fixture() {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/knowledge' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let state: KnowledgeDirectoryState = {
    category: 'recent', subcategory: '', query: '', highlightedSuggestion: -1,
    suggestions: [], filteredItems: [
      { id: '1', slug: 'freebsd-notes', title: 'FreeBSD 系统维护', summary: 'FreeBSD 网络与服务维护', publishedAt: '2026-10-01T00:00:00Z' },
      { id: '2', slug: 'typescript-tools', title: 'TypeScript 工具', summary: '开发工具实践', publishedAt: '2026-09-01T00:00:00Z' },
    ], visibleItems: [
      { id: '1', slug: 'freebsd-notes', title: 'FreeBSD 系统维护', summary: 'FreeBSD 网络与服务维护', publishedAt: '2026-10-01T00:00:00Z' },
      { id: '2', slug: 'typescript-tools', title: 'TypeScript 工具', summary: '开发工具实践', publishedAt: '2026-09-01T00:00:00Z' },
    ], loading: false, error: '',
  }
  const view = mountKnowledgeDirectoryView(host, {
    categories: knowledgeCategoryMenus,
    getState: () => state,
    chooseCategory: (category, subcategory = '') => calls.push(`category:${category}:${subcategory}`),
    setQuery: query => { state = { ...state, query } },
    setHighlightedSuggestion: index => { state = { ...state, highlightedSuggestion: index } },
    chooseSuggestion: item => calls.push(`suggestion:${item.kind}:${item.value}`),
    onSearchKeydown: event => { if (event.key === 'ArrowDown') state = { ...state, highlightedSuggestion: 0 } },
    setVisibleCount: () => calls.push('reset-visible'),
    ensureListFilled: () => calls.push('ensure-list'),
    navigate: target => calls.push(`navigate:${target}`),
  })
  return { dom, host, calls, view, setState(value: KnowledgeDirectoryState) { state = value } }
}

test('knowledge directory renders category navigation and delegates category selection', () => {
  const ctx = fixture()
  assert.equal(ctx.host.querySelectorAll('.knowledge-category-button').length, knowledgeCategoryMenus.length)
  ctx.host.querySelector<HTMLButtonElement>('.knowledge-category-button:nth-child(2)')!.click()
  assert.deepEqual(ctx.calls, ['ensure-list', 'category:education:'])
  ctx.view.dispose()
  ctx.dom.window.close()
})

test('search input, suggestions and article links preserve safe text and route delegation', () => {
  const ctx = fixture()
  ctx.setState({
    category: 'recent', subcategory: '', query: 'FreeBSD', highlightedSuggestion: -1,
    suggestions: [{ label: 'FreeBSD 系统维护', kind: 'article', slug: 'freebsd-notes', summary: '<img src=x onerror=alert(1)>' }],
    filteredItems: [{ id: '1', slug: 'freebsd-notes', title: '<img src=x onerror=alert(1)>', summary: 'FreeBSD notes' }],
    visibleItems: [{ id: '1', slug: 'freebsd-notes', title: '<img src=x onerror=alert(1)>', summary: 'FreeBSD notes' }], loading: false, error: '',
  })
  ctx.view.update({
    category: 'recent', subcategory: '', query: 'FreeBSD', highlightedSuggestion: -1,
    suggestions: [{ label: 'FreeBSD 系统维护', kind: 'article', slug: 'freebsd-notes', summary: '<img src=x onerror=alert(1)>' }],
    filteredItems: [{ id: '1', slug: 'freebsd-notes', title: '<img src=x onerror=alert(1)>', summary: 'FreeBSD notes' }],
    visibleItems: [{ id: '1', slug: 'freebsd-notes', title: '<img src=x onerror=alert(1)>', summary: 'FreeBSD notes' }], loading: false, error: '',
  })
  assert.equal(ctx.host.querySelector('.knowledge-list-copy strong')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(ctx.host.querySelector('.knowledge-list-copy img'), null)
  assert.equal(ctx.host.querySelector('.knowledge-autocomplete-option small')?.textContent, '<img src=x onerror=alert(1)>')
  ctx.host.querySelector<HTMLButtonElement>('.knowledge-autocomplete-option')!.click()
  ctx.host.querySelector<HTMLAnchorElement>('.knowledge-list-item')!.click()
  assert.ok(ctx.calls.includes('suggestion:article:FreeBSD 系统维护'))
  assert.ok(ctx.calls.includes('navigate:/content/freebsd-notes'))
  ctx.view.dispose()
  assert.equal(ctx.host.childElementCount, 0)
  ctx.dom.window.close()
})
