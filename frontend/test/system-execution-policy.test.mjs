import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/SystemExecutionPolicyCenter.vue', import.meta.url), 'utf8')

test('failed policy writes remain visible after a reconciliation read', () => {
  const start = source.indexOf('async function saveMode(')
  const end = source.indexOf('\nasync function loadTasks(', start + 1)
  const body = source.slice(start, end)
  assert.notEqual(start, -1)
  assert.match(body, /const saveError =/)
  assert.match(body, /await loadPolicy\(\)/)
  assert.match(body, /const reloadError = error\.value/)
  assert.match(body, /error\.value = reloadError/)
  assert.match(body, /finally\s*\{ saving\.value = false \}/)
})

test('component request sequencing is explicit', () => {
  const component = readFileSync(new URL('../src/components/SystemOpsCenter.vue', import.meta.url), 'utf8')
  assert.ok(component.includes('const sequence = ++logLoadSequence'))
  assert.ok(component.includes('const source = activeSource.value'))
  assert.ok(component.includes('logLoadSequence += 1'))
  assert.ok(component.includes('sequence === logLoadSequence'))
  assert.ok(component.includes('sequence !== logLoadSequence'))
})

test('policy mutation remains explicit PUT and reload remains read-only GET', () => {
  const start = source.indexOf('async function saveMode(')
  const end = source.indexOf('\nasync function loadTasks(', start + 1)
  const body = source.slice(start, end)
  assert.match(body, /method: 'PUT'/)
  assert.match(body, /\/system-ops\/policy/)
  assert.match(source, /request<PolicyResponse>\('\/system-ops\/policy'\)/)
})
