/**
 * Responsibility: verify TeachingResourceDrawer's rendered data and emitted user intent in jsdom.
 * Dependencies: repository-installed Vue SFC compiler, esbuild, jsdom, and shared resource formatter.
 * Collaboration: App.vue owns the resource record, view mode, and edit/download/close side effects.
 * Limits: DOM/event assertions do not replace browser visual or assistive-technology testing.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from '@vue/compiler-sfc'
import { transform } from 'esbuild'
import { readFileSync, writeFileSync, unlinkSync } from 'node:fs'
import { resolve } from 'node:path'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' })
for (const key of ['window', 'document', 'Node', 'Element', 'HTMLElement', 'SVGElement', 'MouseEvent']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
}

const componentPath = new URL('../src/components/TeachingResourceDrawer.vue', import.meta.url)
const source = readFileSync(componentPath, 'utf8')
const { descriptor, errors } = parse(source, { filename: 'TeachingResourceDrawer.vue' })
if (errors.length) throw errors[0]
const compiled = compileScript(descriptor, { id: 'teaching-resource-drawer-mounted-test', inlineTemplate: true })
const compiledPath = resolve(new URL('../src/components/', import.meta.url).pathname, `.teaching-resource-drawer-test-${process.pid}.mjs`)
const transformed = await transform(compiled.content, { sourcefile: compiledPath, loader: 'ts', format: 'esm' })
writeFileSync(compiledPath, transformed.code)
let TeachingResourceDrawer
try { ({ default: TeachingResourceDrawer } = await import(`file://${compiledPath}?test=${Date.now()}`)) }
finally { unlinkSync(compiledPath) }
const { createApp, defineComponent, h, nextTick } = await import('vue')

const resource = overrides => ({
  id: 'resource-1', title: 'Unit notes', description: 'Lesson summary', type: 'PDF',
  grade: null, isPublic: true, url: null, filename: 'notes.pdf', mimeType: 'application/pdf',
  fileSize: 0, createdAt: '2026-10-29T08:00:00Z', updatedAt: null, deletedAt: null,
  ...overrides,
})
function mountDrawer(item = resource(), mode = 'active') {
  const emitted = []
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(defineComponent({
    setup() {
      return () => h(TeachingResourceDrawer, {
        resource: item,
        resourceViewMode: mode,
        onClose: () => emitted.push(['close']),
        onEdit: id => emitted.push(['edit', id]),
        onDownload: id => emitted.push(['download', id]),
      })
    },
  }))
  app.mount(host)
  return { host, app, emitted, cleanup() { app.unmount(); host.remove() } }
}

test('formats zero-byte files and malformed timestamps without hiding or mislabeling them', () => {
  const view = mountDrawer(resource({ createdAt: 'not-a-date', updatedAt: '2026-10-29T08:00:00Z', fileSize: 0, deletedAt: 'also-not-a-date' }), 'trash')
  try {
    const rows = [...view.host.querySelectorAll('.resource-detail-fields > div')]
    const valueFor = label => rows.find(row => row.querySelector('dt')?.textContent === label)?.querySelector('dd')?.textContent
    assert.equal(valueFor('文件大小'), '0 B')
    assert.equal(valueFor('创建时间'), 'not-a-date')
    assert.notEqual(valueFor('更新时间'), 'Invalid Date')
    assert.equal(valueFor('移入回收站时间'), 'also-not-a-date')
    assert.equal(view.host.querySelectorAll('.resource-detail-actions button').length, 0)
  } finally { view.cleanup() }
})

test('forwards close, edit, and download intents with the resource id in active mode', async () => {
  const view = mountDrawer()
  try {
    view.host.querySelector('[aria-label="关闭详情"]').click()
    view.host.querySelector('.resource-detail-actions button:nth-child(1)').click()
    view.host.querySelector('.resource-detail-actions button:nth-child(2)').click()
    await nextTick()
    assert.deepEqual(view.emitted, [['close'], ['edit', 'resource-1'], ['download', 'resource-1']])
    const link = view.host.querySelector('.resource-detail-fields a')
    assert.equal(link, null)
  } finally { view.cleanup() }
})

test('renders external resource links with safe target attributes and changes download label', () => {
  const view = mountDrawer(resource({ url: 'https://example.com/resource' }))
  try {
    const link = view.host.querySelector('.resource-detail-fields a')
    assert.equal(link.getAttribute('href'), 'https://example.com/resource')
    assert.equal(link.getAttribute('target'), '_blank')
    assert.equal(link.getAttribute('rel'), 'noopener noreferrer')
    assert.equal(view.host.querySelector('.resource-detail-actions button:nth-child(2)').textContent, '打开资源链接')
  } finally { view.cleanup() }
})

test('test harness releases jsdom resources', () => { dom.window.close() })
