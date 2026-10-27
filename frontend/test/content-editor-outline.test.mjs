import assert from 'node:assert/strict'
import test from 'node:test'
import { collectContentEditorOutline } from '../src/composables/useContentEditorOutline.ts'

function editorWithNodes(nodes) {
  return {
    state: {
      doc: {
        descendants(callback) {
          for (const node of nodes) callback(node)
        },
      },
    },
  }
}

test('outline collects non-empty headings, preserves document order, and ignores other nodes', () => {
  const outline = collectContentEditorOutline(editorWithNodes([
    { type: { name: 'paragraph' }, textContent: 'not a heading', attrs: {} },
    { type: { name: 'heading' }, textContent: '  Overview  ', attrs: { level: 1 } },
    { type: { name: 'heading' }, textContent: '   ', attrs: { level: 2 } },
    { type: { name: 'heading' }, textContent: 'Details', attrs: { level: 3 } },
  ]))
  assert.deepEqual(outline, [
    { level: 1, text: 'Overview', index: 0 },
    { level: 3, text: 'Details', index: 2 },
  ])
})

test('outline handles absent editor state without throwing', () => {
  assert.deepEqual(collectContentEditorOutline(undefined), [])
  assert.deepEqual(collectContentEditorOutline(null), [])
})
