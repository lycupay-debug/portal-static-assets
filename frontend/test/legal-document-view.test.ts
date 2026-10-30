/** Native legal-document view tests; the HTML input is explicitly treated as pre-sanitized renderer output. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountLegalDocumentView } from '../src/views/legalDocumentView'

test('legal view renders loading, unavailable and published document states', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/legal/privacy' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const view = mountLegalDocumentView(host, () => undefined)
  view.update({ document: null, html: '', loading: true, error: '' })
  assert.equal(host.textContent?.includes('正在读取政策文件'), true)
  view.update({ document: null, html: '', loading: false, error: '未发布' })
  assert.equal(host.querySelector('h1')?.textContent, '政策文件暂不可用')
  assert.equal(host.textContent?.includes('未发布'), true)
  view.update({ document: { title: '隐私政策', effectiveAt: '2026-10-01T00:00:00Z' }, html: '<h2>数据处理</h2><p>说明</p>', loading: false, error: '' })
  assert.equal(host.querySelector('h1')?.textContent, '隐私政策')
  assert.equal(host.querySelector('.legal-document-effective')?.textContent?.startsWith('生效日期：'), true)
  assert.equal(host.querySelector('.legal-markdown h2')?.textContent, '数据处理')
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('legal view navigates home and does not create dynamic HTML from metadata', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/legal/privacy' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountLegalDocumentView(host, target => calls.push(target))
  view.update({ document: { title: '<img src=x onerror=alert(1)>' }, html: '<p>sanitized</p>', loading: false, error: '' })
  assert.equal(host.querySelector('h1')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(host.querySelector('h1 img'), null)
  host.querySelector<HTMLAnchorElement>('.legal-back-link')!.click()
  assert.deepEqual(calls, ['/'])
  view.dispose()
  dom.window.close()
})
