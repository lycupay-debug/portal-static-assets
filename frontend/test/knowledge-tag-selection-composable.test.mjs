import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, ref } from 'vue'
import { useKnowledgeTagSelection } from '../src/composables/useKnowledgeTagSelection.ts'

function setup(tags = []) {
  const form = ref({ tags })
  const busy = ref(false)
  return { form, busy, picker: useKnowledgeTagSelection(form, busy) }
}

test('knowledge tag picker stages changes until apply and normalizes parent categories', async () => {
  const { form, picker } = setup(['Python'])
  await picker.open()
  assert.equal(picker.dialogOpen.value, true)
  assert.ok(picker.draft.value.includes('编程'))
  picker.toggleTag('数据库', true)
  assert.equal(form.value.tags.includes('数据库'), false)
  picker.close(true)
  assert.deepEqual(form.value.tags, ['编程', 'Python', '数据库'])
  assert.equal(picker.dialogOpen.value, false)
})

test('knowledge tag picker discards draft changes on cancel and supports clearing', async () => {
  const { form, picker } = setup(['Python'])
  await picker.open()
  picker.toggleTag('数据库', true)
  picker.close(false)
  assert.deepEqual(form.value.tags, ['Python'])
  await picker.open()
  picker.clearDraft()
  assert.deepEqual(picker.draft.value, [])
  picker.close(true)
  assert.deepEqual(form.value.tags, [])
})

test('knowledge tag picker refuses opening while content is busy', async () => {
  const { busy, picker } = setup(['Python'])
  busy.value = true
  await picker.open()
  await nextTick()
  assert.equal(picker.dialogOpen.value, false)
})
