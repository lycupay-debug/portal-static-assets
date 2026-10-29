/**
 * Regression tests for the native teaching-resource create/edit dialog.
 * Dependencies: Node test/assert APIs and jsdom; XMLHttpRequest is replaced with an in-memory fake.
 * Collaboration: protects resource payloads, file batching, validation, progress/lock state and callbacks.
 * Boundaries: tests never send a real upload or mutate the server; API semantics remain backend-owned.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import type { TeachingResource } from '../src/api/generated'
import { mountTeachingResourceEditorDialog } from '../src/components/teachingResourceEditorDialog'

type ResponseSpec = { status: number; body: unknown }
type XhrCall = { method: string; url: string; body: string; headers: Record<string, string> }

function setup(options: { resource?: TeachingResource | null; autoRespond?: boolean; respond?: (call: XhrCall, index: number) => ResponseSpec } = {}) {
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: XhrCall[] = []
  const busy: boolean[] = []
  const callbacks = { close: 0, saved: 0, completed: 0 }
  const originalXhr = dom.window.XMLHttpRequest
  const instances: FakeXhr[] = []
  class FakeXhr {
    static instances = instances
    upload: { onprogress: ((event: ProgressEvent) => void) | null } = { onprogress: null }
    status = 0
    responseText = ''
    withCredentials = false
    onload: ((event: ProgressEvent) => void) | null = null
    onerror: ((event: ProgressEvent) => void) | null = null
    private headers: Record<string, string> = {}
    private method = 'GET'
    private url = ''
    constructor() { instances.push(this) }
    open(method: string, url: string) { this.method = method; this.url = url }
    setRequestHeader(name: string, value: string) { this.headers[name] = value }
    send(body: string) {
      const call = { method: this.method, url: this.url, body, headers: this.headers }
      calls.push(call)
      if (options.autoRespond === false) return
      const response = options.respond?.(call, calls.length - 1) || { status: 201, body: { item: { id: `resource-${calls.length}`, title: 'saved' } } }
      queueMicrotask(() => this.complete(response))
    }
    complete(response: ResponseSpec) {
      this.status = response.status
      this.responseText = JSON.stringify(response.body)
      this.onload?.(new dom.window.ProgressEvent('load'))
    }
  }
  Object.defineProperty(dom.window, 'XMLHttpRequest', { configurable: true, value: FakeXhr as unknown as typeof XMLHttpRequest })
  const controller = mountTeachingResourceEditorDialog(host, {
    resource: options.resource ?? null,
    resourceTypeOptions: ['听力材料', '课件', '教学视频'],
    resourceGradeOptions: ['不限定年级', '七年级', '八年级'],
    onClose: () => { callbacks.close += 1 },
    onBusyChange: value => { busy.push(value) },
    onSaved: () => { callbacks.saved += 1 },
    onCompleted: () => { callbacks.completed += 1 },
  })
  return {
    dom, host, calls, busy, callbacks, instances, controller,
    restore() {
      controller.dispose()
      Object.defineProperty(dom.window, 'XMLHttpRequest', { configurable: true, value: originalXhr })
      dom.window.close()
    },
  }
}

const flush = () => new Promise(resolve => setTimeout(resolve, 0))
function submit(context: ReturnType<typeof setup>) {
  context.host.querySelector<HTMLFormElement>('.resource-create-form')!.dispatchEvent(new context.dom.window.Event('submit', { bubbles: true, cancelable: true }))
}
function setInput(context: ReturnType<typeof setup>, selector: string, value: string) {
  const input = context.host.querySelector<HTMLInputElement | HTMLTextAreaElement>(selector)!
  input.value = value
  input.dispatchEvent(new context.dom.window.Event('input', { bubbles: true }))
}

test('creates a URL-only resource and emits busy, saved, completed and close callbacks in order', async () => {
  const context = setup()
  try {
    setInput(context, '#resource-title', '七年级英语听力')
    setInput(context, '#resource-url', 'https://files.example.test/unit-1.pdf')
    const publicInput = context.host.querySelector<HTMLInputElement>('[data-field="isPublic"]')!
    publicInput.checked = true
    publicInput.dispatchEvent(new context.dom.window.Event('change', { bubbles: true }))
    submit(context)
    for (let attempt = 0; attempt < 10 && !context.host.querySelector('.resource-upload-completion'); attempt++) await flush()

    assert.equal(context.calls.length, 1)
    assert.equal(context.calls[0].method, 'POST')
    assert.equal(context.calls[0].url, '/api/resources')
    assert.deepEqual(JSON.parse(context.calls[0].body), {
      title: '七年级英语听力', description: '', type: '听力材料', grade: null,
      isPublic: true, url: 'https://files.example.test/unit-1.pdf',
    })
    assert.deepEqual(context.busy, [true, false])
    assert.equal(context.callbacks.saved, 1)
    assert.equal(context.callbacks.completed, 0)
    context.host.querySelector<HTMLButtonElement>('.resource-upload-completion button')!.click()
    assert.equal(context.callbacks.completed, 1)
    assert.equal(context.callbacks.close, 1)
  } finally { context.restore() }
})

test('rejects an invalid URL-only create before sending any upload request', async () => {
  const context = setup()
  try {
    setInput(context, '#resource-title', '测试资源')
    setInput(context, '#resource-url', 'ftp://files.example.test/file.pdf')
    submit(context)
    await flush()
    assert.equal(context.calls.length, 0)
    assert.ok(context.host.querySelector('.resource-error')?.textContent)
    assert.deepEqual(context.busy, [])
  } finally { context.restore() }
})

test('uploads batch files sequentially and preserves partial success details', async () => {
  const context = setup({ respond: (_call, index) => index === 0
    ? { status: 201, body: { item: { id: 'resource-one', title: 'one.pdf' } } }
    : { status: 500, body: { error: '存储失败' } } })
  try {
    const batch = context.host.querySelector<HTMLInputElement>('[data-field="batchMode"]')!
    batch.checked = true
    batch.dispatchEvent(new context.dom.window.Event('change', { bubbles: true }))
    setInput(context, '#resource-title', '批量教学资源')
    const files = [
      new context.dom.window.File(['first file'], 'one.pdf', { type: 'application/pdf' }),
      new context.dom.window.File(['second file'], 'two.pdf', { type: 'application/pdf' }),
    ]
    const fileInput = context.host.querySelector<HTMLInputElement>('#resource-file')!
    assert.equal(fileInput.multiple, true)
    Object.defineProperty(fileInput, 'files', { configurable: true, value: files })
    fileInput.dispatchEvent(new context.dom.window.Event('change', { bubbles: true }))
    assert.match(context.host.querySelector('.resource-selected-file')?.textContent || '', /已选择 2 个文件/)
    submit(context)
    for (let attempt = 0; attempt < 100 && !context.host.querySelector('.resource-upload-completion'); attempt++) await new Promise(resolve => setTimeout(resolve, 10))

    assert.ok(context.host.querySelector('.resource-upload-completion'), `upload did not finish: ${context.host.textContent}; calls=${context.calls.length}`)
    assert.equal(context.calls.length, 2)
    assert.equal(JSON.parse(context.calls[0].body).filename, 'one.pdf')
    assert.equal(JSON.parse(context.calls[1].body).filename, 'two.pdf')
    assert.ok(JSON.parse(context.calls[0].body).data)
    assert.match(context.host.querySelector('#resource-create-title + p')?.textContent || '', /成功创建 1 条资源，失败 1 条/)
    assert.match(context.host.querySelector('#resource-create-title + p')?.textContent || '', /存储失败/)
    assert.equal(context.callbacks.saved, 1)
  } finally { context.restore() }
})

test('editing updates the existing resource with PATCH and does not require a replacement file', async () => {
  const resource = {
    id: 'resource/123', title: '旧名称', description: '旧说明', type: '课件', grade: '七年级',
    isPublic: false, url: 'https://files.example.test/old.pdf',
  } as TeachingResource
  const context = setup({ resource })
  try {
    setInput(context, '#resource-title', '新名称')
    setInput(context, '#resource-description', '新说明')
    submit(context)
    for (let attempt = 0; attempt < 10 && !context.host.querySelector('.resource-upload-completion'); attempt++) await flush()
    assert.equal(context.calls.length, 1)
    assert.equal(context.calls[0].method, 'PATCH')
    assert.equal(context.calls[0].url, '/api/resources/resource%2F123')
    const payload = JSON.parse(context.calls[0].body)
    assert.equal(payload.title, '新名称')
    assert.equal(payload.description, '新说明')
    assert.equal('filename' in payload, false)
    assert.equal('data' in payload, false)
  } finally { context.restore() }
})

test('locks backdrop, Escape and close controls while upload is pending', async () => {
  const context = setup({ autoRespond: false })
  try {
    setInput(context, '#resource-title', '等待上传')
    setInput(context, '#resource-url', 'https://files.example.test/pending.pdf')
    submit(context)
    for (let attempt = 0; attempt < 10 && !context.calls.length; attempt++) await flush()
    assert.equal(context.calls.length, 1)
    assert.equal(context.host.querySelector<HTMLButtonElement>('.share-dialog-close')?.hidden, true)
    context.host.querySelector('.resource-upload-backdrop')!.dispatchEvent(new context.dom.window.MouseEvent('click', { bubbles: true }))
    context.host.querySelector('.resource-create-dialog')!.dispatchEvent(new context.dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    assert.equal(context.callbacks.close, 0)
    context.instances[0].complete({ status: 201, body: { item: { id: 'resource-pending', title: 'saved' } } })
    await flush()
    assert.ok(context.host.querySelector('.resource-upload-completion'))
    assert.equal(context.callbacks.close, 0)
  } finally { context.restore() }
})
