/**
 * Regression tests for the native system-operations host.
 * Dependencies: Node test/assert APIs and jsdom; API and EventSource transports are stubbed locally.
 * Collaboration: protects log cursors/stream lifecycle, warning rendering, and policy-child ownership.
 * Boundaries: no production API, credential, command executor, or database is contacted by these tests.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountSystemOpsCenter } from '../src/components/systemOpsCenter'

const warningItems = [{ id: 7, source: 'server' as const, level: 'error', message: '<img src=x onerror=alert(1)>', action: 'execute_shell_error', time: '2026-10-30T00:00:00Z' }]
const logItems = [
  { id: 1, source: 'server' as const, level: 'info', message: 'server started', time: '2026-10-30T00:00:00Z' },
  { id: 2, source: 'server' as const, level: 'warning', message: 'disk usage high', time: '2026-10-30T00:01:00Z' },
]

class FakeEventSource {
  static instances: FakeEventSource[] = []
  readonly url: string
  readonly options?: EventSourceInit
  closed = false
  onopen: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  private handlers = new Map<string, Array<(event: Event) => void>>()
  constructor(url: string, options?: EventSourceInit) {
    this.url = url
    this.options = options
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, callback: EventListenerOrEventListenerObject): void {
    const handler = typeof callback === 'function' ? callback : (event: Event) => callback.handleEvent(event)
    this.handlers.set(type, [...(this.handlers.get(type) || []), handler])
  }
  emit(type: string, event: Event): void { for (const handler of this.handlers.get(type) || []) handler(event) }
  open(): void { this.onopen?.(new Event('open')) }
  close(): void { this.closed = true }
}

function setup() {
  FakeEventSource.instances = []
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: Array<{ url: string; method: string }> = []
  const originalFetch = globalThis.fetch
  const originalEventSource = (dom.window as unknown as { EventSource?: typeof EventSource }).EventSource
  Object.defineProperty(dom.window, 'EventSource', { configurable: true, value: FakeEventSource as unknown as typeof EventSource })
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input)
    calls.push({ url, method: init.method || 'GET' })
    const payload = url.includes('/system-ops/warnings')
      ? { items: warningItems, failedFileCleanupEvents: 2, checkedAt: '2026-10-30T00:00:00Z' }
      : url.includes('/system-ops/logs') ? { items: logItems, cursor: 2 }
        : url.endsWith('/admin/users') || url.endsWith('/admin/roles') || url.endsWith('/admin/permissions') ? { items: [] }
              : {}
    return new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as typeof fetch
  return {
    dom, host, calls,
    restore() {
      globalThis.fetch = originalFetch
      if (originalEventSource) Object.defineProperty(dom.window, 'EventSource', { configurable: true, value: originalEventSource })
      else delete (dom.window as unknown as { EventSource?: typeof EventSource }).EventSource
      dom.window.close()
    },
  }
}

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

test('warnings view safely renders server data and summary counts', async () => {
  const context = setup()
  const controller = mountSystemOpsCenter(context.host, { view: 'warnings' })
  try {
    await controller.ready
    assert.equal(context.host.querySelector('h1')?.textContent, '系统警告')
    assert.equal(context.host.querySelector('img'), null)
    assert.equal(context.host.querySelector('.ops-log-copy strong')?.textContent, '<img src=x onerror=alert(1)>')
    assert.equal(context.host.querySelector('.ops-summary-row article:nth-child(2) strong')?.textContent, '2')
    assert.equal(context.calls.filter(call => call.url.includes('/system-ops/warnings')).length, 1)
  } finally { controller.dispose(); context.restore() }
})

test('logs view maintains cursor, streams new records, filters, pauses, resumes and switches source', async () => {
  const context = setup()
  const controller = mountSystemOpsCenter(context.host, { view: 'logs' })
  try {
    await controller.ready
    assert.equal(context.host.querySelectorAll('.ops-live-list .ops-log-row').length, 2)
    assert.ok(context.calls[0].url.includes('source=server'))
    const firstStream = FakeEventSource.instances[0]
    assert.ok(firstStream.url.includes('after=2'))
    assert.equal(firstStream.options?.withCredentials, true)
    firstStream.open()
    assert.match(context.host.querySelector('.ops-connection')?.textContent || '', /实时连接中/)

    firstStream.emit('log', new context.dom.window.MessageEvent('log', { data: JSON.stringify({ id: 3, source: 'server', level: 'error', message: 'database unavailable' }) }))
    assert.equal(context.host.querySelectorAll('.ops-live-list .ops-log-row').length, 3)

    const search = context.host.querySelector<HTMLInputElement>('[data-focus-id="log-search"]')!
    search.focus()
    search.value = 'database'
    search.dispatchEvent(new context.dom.window.Event('input', { bubbles: true }))
    assert.equal(context.host.querySelectorAll('.ops-live-list .ops-log-row').length, 1)
    assert.equal(context.dom.window.document.activeElement, context.host.querySelector('[data-focus-id="log-search"]'))

    context.host.querySelector<HTMLButtonElement>('[data-action="toggle-pause"]')!.click()
    assert.equal(firstStream.closed, true)
    assert.match(context.host.querySelector('.ops-connection')?.textContent || '', /已暂停/)
    context.host.querySelector<HTMLButtonElement>('[data-action="toggle-pause"]')!.click()
    await flush()
    assert.ok(FakeEventSource.instances.length >= 2)

    context.host.querySelector<HTMLButtonElement>('[data-action="source-ssh"]')!.click()
    await flush()
    assert.ok(context.calls.some(call => call.url.includes('source=ssh')))
    assert.equal(FakeEventSource.instances.at(-1)?.url.includes('source=ssh'), true)
  } finally {
    const active = FakeEventSource.instances.at(-1)
    controller.dispose()
    assert.equal(active?.closed, true)
    context.restore()
  }
})

test('permissions view mounts only RBAC management and never requests a command policy or task queue', async () => {
  const context = setup()
  const controller = mountSystemOpsCenter(context.host, { view: 'permissions' })
  try {
    await controller.ready
    assert.ok(context.host.querySelector('.permission-center'))
    assert.equal(context.calls.some(call => call.url.includes('/system-ops/policy') || call.url.includes('/system-ops/tasks')), false)
    assert.ok(context.calls.some(call => call.url.endsWith('/admin/users')))
    assert.ok(context.calls.some(call => call.url.endsWith('/admin/roles')))
    assert.ok(context.calls.some(call => call.url.endsWith('/admin/permissions')))
  } finally { controller.dispose(); context.restore() }
})

test('dispose removes host content and stops the active EventSource', async () => {
  const context = setup()
  const controller = mountSystemOpsCenter(context.host, { view: 'logs' })
  await controller.ready
  const stream = FakeEventSource.instances[0]
  controller.dispose()
  assert.equal(stream.closed, true)
  assert.equal(context.host.children.length, 0)
  context.restore()
})
