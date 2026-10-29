/**
 * Responsibility: verify lazy module initialization caching and recovery after initialization failure.
 * Dependencies: Node test runner and the framework-independent retryable lazy-loader helper.
 * Collaboration: protects App.vue's dynamic list-loader initialization boundary without mounting the app.
 * Boundaries: does not simulate Vite chunk fetching or a browser deployment; the factory's promise behavior
 * is deterministic and injected by each test.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { createRetryableLazyLoader } from '../src/utils/retryableLazyLoader.ts'

test('concurrent initialization shares one promise and caches a successful value', async () => {
  let calls = 0
  const value = { initialized: true }
  const get = createRetryableLazyLoader(async () => {
    calls += 1
    await Promise.resolve()
    return value
  })

  const first = get()
  const second = get()
  assert.strictEqual(first, second)
  assert.strictEqual(await first, value)
  assert.strictEqual(await get(), value)
  assert.equal(calls, 1)
})

test('a rejected initialization clears the cached promise and allows a later retry', async () => {
  let calls = 0
  const failure = new Error('chunk load failed')
  const value = { initialized: true }
  const get = createRetryableLazyLoader(async () => {
    calls += 1
    if (calls === 1) throw failure
    return value
  })

  await assert.rejects(get(), error => error === failure)
  assert.strictEqual(await get(), value)
  assert.strictEqual(await get(), value)
  assert.equal(calls, 2)
})

test('a synchronous factory throw is exposed and can be retried', async () => {
  let calls = 0
  const get = createRetryableLazyLoader(() => {
    calls += 1
    if (calls === 1) throw new Error('synchronous initialization failure')
    return Promise.resolve('ready')
  })

  await assert.rejects(get(), /synchronous initialization failure/)
  assert.equal(await get(), 'ready')
  assert.equal(calls, 2)
})
