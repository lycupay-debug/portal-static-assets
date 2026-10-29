/**
 * Regression tests for the native teaching-resource detail drawer.
 * Dependencies: Node's built-in test/assert modules and jsdom.
 * Collaboration: protects the DOM contract consumed by App.vue while the Vue component is removed.
 * Boundaries: no network calls, live resources, or service dependencies.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import type { TeachingResource } from '../src/api/generated'
import { mountTeachingResourceDrawer } from '../src/components/teachingResourceDrawer'

function createResource(overrides: Partial<TeachingResource> = {}): TeachingResource {
  return {
    id: 'resource-123',
    title: '测试资源',
    description: '一段说明',
    type: '课件',
    grade: '高一',
    isPublic: true,
    url: 'https://example.com/resource.pdf',
    filename: 'resource.pdf',
    mimeType: 'application/pdf',
    fileSize: 1024,
    createdAt: '2026-10-29T10:00:00Z',
    updatedAt: 'not-a-date',
    ...overrides,
  }
}

function createDom() {
  return new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', {
    url: 'https://portal.example.test/',
  })
}

test('drawer renders resource fields as text and uses safe external-link attributes', () => {
  const dom = createDom()
  const document = dom.window.document
  const host = document.querySelector<HTMLElement>('#host')!
  const dispose = mountTeachingResourceDrawer(host, createResource({ title: '<img src=x onerror=alert(1)>' }), 'active', {
    onClose() {}, onEdit() {}, onDownload() {},
  })

  assert.equal(host.querySelector('[role="dialog"]')?.getAttribute('aria-modal'), 'true')
  assert.equal(host.querySelector('h3')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(host.querySelector('img'), null)
  const link = host.querySelector<HTMLAnchorElement>('.resource-detail-fields a')
  assert.equal(link?.href, 'https://example.com/resource.pdf')
  assert.equal(link?.target, '_blank')
  assert.equal(link?.rel, 'noopener noreferrer')
  assert.equal(host.textContent?.includes('not-a-date'), true)
  assert.equal(host.textContent?.includes('文件名称'), true)

  dispose()
  dom.window.close()
})

test('active drawer forwards edit and download actions with the resource id', () => {
  const dom = createDom()
  const document = dom.window.document
  const host = document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const dispose = mountTeachingResourceDrawer(host, createResource(), 'active', {
    onClose() { calls.push('close') },
    onEdit(id) { calls.push('edit:' + id) },
    onDownload(id) { calls.push('download:' + id) },
  })

  host.querySelector<HTMLButtonElement>('.content-secondary-button')!.click()
  host.querySelector<HTMLButtonElement>('.content-primary-button')!.click()
  assert.deepEqual(calls, ['edit:resource-123', 'download:resource-123'])

  dispose()
  dom.window.close()
})

test('trash drawer hides edit and download actions and renders deletion time', () => {
  const dom = createDom()
  const document = dom.window.document
  const host = document.querySelector<HTMLElement>('#host')!
  const dispose = mountTeachingResourceDrawer(host, createResource({ deletedAt: '2026-10-29T11:00:00Z', url: null }), 'trash', {
    onClose() {}, onEdit() {}, onDownload() {},
  })

  assert.equal(host.querySelector('.resource-detail-actions'), null)
  assert.equal(host.textContent?.includes('移入回收站时间'), true)
  assert.equal(host.textContent?.includes('站内文件'), true)

  dispose()
  dom.window.close()
})

test('drawer refuses unsafe URL schemes and keeps the supplied value as text', () => {
  const dom = createDom()
  const document = dom.window.document
  const host = document.querySelector<HTMLElement>('#host')!
  const unsafeUrl = 'javascript:alert(1)'
  const dispose = mountTeachingResourceDrawer(host, createResource({ url: unsafeUrl }), 'active', {
    onClose() {}, onEdit() {}, onDownload() {},
  })

  assert.equal(host.querySelector('.resource-detail-fields a'), null)
  assert.equal(host.textContent?.includes(unsafeUrl), true)

  dispose()
  dom.window.close()
})

test('backdrop, close button and Escape forward close intent; dispose removes listeners and DOM', () => {
  const dom = createDom()
  const document = dom.window.document
  const host = document.querySelector<HTMLElement>('#host')!
  let closes = 0
  const dispose = mountTeachingResourceDrawer(host, createResource(), 'active', {
    onClose() { closes += 1 }, onEdit() {}, onDownload() {},
  })
  const backdrop = host.querySelector<HTMLElement>('.resource-detail-drawer-backdrop')!
  backdrop.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }))
  host.querySelector<HTMLButtonElement>('[aria-label="关闭详情"]')!.click()
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(closes, 3)

  dispose()
  dispose()
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(closes, 3)
  assert.equal(host.children.length, 0)
  dom.window.close()
})
