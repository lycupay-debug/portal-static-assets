/**
 * Framework-independence checks for the native Tiptap 3 integration.
 * Dependencies: Node test/assert APIs and jsdom; the production schema/extensions are reused unchanged.
 * Collaboration: verifies that @tiptap/core can replace the Vue adapter without changing Markdown behavior.
 * Boundaries: synthetic in-memory documents only; no uploads, network calls, or persisted content.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { createNativeContentEditor } from '../src/components/nativeContentEditor'

function setup(markdown = '# First heading\n\nInitial paragraph') {
  const dom = new JSDOM('<!doctype html><html><body><div id="editor"></div></body></html>', {
    url: 'https://portal.example.test/',
    pretendToBeVisual: true,
  })
  const win = dom.window
  const globals = ['window', 'document', 'navigator', 'HTMLElement', 'Element', 'Node', 'DOMParser', 'MutationObserver', 'getComputedStyle'] as const
  const previous = new Map<string, PropertyDescriptor | undefined>()
  for (const key of globals) {
    previous.set(key, Object.getOwnPropertyDescriptor(globalThis, key))
    Object.defineProperty(globalThis, key, { configurable: true, writable: true, value: key === 'window' ? win : key === 'document' ? win.document : key === 'navigator' ? win.navigator : key === 'HTMLElement' ? win.HTMLElement : key === 'Element' ? win.Element : key === 'Node' ? win.Node : key === 'DOMParser' ? win.DOMParser : key === 'MutationObserver' ? win.MutationObserver : win.getComputedStyle.bind(win) })
  }
  const host = win.document.querySelector<HTMLElement>('#editor')!
  const updates: string[] = []
  const editor = createNativeContentEditor({ element: host, markdown, onMarkdownChange: value => updates.push(value) })
  return {
    dom, host, editor, updates,
    restore() {
      editor.destroy()
      dom.window.close()
      for (const key of globals) {
        const descriptor = previous.get(key)
        if (descriptor) Object.defineProperty(globalThis, key, descriptor)
        else Reflect.deleteProperty(globalThis, key)
      }
    },
  }
}

test('core Editor mounts the shared schema and round-trips Markdown without the Vue adapter', () => {
  const context = setup()
  try {
    assert.ok(context.host.querySelector('.ProseMirror'))
    assert.equal(context.editor.getMarkdown(), '# First heading\n\nInitial paragraph')
    assert.equal(context.editor.isEditable, true)
    assert.equal(context.host.querySelector('[aria-label="文档正文编辑器"]'), context.editor.view.dom)
  } finally { context.restore() }
})

test('core Editor emits Markdown updates and supports programmatic load, undo and redo', () => {
  const context = setup()
  try {
    context.editor.commands.setContent('# Updated\n\nChanged text', { contentType: 'markdown' })
    assert.equal(context.editor.getMarkdown(), '# Updated\n\nChanged text')
    assert.equal(context.updates.at(-1), '# Updated\n\nChanged text')
    context.editor.commands.setContent('# Loaded from server', { contentType: 'markdown', emitUpdate: false })
    assert.equal(context.editor.getMarkdown().trimEnd(), '# Loaded from server')
    assert.equal(context.updates.length, 1)
  } finally { context.restore() }

  const history = setup('History baseline')
  try {
    history.editor.commands.insertContent(' changed')
    assert.match(history.editor.getMarkdown(), /changedHistory baseline/)
    history.editor.commands.undo()
    assert.equal(history.editor.getMarkdown().trimEnd(), 'History baseline')
    history.editor.commands.redo()
    assert.match(history.editor.getMarkdown(), /changedHistory baseline/)
  } finally { history.restore() }
})

test('core Editor preserves disabled/read-only behavior through setEditable', () => {
  const context = setup()
  try {
    context.editor.setEditable(false)
    assert.equal(context.editor.isEditable, false)
    context.editor.setEditable(true)
    assert.equal(context.editor.isEditable, true)
  } finally { context.restore() }
})
