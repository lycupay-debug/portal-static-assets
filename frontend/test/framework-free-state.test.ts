/** Framework-neutral state controllers tested with plain cells and browser adapters. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { createDebouncedValue } from '../src/state/debouncedValue'
import { createLegalDocumentsController } from '../src/state/legalDocuments'
import { createScrollRestoration } from '../src/state/scrollRestoration'
import type { MutableValue } from '../src/state/valueContracts'

function cell<T>(value: T): MutableValue<T> { return { value } }

test('debounced value mirrors the last source change and disposes its subscription', async () => {
  const source = cell('first')
  const listeners = new Set<(value: string) => void>()
  const state = createDebouncedValue(source, 5, {
    createValue: cell,
    subscribe: (_source, listener) => { listeners.add(listener); return () => listeners.delete(listener) },
  })
  assert.equal(state.value.value, 'first')
  source.value = 'second'
  for (const listener of listeners) listener('second')
  source.value = 'third'
  for (const listener of listeners) listener('third')
  await new Promise(resolve => setTimeout(resolve, 15))
  assert.equal(state.value.value, 'third')
  state.dispose()
  assert.equal(listeners.size, 0)
})

test('legal-document controller loads published links and route-matched Markdown', async () => {
  const route = cell('/legal/privacy')
  const calls: string[] = []
  const controller = createLegalDocumentsController(route, {
    createValue: cell,
    request: async <T>(path: string) => {
      calls.push(path)
      if (path === '/public/legal') return { items: [{ slug: 'privacy', title: 'Privacy', documentType: 'privacy' }] } as T
      return { item: { slug: 'privacy', title: 'Privacy', bodyMarkdown: '# Privacy' } } as T
    },
    renderMarkdown: markdown => `<h1>${markdown.slice(2)}</h1>`,
  })
  assert.equal(controller.legalSlug.value, 'privacy')
  await controller.loadPublishedLegalDocuments()
  await controller.loadLegalDocument('privacy')
  assert.equal(controller.publishedLegalDocuments.value.length, 1)
  assert.equal(controller.legalDocument.value?.title, 'Privacy')
  assert.equal(controller.legalDocumentHtml.value, '<h1>Privacy</h1>')
  assert.deepEqual(calls, ['/public/legal', '/public/legal/privacy'])
})

test('scroll restoration persists and restores window and nested workspace positions', async () => {
  const dom = new JSDOM('<main><div class="account-content"></div><aside class="account-sidebar"></aside><div class="virtual-table-scroll"></div></main>', { url: 'https://portal.example.test/account' })
  const route = cell('/account')
  const scrollCalls: number[] = []
  Object.defineProperty(dom.window, 'scrollY', { configurable: true, value: 120 })
  Object.defineProperty(dom.window, 'scrollTo', { configurable: true, value: (options: ScrollToOptions) => scrollCalls.push(Number(options.top)) })
  const content = dom.window.document.querySelector<HTMLElement>('.account-content')!
  const sidebar = dom.window.document.querySelector<HTMLElement>('.account-sidebar')!
  const table = dom.window.document.querySelector<HTMLElement>('.virtual-table-scroll')!
  content.scrollTop = 20; sidebar.scrollTop = 30; table.scrollTop = 40
  const controller = createScrollRestoration(route, { browser: dom.window, document: dom.window.document, storage: dom.window.sessionStorage, afterRender: async () => undefined })
  controller.saveScrollPosition()
  content.scrollTop = 0; sidebar.scrollTop = 0; table.scrollTop = 0
  await controller.restoreScrollPosition()
  assert.deepEqual(scrollCalls, [120])
  assert.equal(content.scrollTop, 20)
  assert.equal(sidebar.scrollTop, 30)
  assert.equal(table.scrollTop, 40)
  dom.window.close()
})
