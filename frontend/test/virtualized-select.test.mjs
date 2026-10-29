/**
 * Responsibility: guard VirtualizedSelect's selected-label contract at source boundaries.
 * Dependencies: Node test runner; no browser or production API is required.
 * Collaboration: App.vue and TeachingResourceEditorDialog.vue provide options and own v-model state.
 * Limits: these assertions do not mount Vue or simulate browser keyboard/focus behavior.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/VirtualizedSelect.vue', import.meta.url), 'utf8')

test('a selected option with an empty label is not replaced by the placeholder', () => {
  assert.match(source, /selected\?\.label\s*\?\?\s*placeholder/)
  assert.doesNotMatch(source, /selected\?\.label\s*\|\|\s*placeholder/)
})

test('the select keeps caller-owned model state and valid empty-string option values', () => {
  assert.match(source, /modelValue: string \| null/)
  assert.match(source, /type Option = \{ label: string; value: string \}/)
  assert.match(source, /emit\('update:modelValue', value\)/)
})
