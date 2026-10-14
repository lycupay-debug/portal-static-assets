import assert from 'node:assert/strict'
import test from 'node:test'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body><div id="editor"></div></body></html>', { pretendToBeVisual: true })
for (const key of ['window', 'document', 'navigator', 'Node', 'Element', 'HTMLElement', 'MutationObserver', 'getComputedStyle']) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: key === 'getComputedStyle' ? dom.window.getComputedStyle.bind(dom.window) : dom.window[key],
  })
}
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window)
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window)

const [
  { Editor }, { default: StarterKit }, { Markdown }, { Table }, { default: TableRow },
  { default: TableHeader }, { default: TableCell }, { default: TaskList }, { default: TaskItem },
  { default: Placeholder }, { default: Image }, { default: Highlight }, { default: TextAlign },
] = await Promise.all([
  import('@tiptap/core'), import('@tiptap/starter-kit'), import('@tiptap/markdown'),
  import('@tiptap/extension-table'), import('@tiptap/extension-table-row'),
  import('@tiptap/extension-table-header'), import('@tiptap/extension-table-cell'),
  import('@tiptap/extension-task-list'), import('@tiptap/extension-task-item'),
  import('@tiptap/extension-placeholder'), import('@tiptap/extension-image'),
  import('@tiptap/extension-highlight'), import('@tiptap/extension-text-align'),
])

function createEditor(markdown) {
  return new Editor({
    element: document.querySelector('#editor'),
    content: markdown,
    contentType: 'markdown',
    extensions: [
      StarterKit.configure({ heading: { levels: [1, 2, 3] } }), Markdown,
      Table.configure({ resizable: true }), TableRow, TableHeader, TableCell,
      TaskList, TaskItem.configure({ nested: true }), Placeholder, Image, Highlight,
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
    ],
  })
}

test('existing Markdown loads as formatted document blocks and remains Markdown-compatible', () => {
  const source = '# Existing title\n\nA **bold** paragraph.\n\n- first\n- second\n\n| Name | Value |\n| --- | --- |\n| alpha | 1 |\n\n> a quotation\n\n```js\nconst value = 1\n```'
  const editor = createEditor(source)
  try {
    const html = editor.getHTML()
    const output = editor.getMarkdown()
    assert.match(html, /<h1>Existing title<\/h1>/)
    assert.match(html, /<strong>bold<\/strong>/)
    assert.match(html, /<table/)
    assert.match(html, /<blockquote>/)
    assert.match(html, /<pre><code class="language-js">/)
    assert.match(output, /# Existing title/)
    assert.match(output, /\*\*bold\*\*/)
    assert.match(output, /\| Name\s+\| Value\s+\|/)
    assert.match(output, /```js/)
  } finally { editor.destroy() }
})

test('task lists and images preserve their Markdown representation', () => {
  const source = '## Checklist\n\n- [ ] pending task\n- [x] done task\n\n![example](https://example.com/image.png)'
  const editor = createEditor(source)
  try {
    assert.match(editor.getHTML(), /data-type="taskList"/)
    assert.match(editor.getHTML(), /<img src="https:\/\/example\.com\/image\.png"/)
    const output = editor.getMarkdown()
    assert.match(output, /- \[ \] pending task/)
    assert.match(output, /- \[x\] done task/)
    assert.match(output, /!\[example\]\(https:\/\/example\.com\/image\.png\)/)
  } finally { editor.destroy() }
})

test('task-list checkbox clicks update the checked state and Markdown output', () => {
  const editor = createEditor('- [ ] pending task\n- [x] done task')
  try {
    const checkboxes = editor.view.dom.querySelectorAll('ul[data-type="taskList"] > li > label input[type="checkbox"]')
    assert.equal(checkboxes.length, 2)
    assert.equal(checkboxes[0].checked, false)
    checkboxes[0].click()
    assert.equal(editor.view.dom.querySelectorAll('ul[data-type="taskList"] > li > label input[type="checkbox"]')[0].checked, true)
    assert.match(editor.getMarkdown(), /- \[x\] pending task/)
    assert.match(editor.getMarkdown(), /- \[x\] done task/)
  } finally { editor.destroy() }
})

test('nested task lists preserve both checkbox items and ordinary nested bullets', () => {
  const source = '- [ ] parent\n  - [ ] checkbox child\n  - plain child'
  const editor = createEditor(source)
  try {
    const html = editor.getHTML()
    const output = editor.getMarkdown()
    assert.match(html, /checkbox child/)
    assert.match(html, /plain child/)
    assert.match(output, /- \[ \] parent/)
    assert.match(output, /checkbox child/)
    assert.match(output, /plain child/)
  } finally { editor.destroy() }
})

dom.window.close()
