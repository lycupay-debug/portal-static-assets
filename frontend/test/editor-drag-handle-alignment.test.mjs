import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../src/components/ContentDocumentEditor.vue', import.meta.url), 'utf8')
const dragHandle = await readFile(new URL('../src/utils/contentEditorDragHandle.ts', import.meta.url), 'utf8')
const theme = await readFile(new URL('../src/theme.css', import.meta.url), 'utf8')

test('drag handle anchors to the first visible text line of multiline blocks', () => {
  assert.match(dragHandle, /onNodeChange:\s*change\s*=>/)
  assert.match(dragHandle, /currentEditor\.view\.nodeDOM\(pos\)/)
  assert.match(dragHandle, /getReferencedVirtualElement:\s*\(\)\s*=>/)
  assert.match(dragHandle, /:scope > li > div > p:first-child/)
  assert.match(dragHandle, /const listRect = root\.getBoundingClientRect\(\)/)
  assert.match(dragHandle, /const handleHeight = 32/)
  assert.match(dragHandle, /const alignedTop = rect\.top \+ \(rect\.height - handleHeight\) \/ 2/)
  assert.match(dragHandle, /new DOMRect\(rect\.left, alignedTop, rect\.width, rect\.height\)/)
  assert.match(dragHandle, /new DOMRect\(listRect\.left, alignedTop, listRect\.width, rect\.height\)/)
  assert.match(dragHandle, /For lists, anchor horizontally to the list container's outer edge/)
})

  assert.match(component, /createContentEditorDragHandle\(\)/)
test('drag handle uses a larger six-dot icon and an accessible 30 by 32 pixel hit area', () => {
  assert.match(theme, /\.content-editor-drag-handle\s*\{[^}]*width:\s*30px/s)
  assert.match(theme, /\.content-editor-drag-handle\s*\{[^}]*height:\s*32px/s)
  assert.match(theme, /\.content-editor-drag-icon\s*\{[^}]*width:\s*20px/s)
  assert.match(dragHandle, /viewBox=\"0 0 20 20\"/ )
  assert.match(dragHandle, /<circle cx=\"6\" cy=\"4\"/)
})
