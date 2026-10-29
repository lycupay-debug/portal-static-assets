/**
 * Responsibility: verify debounce timing, latest-value behavior, argument validation, and scope cleanup.
 * Dependencies: Vue effect scopes/refs and the project's debounce composable; no network or wall-clock assumptions beyond bounded waits.
 * Collaboration: these tests exercise the composable contract used by table and public-directory search.
 * Side effects: creates and stops isolated Vue effect scopes; no application state is shared between tests.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { effectScope, nextTick, ref } from 'vue'
import { useDebouncedValue } from '../src/composables/useDebouncedValue.ts'

const wait = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))

test('debounced value publishes only the latest source value after the quiet period', async () => {
  const scope = effectScope()
  const source = ref('')
  const debounced = scope.run(() => useDebouncedValue(source, 25))

  source.value = 'f'
  await nextTick()
  await wait(10)
  source.value = 'fi'
  await nextTick()
  await wait(10)
  source.value = 'final'
  await nextTick()
  assert.equal(debounced.value, '')
  await wait(35)
  assert.equal(debounced.value, 'final')
  scope.stop()
})

test('stopping the owning scope cancels a pending debounced update', async () => {
  const scope = effectScope()
  const source = ref('initial')
  const debounced = scope.run(() => useDebouncedValue(source, 20))
  source.value = 'pending'
  await nextTick()
  scope.stop()
  await wait(30)
  assert.equal(debounced.value, 'initial')
})

test('debounce delay must be finite and non-negative', () => {
  const source = ref('value')
  const scope = effectScope()
  assert.throws(() => scope.run(() => useDebouncedValue(source, -1)), RangeError)
  assert.throws(() => scope.run(() => useDebouncedValue(source, Number.NaN)), RangeError)
  scope.stop()
})
