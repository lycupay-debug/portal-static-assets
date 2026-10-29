/**
 * Responsibility: verify busy-state cleanup for successful and failed asynchronous operations.
 * Dependencies: Node test runner and the framework-independent lifecycle helper.
 * Collaboration: protects the busy-state invariant used by ComplianceCenter save and evidence actions.
 * Boundaries: no Vue component mounting, browser APIs, or live compliance API calls.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { runWithBusyState } from '../src/utils/runWithBusyState.ts'

test('busy state stays active until a successful operation completes, then clears', async () => {
  const states = []
  const result = await runWithBusyState(value => states.push(value), async () => {
    assert.deepEqual(states, [true])
    await Promise.resolve()
    assert.deepEqual(states, [true])
    return 'saved'
  })
  assert.equal(result, 'saved')
  assert.deepEqual(states, [true, false])
})

test('busy state clears when a post-save refresh rejects and the error remains observable', async () => {
  const states = []
  const failure = new Error('refresh failed')
  await assert.rejects(
    runWithBusyState(value => states.push(value), async () => { throw failure }),
    error => error === failure,
  )
  assert.deepEqual(states, [true, false])
})

test('busy state clears when an operation throws before returning a promise result', async () => {
  const states = []
  await assert.rejects(runWithBusyState(value => states.push(value), () => { throw new Error('sync failure') }))
  assert.deepEqual(states, [true, false])
})
