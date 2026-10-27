import assert from 'node:assert/strict'
import test from 'node:test'
import { ApiRequestError, apiRequest } from '../src/api/client.ts'

test('API transport sends same-origin credentials and JSON headers for bodies', async () => {
  const previous = globalThis.fetch
  let request
  globalThis.fetch = async (url, init) => {
    request = { url, init }
    return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'content-type': 'application/json' } })
  }
  try {
    const result = await apiRequest('/admin/roles', { method: 'POST', body: '{}' })
    assert.deepEqual(result, { ok: true })
    assert.equal(request.url, '/api/admin/roles')
    assert.equal(request.init.credentials, 'same-origin')
    assert.equal(request.init.headers.Accept, 'application/json')
    assert.equal(request.init.headers['Content-Type'], 'application/json')
  } finally {
    globalThis.fetch = previous
  }
})

test('API transport preserves caller cache and headers while supplying defaults', async () => {
  const previous = globalThis.fetch
  let request
  globalThis.fetch = async (url, init) => {
    request = { url, init }
    return new Response(JSON.stringify({ items: [] }), { status: 200, headers: { 'content-type': 'application/json' } })
  }
  try {
    await apiRequest('/system-ops/tasks', { cache: 'no-store', headers: { 'X-Test': 'present' } })
    assert.equal(request.init.cache, 'no-store')
    assert.equal(request.init.headers['X-Test'], 'present')
    assert.equal(request.init.headers.Accept, 'application/json')
  } finally {
    globalThis.fetch = previous
  }
})

test('API transport exposes structured HTTP errors including status and payload', async () => {
  const previous = globalThis.fetch
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'forbidden' }), { status: 403, headers: { 'content-type': 'application/json' } })
  try {
    await assert.rejects(apiRequest('/compliance/reviewers'), error => {
      assert.ok(error instanceof ApiRequestError)
      assert.equal(error.status, 403)
      assert.equal(error.message, 'forbidden')
      assert.deepEqual(error.payload, { error: 'forbidden' })
      return true
    })
  } finally {
    globalThis.fetch = previous
  }
})

test('API transport safely handles non-JSON error responses', async () => {
  const previous = globalThis.fetch
  globalThis.fetch = async () => new Response('upstream error', { status: 502, headers: { 'content-type': 'text/plain' } })
  try {
    await assert.rejects(apiRequest('/system-ops/logs'), error => {
      assert.ok(error instanceof ApiRequestError)
      assert.equal(error.status, 502)
      assert.equal(error.message, '请求失败（502）')
      assert.equal(error.payload, null)
      return true
    })
  } finally {
    globalThis.fetch = previous
  }
})
