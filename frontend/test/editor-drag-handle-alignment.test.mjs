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
  assert.match(component, /getBoundingClientRect:\s*\(\)\s*=> target\.getBoundingClientRect\(\)/)
})

test('drag handle has a stable line-sized hit area without inherited button baseline drift', () => {
  assert.match(theme, /\.content-editor-drag-handle\s*\{[^}]*height:\s*28px/s)
  assert.match(theme, /\.content-editor-drag-handle\s*>\s*span\s*\{[^}]*line-height:\s*1/s)
  assert.match(theme, /transform:\s*translateY\(-1px\)/)
})
