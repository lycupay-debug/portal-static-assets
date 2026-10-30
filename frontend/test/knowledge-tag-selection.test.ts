/** Framework-neutral tests for knowledge-tag draft state and focus/scheduling boundaries. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createKnowledgeTagSelection } from '../src/state/knowledgeTagSelection'
import type { MutableValue } from '../src/state/valueContracts'

function cell<T>(value: T): MutableValue<T> { return { value } }

test('tag selection edits a draft and applies normalized tags only on confirmation', async () => {
  const form = cell({ tags: ['数学'] })
  const state = createKnowledgeTagSelection(form, cell(false), { createValue: cell, afterRender: async () => undefined })
  await state.open()
  assert.equal(state.dialogOpen.value, true)
  assert.equal(state.isSelected('数学'), true)
  state.toggleTag('英语', true)
  assert.deepEqual(form.value.tags, ['数学'])
  state.close(true)
  assert.deepEqual(form.value.tags, ['数学', '英语'])
  assert.equal(state.dialogOpen.value, false)
})

test('tag picker refuses to open while busy and restores focus after closing', async () => {
  let focused = 0
  const trigger = { focus: () => { focused += 1 } } as HTMLButtonElement
  const dialog = { focus: () => { focused += 10 } } as HTMLElement
  const state = createKnowledgeTagSelection(cell({ tags: [] }), cell(true), { createValue: cell, afterRender: async () => undefined })
  state.trigger.value = trigger
  state.dialog.value = dialog
  await state.open()
  assert.equal(state.dialogOpen.value, false)
  assert.equal(focused, 0)
  const ready = createKnowledgeTagSelection(cell({ tags: [] }), cell(false), { createValue: cell, afterRender: async () => undefined })
  ready.trigger.value = trigger
  ready.dialog.value = dialog
  await ready.open()
  assert.equal(focused, 10)
  ready.close()
  await Promise.resolve()
  assert.equal(focused, 11)
})

test('tag group expansion and clear operations update their owned state', () => {
  const form = cell({ tags: ['数学'] })
  const state = createKnowledgeTagSelection(form, cell(false), { createValue: cell, afterRender: async () => undefined })
  assert.deepEqual(state.expandedGroups.value, ['教育'])
  state.toggleGroup('教育')
  assert.deepEqual(state.expandedGroups.value, [])
  state.toggleGroup('数学')
  assert.deepEqual(state.expandedGroups.value, ['数学'])
  state.removeSelected('数学')
  assert.deepEqual(form.value.tags, [])
  state.clearSelected()
  state.toggleTag('英语', true)
  state.clearDraft()
  assert.deepEqual(state.draft.value, [])
})
