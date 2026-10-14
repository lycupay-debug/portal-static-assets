import assert from 'node:assert/strict'
import test from 'node:test'
import { JSDOM } from 'jsdom'
import createDOMPurify from 'dompurify'
import { renderMarkdown } from '../src/utils/markdown.ts'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
const purify = createDOMPurify(dom.window)
const render = (source) => renderMarkdown(source, purify)

test('renders headings, lists, links, fenced code and GFM tables', () => {
  const html = render('# Heading\n\n- one\n- two\n\n[docs](https://example.com)\n\n```js\nconst value = 1\n```\n\n| A | B |\n| - | - |\n| 1 | 2 |')
  assert.match(html, /<h1>Heading<\/h1>/)
  assert.match(html, /<ul>/)
  assert.match(html, /href="https:\/\/example\.com"/)
  assert.match(html, /<pre><code class="language-js">/)
  assert.match(html, /<table>/)
})

test('escapes raw HTML and removes executable URL schemes', () => {
  const html = render('<img src=x onerror="alert(1)"><script>alert(2)</script>\n\n[xss](javascript:alert%281%29)')
  assert.doesNotMatch(html, /<script\b/i)
  assert.doesNotMatch(html, /<img\b/i)
  assert.doesNotMatch(html, /href="javascript:/i)
  assert.match(html, /&lt;img/)
})

test('allows ordinary text, inline emphasis and safe links', () => {
  const html = render('A **bold** word and *italic* word.\n\n[secure](https://example.org/path)')
  assert.match(html, /<strong>bold<\/strong>/)
  assert.match(html, /<em>italic<\/em>/)
  assert.match(html, /href="https:\/\/example\.org\/path"/)
})

dom.window.close()
