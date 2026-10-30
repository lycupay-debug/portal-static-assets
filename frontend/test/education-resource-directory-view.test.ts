/** Native education-resource directory tests for filters, detail metadata, safe links and disposal. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountEducationResourceDirectoryView, type EducationResourceDirectoryState } from '../src/views/educationResourceDirectoryView'

type Resource = NonNullable<EducationResourceDirectoryState['selected']>
function createState(): EducationResourceDirectoryState {
  const resource: Resource = {
    id: 'resource-1', title: '英语听力练习', description: 'MP3 听力材料', type: '听力材料', grade: '高一',
    filename: 'lesson.mp3', fileSize: 2048, url: 'https://resources.example.test/lesson',
    downloadUrl: '/api/public/resources/resource-1/download', createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
  }
  return { category: '全部资源', grade: '全部年级', search: '', types: ['全部资源', '听力材料', '课件'], grades: ['全部年级', '高一', '高二'], resources: [resource], visibleResources: [resource], loading: false, error: '', selected: null }
}

test('education directory renders filters, safely displays resource text and delegates filter changes', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/education-resources' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  let state = createState()
  const calls: string[] = []
  const view = mountEducationResourceDirectoryView(host, {
    getState: () => state,
    setCategory: value => calls.push(`category:${value}`),
    setGrade: value => calls.push(`grade:${value}`),
    setSearch: value => { state = { ...state, search: value }; calls.push(`search:${value}`) },
    resetVisible: () => calls.push('reset-visible'),
    selectResource: value => { state = { ...state, selected: value }; calls.push(`select:${value?.id || 'none'}`) },
    ensureListFilled: () => calls.push('ensure-list'),
  })
  assert.equal(host.querySelector('.education-resource-heading h1')?.textContent, '教育资源.')
  assert.equal(host.querySelectorAll('.education-resource-list-item').length, 1)
  host.querySelector<HTMLButtonElement>('.knowledge-category-button:nth-child(2)')!.click()
  host.querySelector<HTMLButtonElement>('.knowledge-subcategory-button:nth-child(2)')!.click()
  const search = host.querySelector<HTMLInputElement>('input[aria-label="模糊搜索教育资源"]')!
  search.value = '英语'
  search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.ok(calls.includes('category:听力材料'))
  assert.ok(calls.includes('grade:高一'))
  assert.ok(calls.includes('search:英语'))
  assert.equal(host.querySelector('.education-resource-list-item strong')?.textContent, '英语听力练习')
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('resource detail dialog renders safe links, MP3 preview and closes with Escape', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/education-resources' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const state = createState()
  state.selected = state.resources[0]!
  const view = mountEducationResourceDirectoryView(host, {
    getState: () => state,
    setCategory: () => undefined,
    setGrade: () => undefined,
    setSearch: () => undefined,
    resetVisible: () => undefined,
    selectResource: () => { state.selected = null; view.update(state) },
    ensureListFilled: () => undefined,
  })
  const dialog = host.querySelector('[role="dialog"]')!
  assert.equal(dialog.querySelector('h2')?.textContent, '英语听力练习')
  assert.equal(dialog.querySelector('audio')?.src, 'https://portal.example.test/api/public/resources/resource-1/preview')
  assert.equal(dialog.querySelector('a.education-resource-detail-url')?.getAttribute('href'), 'https://resources.example.test/lesson')
  assert.equal(dialog.querySelector('a.education-resource-detail-download')?.getAttribute('href'), 'https://portal.example.test/api/public/resources/resource-1/download')
  dialog.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(host.querySelector('[role="dialog"]'), null)
  view.dispose()
  dom.window.close()
})

test('unsafe resource URL schemes are not rendered as actionable links', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/education-resources' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const state = createState()
  state.selected = { ...state.resources[0]!, url: 'javascript:alert(1)', downloadUrl: 'data:text/html,unsafe' }
  const view = mountEducationResourceDirectoryView(host, {
    getState: () => state,
    setCategory: () => undefined,
    setGrade: () => undefined,
    setSearch: () => undefined,
    resetVisible: () => undefined,
    selectResource: () => undefined,
    ensureListFilled: () => undefined,
  })
  const dialog = host.querySelector('[role="dialog"]')!
  assert.equal(dialog.querySelectorAll('a').length, 0)
  assert.equal(dialog.querySelector('.education-resource-detail-warning')?.textContent, '该资源当前没有可访问的文件或链接，请联系资源发布者。')
  view.dispose()
  dom.window.close()
})
