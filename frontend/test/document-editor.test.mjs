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

const { Editor } = await import('@tiptap/core')
const { createContentEditorExtensions, createContentEditorTools, executeContentEditorTool, normalizeEditorImageUrl, normalizeEditorLinkUrl } = await import('../src/utils/contentEditor.ts')

function createEditor(markdown) {
  return new Editor({
    element: document.querySelector('#editor'),
    content: markdown,
    contentType: 'markdown',
    extensions: createContentEditorExtensions(),
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

test('alignment, underline and highlight survive a Markdown round trip', () => {
  const source = ':::align-center\n## centered **heading**\n:::\n\n:::align-right\ntext with ++underline++ and ==highlight==\n:::'
  const editor = createEditor(source)
  try {
    const json = editor.getJSON()
    assert.equal(json.content[0].attrs.textAlign, 'center')
    assert.equal(json.content[1].attrs.textAlign, 'right')
    const output = editor.getMarkdown()
    assert.ok(output.includes(':::align-center\n## centered **heading**\n:::'))
    assert.match(output, /:::align-right/)
    assert.ok(output.includes('++underline++'))
    assert.match(output, /==highlight==/)
    const reloaded = createEditor(output)
    try {
      const roundTrip = reloaded.getJSON()
      assert.equal(roundTrip.content[0].attrs.textAlign, 'center')
      assert.equal(roundTrip.content[1].attrs.textAlign, 'right')
      assert.ok(reloaded.getHTML().includes('<u>underline</u>'))
      assert.ok(reloaded.getHTML().includes('<mark>highlight</mark>'))
    } finally { reloaded.destroy() }
  } finally { editor.destroy() }
})

test('every slash-menu tool is registered and executable without throwing', () => {
  const actions = {
    addImage: editor => editor.chain().focus().setImage({ src: 'https://example.com/image.png', alt: 'test image' }).run(),
    addLink: editor => editor.chain().focus().insertContent({ type: 'text', text: 'https://example.com', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }] }).run(),
  }
  const expected = [
    '文本', '一级标题', '二级标题', '三级标题', '项目符号列表', '编号列表', '待办清单', '引用', '提示块',
    '代码块', '分割线', '表格', '图片', '链接', '加粗', '斜体', '下划线', '删除线', '行内代码',
    '高亮文本', '左对齐', '居中对齐', '右对齐', '清除格式', '撤销', '重做',
  ]
  const editor = createEditor('Tool test text')
  try {
    const tools = createContentEditorTools(editor, actions)
    assert.deepEqual(tools.map(tool => tool.label), expected)
    for (const tool of tools) {
      editor.commands.setContent('Tool test text')
      editor.commands.setTextSelection({ from: 1, to: 6 })
      assert.doesNotThrow(() => tool.run(), tool.label)
    }
  } finally { editor.destroy() }
})

test('content tools apply their intended block and mark types', () => {
  const actions = {
    addImage: editor => editor.chain().focus().setImage({ src: 'https://example.com/image.png', alt: 'test image' }).run(),
    addLink: editor => editor.chain().focus().insertContent({ type: 'text', text: 'linked', marks: [{ type: 'link', attrs: { href: 'https://example.com' } }] }).run(),
  }
  const checks = [
    ['文本', node => node.type === 'paragraph'],
    ['一级标题', node => node.type === 'heading' && node.attrs.level === 1],
    ['二级标题', node => node.type === 'heading' && node.attrs.level === 2],
    ['三级标题', node => node.type === 'heading' && node.attrs.level === 3],
    ['项目符号列表', node => node.type === 'bulletList'],
    ['编号列表', node => node.type === 'orderedList'],
    ['待办清单', node => node.type === 'taskList'],
    ['引用', node => node.type === 'blockquote'],
    ['提示块', node => node.type === 'blockquote' && JSON.stringify(node).includes('提示：')],
    ['代码块', node => node.type === 'codeBlock'],
    ['分割线', node => node.type === 'horizontalRule'],
    ['表格', node => node.type === 'table'],
    ['图片', node => node.type === 'image'],
    ['链接', node => JSON.stringify(node).includes('"type":"link"')],
    ['加粗', node => JSON.stringify(node).includes('"type":"bold"')],
    ['斜体', node => JSON.stringify(node).includes('"type":"italic"')],
    ['下划线', node => JSON.stringify(node).includes('"type":"underline"')],
    ['删除线', node => JSON.stringify(node).includes('"type":"strike"')],
    ['行内代码', node => JSON.stringify(node).includes('"type":"code"')],
    ['高亮文本', node => JSON.stringify(node).includes('"type":"highlight"')],
    ['左对齐', node => node.attrs?.textAlign === 'left'],
    ['居中对齐', node => node.attrs?.textAlign === 'center'],
    ['右对齐', node => node.attrs?.textAlign === 'right'],
    ['清除格式', node => node.type === 'paragraph' && !node.content?.some(child => child.marks?.length)],
  ]
  for (const [label, check] of checks) {
    const editor = createEditor('Tool test text')
    try {
      editor.commands.setTextSelection({ from: 1, to: 6 })
      const tool = createContentEditorTools(editor, actions).find(item => item.label === label)
      assert.ok(tool, 'missing tool: ' + label)
      tool.run()
      const json = editor.getJSON()
      const allNodes = []
      const visit = node => { allNodes.push(node); for (const child of node.content || []) visit(child) }
      visit(json)
      assert.ok(allNodes.some(check), 'tool did not apply expected effect: ' + label + '\n' + JSON.stringify(json))
    } finally { editor.destroy() }
  }
})

test('link and image tools reject unsafe URL schemes', () => {
  assert.equal(normalizeEditorLinkUrl('example.com/path'), 'https://example.com/path')
  assert.equal(normalizeEditorLinkUrl('mailto:hello@example.com'), 'mailto:hello@example.com')
  assert.equal(normalizeEditorLinkUrl('javascript:alert(1)'), null)
  assert.equal(normalizeEditorLinkUrl('data:text/html,hi'), null)
  assert.equal(normalizeEditorImageUrl('https://example.com/image.png'), 'https://example.com/image.png')
  assert.equal(normalizeEditorImageUrl('javascript:alert(1)'), null)
  assert.equal(normalizeEditorImageUrl('data:image/svg+xml,hi'), null)
})

test('slash execution removes only the slash command and applies the selected block', () => {
  const editor = createEditor('/待办')
  try {
    editor.commands.setTextSelection(4)
    const tool = createContentEditorTools(editor, {
      addImage: current => current.chain().focus().setImage({ src: 'https://example.com/image.png' }).run(),
      addLink: current => current.chain().focus().insertContent('https://example.com').run(),
    }).find(item => item.label === '待办清单')
    assert.ok(tool)
    executeContentEditorTool(editor, tool)
    assert.match(editor.getHTML(), /data-type="taskList"/)
    assert.doesNotMatch(editor.getMarkdown(), /待办/)
  } finally { editor.destroy() }
})

dom.window.close()
