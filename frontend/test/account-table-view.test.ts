/** Native account-table tests for row actions, virtual scrolling, cell details and version navigation. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountAccountTableView, type AccountTableViewState } from '../src/views/accountTableView'
import type { AccountTableDefinition } from '../src/config/accountTables'

const contentDefinition: AccountTableDefinition = { title: '内容列表', columns: [{ key: 'title', label: '内容标题' }, { key: 'status', label: '状态' }, { key: 'actions', label: '操作' }], note: '内容数据源尚未接入。' }
const resourceDefinition: AccountTableDefinition = { title: '教学资源', columns: [{ key: 'title', label: '资源名称' }, { key: 'actions', label: '操作' }], note: '教学资源已接入数据库。' }
function baseState(): AccountTableViewState {
  const record = { id: 'content-1', title: 'Native content', status: '已发布', actions: '已发布' }
  return {
    section: 'content-management', itemLabel: '内容管理', itemDescription: '管理内容', definition: contentDefinition,
    versionsContentId: null, versionsContentTitle: '', versions: [], contentBusy: false, contentLoading: false, resourceBusy: false,
    records: [record], visibleRecords: [record], columnTemplate: '2fr 1fr 2fr', topSpace: 0, bottomSpace: 0, scrollTop: 0,
    query: '', contentViewMode: 'active', resourceViewMode: 'active', selectedCell: null,
  }
}
function createOptions(state: () => AccountTableViewState, calls: string[], setScrollElement?: (element: HTMLElement | null) => void) {
  return {
    getState: state,
    setQuery: (value: string) => calls.push(`query:${value}`),
    setScrollTop: (value: number) => calls.push(`scroll:${value}`),
    setScrollElement: (element: HTMLElement | null) => setScrollElement?.(element),
    createContent: () => calls.push('create-content'),
    switchContentView: mode => calls.push(`content-view:${mode}`),
    createResource: () => calls.push('create-resource'),
    switchResourceView: () => calls.push('resource-view'),
    resourceHasUrl: () => true,
    openResource: id => calls.push(`open-resource:${id}`),
    editResource: id => calls.push(`edit-resource:${id}`),
    downloadResource: id => calls.push(`download-resource:${id}`),
    changeResource: (id, action) => calls.push(`change-resource:${id}:${action}`),
    editContent: id => calls.push(`edit-content:${id}`),
    openVersions: id => calls.push(`versions:${id}`),
    changeContentStatus: (id, action) => calls.push(`status:${id}:${action}`),
    deleteContent: id => calls.push(`delete-content:${id}`),
    restoreContent: id => calls.push(`restore-content:${id}`),
    permanentlyDeleteContent: id => calls.push(`permanent-content:${id}`),
    restoreVersion: version => calls.push(`restore-version:${version.versionId}`),
    backToContentList: () => calls.push('back-to-content'),
    openCell: (label: string, value: string) => calls.push(`cell:${label}:${value}`),
    closeCell: () => calls.push('close-cell'),
    cellValue: (record: Record<string, string | number>, key: string) => String(record[key] ?? ''),
    cellPreview: (value: string) => value.slice(0, 12),
    contentStatusLabel: (status: string) => status,
  }
}

test('content table forwards search, edit, version, publish and delete actions', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/account/content-management' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const state = baseState()
  const view = mountAccountTableView(host, createOptions(() => state, calls))
  assert.equal(host.querySelector('.virtual-table-heading h2')?.textContent, '内容列表')
  assert.equal(host.querySelectorAll('.virtual-table-row').length, 1)
  host.querySelector<HTMLInputElement>('input[name="tableSearch"]')!.value = 'native'
  host.querySelector<HTMLInputElement>('input[name="tableSearch"]')!.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  host.querySelector<HTMLButtonElement>('.table-cell-preview')!.click()
  for (const label of ['编辑', '版本', '下架', '删除', '新建内容']) host.querySelectorAll<HTMLButtonElement>('button').forEach(button => { if (button.textContent === label) button.click() })
  assert.ok(calls.includes('query:native'))
  assert.ok(calls.includes('cell:内容标题:Native content'))
  assert.ok(calls.includes('edit-content:content-1'))
  assert.ok(calls.includes('versions:content-1'))
  assert.ok(calls.includes('status:content-1:unpublish'))
  assert.ok(calls.includes('delete-content:content-1'))
  assert.ok(calls.includes('create-content'))
  view.update({ ...state, selectedCell: { label: '摘要', value: '完整内容' } })
  assert.equal(host.querySelector('[role="dialog"] p')?.textContent, '完整内容')
  host.querySelector<HTMLButtonElement>('[aria-label="关闭"]')!.click()
  assert.ok(calls.includes('close-cell'))
  view.dispose(); dom.window.close()
})

test('resource table delegates row details and active/trash actions without bubbling action clicks', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/account/teaching-resources' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const record = { id: 'resource-1', title: 'Lesson file', actions: '可用' }
  let state = { ...baseState(), section: 'teaching-resources', definition: resourceDefinition, records: [record], visibleRecords: [record] }
  const view = mountAccountTableView(host, createOptions(() => state, calls))
  host.querySelector<HTMLElement>('.virtual-table-row')!.click()
  for (const label of ['编辑', '打开链接', '删除', '回收站']) host.querySelectorAll<HTMLButtonElement>('button').forEach(button => { if (button.textContent === label) button.click() })
  assert.ok(calls.includes('open-resource:resource-1'))
  assert.ok(calls.includes('edit-resource:resource-1'))
  assert.ok(calls.includes('download-resource:resource-1'))
  assert.ok(calls.includes('change-resource:resource-1:trash'))
  assert.ok(calls.includes('resource-view'))
  state = { ...state, resourceViewMode: 'trash' }
  view.update(state)
  for (const label of ['恢复', '永久删除']) host.querySelectorAll<HTMLButtonElement>('button').forEach(button => { if (button.textContent === label) button.click() })
  assert.ok(calls.includes('change-resource:resource-1:restore'))
  assert.ok(calls.includes('change-resource:resource-1:permanent'))
  view.dispose(); dom.window.close()
})

test('version history renders empty state and delegates return to content list', () => {
  const dom = new JSDOM('<div id="host"></div>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const state = { ...baseState(), versionsContentId: 'content-1', versionsContentTitle: 'Native content' }
  const view = mountAccountTableView(host, createOptions(() => state, calls))
  assert.equal(host.querySelector('.content-versions-panel h2')?.textContent, '版本历史')
  assert.equal(host.querySelector('.virtual-table-empty strong')?.textContent, '暂无历史版本')
  host.querySelector<HTMLButtonElement>('.content-editor-heading button')!.click()
  assert.ok(calls.includes('back-to-content'))
  view.dispose(); dom.window.close()
})

test('virtualized table preserves scroll position and search focus across snapshots', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/account/content-management' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const records = Array.from({ length: 100 }, (_, index) => ({ id: `content-${index}`, title: `Content ${index}`, status: '草稿', actions: '待发布' }))
  let state: AccountTableViewState = { ...baseState(), records, visibleRecords: records.slice(20, 30), topSpace: 880, bottomSpace: 3080, scrollTop: 900 }
  const scrolls: Array<HTMLElement | null> = []
  const view = mountAccountTableView(host, createOptions(() => state, calls, element => scrolls.push(element)))
  const input = host.querySelector<HTMLInputElement>('input[name="tableSearch"]')!
  input.value = 'Native content'
  input.focus(); input.setSelectionRange(2, 4)
  const scroll = host.querySelector<HTMLElement>('.virtual-table-scroll')!
  assert.equal(scroll.scrollTop, 900)
  state = { ...state, query: 'Content', selectedCell: { label: 'title', value: 'Content' } }
  view.update(state)
  const nextInput = host.querySelector<HTMLInputElement>('input[name="tableSearch"]')!
  const nextScroll = host.querySelector<HTMLElement>('.virtual-table-scroll')!
  assert.equal(dom.window.document.activeElement, nextInput)
  assert.equal(nextInput.selectionStart, 2)
  assert.equal(nextInput.selectionEnd, 4)
  assert.equal(nextScroll.scrollTop, 900)
  view.dispose()
  assert.equal(scrolls.at(-1), null)
  dom.window.close()
})
