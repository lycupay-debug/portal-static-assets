/** Tests for the framework-independent signal/ref adapter used by portal controllers. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { computed, nextTick, ref, watch } from '../src/state/reactivity'

test('cells notify scalar and computed observers after the current synchronous turn', async () => {
  const count = ref(1)
  const doubled = computed(() => count.value * 2)
  const observed: number[] = []
  const stop = watch(doubled, value => observed.push(value))
  count.value = 2
  count.value = 3
  assert.deepEqual(observed, [])
  await nextTick()
  assert.deepEqual(observed, [6])
  assert.equal(doubled.value, 6)
  stop()
  count.value = 4
  await nextTick()
  assert.deepEqual(observed, [6])
})

test('tuple watchers preserve current and previous snapshots and support synchronous flush', () => {
  const first = ref('a')
  const second = ref(1)
  const calls: Array<{ value: [string, number]; previous: [string, number] }> = []
  watch([first, second] as const, (value, previous) => calls.push({ value, previous }), { flush: 'sync' })
  first.value = 'b'
  second.value = 2
  assert.deepEqual(calls, [
    { value: ['b', 1], previous: ['a', 1] },
    { value: ['b', 2], previous: ['b', 1] },
  ])
})

test('deep watchers observe nested object and array mutation', async () => {
  const state = ref({ form: { title: 'Draft' }, tags: ['A'] })
  const observed: string[] = []
  watch(state, value => observed.push(`${value.form.title}:${value.tags.join(',')}`), { deep: true })
  state.value.form.title = 'Updated'
  state.value.tags.push('B')
  await nextTick()
  assert.deepEqual(observed, ['Updated:A,B'])
})

test('getter sources track dependencies and default callbacks are coalesced', async () => {
  const route = ref('/')
  const enabled = ref(false)
  const observed: Array<[string, boolean]> = []
  watch([() => route.value, () => enabled.value] as const, value => observed.push(value))
  route.value = '/account'
  enabled.value = true
  await nextTick()
  assert.deepEqual(observed, [['/account', true]])
})


test('watch scopes dispose every observer registered during mount', async () => {
  const scope = (await import('../src/state/reactivity')).createWatchScope()
  const value = ref(0)
  const seen: number[] = []
  scope.run(() => watch(value, current => seen.push(current)))
  value.value = 1
  await nextTick()
  assert.deepEqual(seen, [1])
  scope.dispose()
  value.value = 2
  await nextTick()
  assert.deepEqual(seen, [1])
})
