/** Native static directory tests for route content, published cards, safe text and cleanup. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountPublicDirectoryOverview } from '../src/views/publicDirectoryOverviewView'
import { directoryPages, directoryTiles } from '../src/config/publicDirectories'

test('homestay overview renders its configured categories and coming-soon content', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/homestays' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const view = mountPublicDirectoryOverview(host, { page: directoryPages['/homestays']!, tiles: directoryTiles['/homestays']!, contentTypeLabel: '', navigate: () => undefined })
  assert.equal(host.querySelector('main.directory-page h1')?.textContent, '民宿房源.')
  assert.equal(host.querySelectorAll('.directory-tile').length, 4)
  assert.equal(host.querySelector('.directory-coming-soon h2')?.textContent, '房源内容正在整理中')
  view.update({ items: [], loading: false, error: '' })
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('software tools overview updates published cards and delegates safe route navigation', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/software-tools' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountPublicDirectoryOverview(host, { page: directoryPages['/software-tools']!, tiles: directoryTiles['/software-tools']!, contentTypeLabel: '在线工具', navigate: target => calls.push(target) })
  view.update({ items: [{ id: '1', title: '<img src=x onerror=alert(1)>', slug: 'safe-tool', contentType: 'tool', tags: ['测试'], summary: '工具摘要', publishedAt: '2026-10-01T00:00:00Z' }], loading: false, error: '' })
  assert.equal(host.querySelectorAll('.directory-tile').length, 4)
  assert.equal(host.querySelector('.published-content-copy h3')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(host.querySelector('.published-content-copy img'), null)
  host.querySelector<HTMLAnchorElement>('.published-content-card')!.click()
  assert.deepEqual(calls, ['/content/safe-tool'])
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})
