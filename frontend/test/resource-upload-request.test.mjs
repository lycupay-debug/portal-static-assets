import assert from 'node:assert/strict'
import test from 'node:test'
import { requestWithUploadProgress } from '../src/utils/resourceUploadRequest.ts'

class FakeXMLHttpRequest {
  static latest
  upload = {}
  headers = {}
  withCredentials = false
  status = 0
  responseText = ''
  openArgs
  sentBody
  constructor() { FakeXMLHttpRequest.latest = this }
  open(...args) { this.openArgs = args }
  setRequestHeader(name, value) { this.headers[name] = value }
  send(body) { this.sentBody = body }
}

async function withFakeXHR(run) {
  const original = globalThis.XMLHttpRequest
  globalThis.XMLHttpRequest = FakeXMLHttpRequest
  try { await run() } finally { globalThis.XMLHttpRequest = original }
}

test('resource write request sends same-origin JSON and reports bounded upload progress', async () => {
  await withFakeXHR(async () => {
    const progress = []
    const body = { title: 'Lesson', description: '', type: '课件', grade: null, isPublic: false, url: null }
    const pending = requestWithUploadProgress('/api/resources', 'POST', body, value => progress.push(value))
    const xhr = FakeXMLHttpRequest.latest
    assert.deepEqual(xhr.openArgs, ['POST', '/api/resources'])
    assert.equal(xhr.withCredentials, true)
    assert.equal(xhr.headers['Content-Type'], 'application/json')
    assert.equal(xhr.headers.Accept, 'application/json')
    assert.deepEqual(JSON.parse(xhr.sentBody), body)

    xhr.upload.onprogress({ lengthComputable: true, loaded: 1, total: 3 })
    xhr.upload.onprogress({ lengthComputable: false, loaded: 1, total: 0 })
    xhr.status = 201
    xhr.responseText = JSON.stringify({ item: { id: 'resource-1', title: 'Lesson' } })
    xhr.onload()
    assert.deepEqual(await pending, { item: { id: 'resource-1', title: 'Lesson' } })
    assert.deepEqual(progress, [33, 100])
  })
})

test('resource write request preserves server error messages for non-success status', async () => {
  await withFakeXHR(async () => {
    const pending = requestWithUploadProgress('/api/resources', 'POST', {}, () => {})
    const xhr = FakeXMLHttpRequest.latest
    xhr.status = 413
    xhr.responseText = JSON.stringify({ error: '文件太大' })
    xhr.onload()
    await assert.rejects(pending, /文件太大/)
  })
})

test('resource write request rejects network errors with a user-safe message', async () => {
  await withFakeXHR(async () => {
    const pending = requestWithUploadProgress('/api/resources', 'POST', {}, () => {})
    FakeXMLHttpRequest.latest.onerror()
    await assert.rejects(pending, /网络连接失败，资源未能保存/)
  })
})

test('resource write request uses fallback errors for malformed error responses', async () => {
  await withFakeXHR(async () => {
    const pending = requestWithUploadProgress('/api/resources', 'POST', {}, () => {})
    const xhr = FakeXMLHttpRequest.latest
    xhr.status = 500
    xhr.responseText = 'not-json'
    xhr.onload()
    await assert.rejects(pending, /保存资源失败/)
  })
})
