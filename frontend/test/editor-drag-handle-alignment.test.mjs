import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'

const component = await readFile(new URL('../src/components/ContentDocumentEditor.vue', import.meta.url), 'utf8')
const theme = await readFile(new URL('../src/theme.css', import.meta.url), 'utf8')

test('drag handle anchors to the first visible text line of multiline blocks', () => {
  assert.match(component, /onNodeChange:\s*change\s*=>/)
  assert.match(component, /currentEditor\.view\.nodeDOM\(pos\)/)
  assert.match(component, /getReferencedVirtualElement:\s*\(\)\s*=>/)
  assert.match(component, /:scope > li > div > p:first-child/)
  assert.match(component, /const listRect = root\.getBoundingClientRect\(\)/)
  assert.match(component, /const handleHeight = 32/)
  assert.match(component, /const alignedTop = rect\.top \+ \(rect\.height - handleHeight\) \/ 2/)
  assert.match(component, /new DOMRect\(rect\.left, alignedTop, rect\.width, rect\.height\)/)
  assert.match(component, /new DOMRect\(listRect\.left, alignedTop, listRect\.width, rect\.height\)/)
  assert.match(component, /For lists, anchor horizontally to the list container's outer edge/)
})

test('drag handle uses a larger six-dot icon and an accessible 30 by 32 pixel hit area', () => {
  assert.match(theme, /\.content-editor-drag-handle\s*\{[^}]*width:\s*30px/s)
  assert.match(theme, /\.content-editor-drag-handle\s*\{[^}]*height:\s*32px/s)
  assert.match(theme, /\.content-editor-drag-icon\s*\{[^}]*width:\s*20px/s)
  assert.match(component, /viewBox=\"0 0 20 20\"/ )
  assert.match(component, /<circle cx=\"6\" cy=\"4\"/)
})
