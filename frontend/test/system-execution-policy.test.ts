/**
 * Regression tests for the native system execution-policy and task console.
 * Dependencies: Node test/assert APIs and jsdom; API responses and confirmation prompts are stubbed.
 * Collaboration: protects task payload validation, policy writes, and explicit confirmation gates.
 * Boundaries: tests never contact the server or execute commands; the Go executor remains authoritative.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountSystemExecutionPolicyCenter } from '../src/components/systemExecutionPolicyCenter'

const policy = {
  mode: 'stable' as const,
  updatedAt: '2026-10-30T00:00:00Z',
  changedBy: 'admin@example.com',
  enforcement: 'executor_connected' as const,
  executorStatus: 'online',
}
const tasks = [{
  taskId: 'task-1', idempotencyKey: 'idem-1', title: '<img src=x onerror=alert(1)>', taskClass: 'read_only',
  executable: '/usr/bin/uname', arguments: ['-a'], workingDirectory: '/opt/personal-portal', timeoutSeconds: 120,
  requestedBy: 'admin@example.com', modeAtSubmission: 'stable', status: 'pending_approval', approvalRequired: true,
  createdAt: '2026-10-30T00:00:00Z', updatedAt: '2026-10-30T00:00:00Z', stdout: Array.from({ length: 80 }, (_, index) => `output line ${index}`).join('\n'), stderr: '', attempt: 0,
}]

function setup() {
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: Array<{ url: string; method: string; body?: string }> = []
  const originalFetch = globalThis.fetch
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input)
    const method = init.method || 'GET'
    const body = typeof init.body === 'string' ? init.body : undefined
    calls.push({ url, method, body })
    let payload: unknown = {}
    if (url.endsWith('/system-ops/policy') && method === 'GET') payload = policy
    else if (url.endsWith('/system-ops/policy') && method === 'PUT') payload = { ...policy, mode: JSON.parse(body || '{}').mode }
    else if (url.endsWith('/system-ops/tasks') && method === 'GET') payload = { items: tasks }
    else if (url.endsWith('/system-ops/tasks') && method === 'POST') payload = { taskId: 'task-new', status: 'pending_approval', approvalRequired: true, policyDecision: 'approval_required', mode: 'stable' }
    return new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as typeof fetch
  const controller = mountSystemExecutionPolicyCenter(host)
  return {
    dom, host, calls, controller,
    restore() { controller.dispose(); dom.window.close(); globalThis.fetch = originalFetch },
  }
}

async function flush() { await new Promise(resolve => setTimeout(resolve, 0)) }

test('policy mode changes are local until explicitly saved and can be reset', async () => {
  const context = setup()
  try {
    await context.controller.ready
    assert.equal(context.host.querySelector('.policy-current strong')?.textContent, '稳定运行模式')
    assert.equal(context.calls.some(call => call.method === 'PUT'), false)
    context.host.querySelector<HTMLButtonElement>('[data-action="toggle-mode"]')!.click()
    assert.equal(context.host.querySelector('[role="switch"]')?.getAttribute('aria-checked'), 'true')
    assert.match(context.host.querySelector('.policy-actions')?.textContent || '', /尚未保存/)
    context.host.querySelector<HTMLButtonElement>('[data-action="reset-mode"]')!.click()
    assert.equal(context.host.querySelector('[role="switch"]')?.getAttribute('aria-checked'), 'false')
    context.host.querySelector<HTMLButtonElement>('[data-action="toggle-mode"]')!.click()
    context.host.querySelector<HTMLButtonElement>('[data-action="save-mode"]')!.click()
    for (let attempt = 0; attempt < 10 && !context.calls.some(call => call.method === 'PUT'); attempt++) await flush()
    const update = context.calls.find(call => call.method === 'PUT')
    assert.equal(update?.url, '/api/system-ops/policy')
    assert.deepEqual(JSON.parse(update?.body || '{}'), { mode: 'developer' })
    await flush()
    assert.match(context.host.querySelector('[role="status"]')?.textContent || '', /运行模式已保存/)
  } finally { context.restore() }
})

test('task arguments must be a string array and task creation is explicit with shell mode disabled', async () => {
  const context = setup()
  try {
    await context.controller.ready
    const submitWithArguments = (value: string) => {
      const form = context.host.querySelector<HTMLFormElement>('.task-create-form')!
      const args = form.querySelector<HTMLTextAreaElement>('[data-field="argumentsText"]')!
      args.value = value
      args.dispatchEvent(new context.dom.window.Event('input', { bubbles: true }))
      form.dispatchEvent(new context.dom.window.Event('submit', { bubbles: true, cancelable: true }))
    }
    submitWithArguments('{bad json')
    await flush()
    assert.match(context.host.querySelector('[role="alert"]')?.textContent || '', /合法 JSON 数组/)
    assert.equal(context.calls.some(call => call.method === 'POST' && call.url.endsWith('/system-ops/tasks')), false)

    submitWithArguments('["-a", 5]')
    await flush()
    assert.match(context.host.querySelector('[role="alert"]')?.textContent || '', /字符串数组/)
    assert.equal(context.calls.some(call => call.method === 'POST' && call.url.endsWith('/system-ops/tasks')), false)

    submitWithArguments('["-a"]')
    for (let attempt = 0; attempt < 10 && !context.calls.some(call => call.method === 'POST'); attempt++) await flush()
    const submission = context.calls.find(call => call.method === 'POST' && call.url.endsWith('/system-ops/tasks'))
    assert.ok(submission)
    const payload = JSON.parse(submission.body || '{}')
    assert.deepEqual(payload.arguments, ['-a'])
    assert.equal(payload.shellMode, false)
    assert.equal(payload.executable, '/usr/bin/uname')
    assert.ok(payload.idempotencyKey)
  } finally { context.restore() }
})

test('approval and cancellation require confirmation before sending POST requests', async () => {
  const context = setup()
  try {
    await context.controller.ready
    assert.equal(context.host.querySelector('img'), null)
    assert.equal(context.host.querySelector('.task-item-head strong')?.textContent, '<img src=x onerror=alert(1)>')
    const confirmCalls: string[] = []
    context.dom.window.confirm = message => { confirmCalls.push(message); return false }
    context.host.querySelector<HTMLButtonElement>('[data-action="approve-task"]')!.click()
    context.host.querySelector<HTMLButtonElement>('[data-action="cancel-task"]')!.click()
    await flush()
    assert.equal(context.calls.some(call => call.method === 'POST' && call.url.includes('/approve')), false)
    assert.equal(context.calls.some(call => call.method === 'POST' && call.url.includes('/cancel')), false)

    context.dom.window.confirm = message => { confirmCalls.push(message); return true }
    context.host.querySelector<HTMLButtonElement>('[data-action="approve-task"]')!.click()
    for (let attempt = 0; attempt < 10 && !context.calls.some(call => call.url.includes('/approve')); attempt++) await flush()
    context.host.querySelector<HTMLButtonElement>('[data-action="cancel-task"]')!.click()
    for (let attempt = 0; attempt < 10 && !context.calls.some(call => call.url.includes('/cancel')); attempt++) await flush()
    assert.ok(context.calls.some(call => call.url === '/api/system-ops/tasks/task-1/approve' && call.method === 'POST'))
    assert.ok(context.calls.some(call => call.url === '/api/system-ops/tasks/task-1/cancel' && call.method === 'POST'))
    assert.equal(confirmCalls.length, 4)
  } finally { context.restore() }
})


test('task polling preserves focused form fields and expanded output state', async () => {
  const context = setup()
  try {
    await context.controller.ready
    const title = context.host.querySelector<HTMLInputElement>('[data-field="title"]')!
    title.focus()
    title.value = '正在输入的任务标题'
    title.dispatchEvent(new context.dom.window.Event('input', { bubbles: true }))
    const details = context.host.querySelector<HTMLDetailsElement>('.task-output')!
    details.open = true
    const output = details.querySelector<HTMLElement>('pre')!
    output.scrollTop = 20

    await context.controller.refreshTasks()

    const refreshedTitle = context.host.querySelector<HTMLInputElement>('[data-field="title"]')!
    const refreshedDetails = context.host.querySelector<HTMLDetailsElement>('.task-output')!
    const refreshedOutput = refreshedDetails.querySelector<HTMLElement>('pre')!
    assert.equal(context.dom.window.document.activeElement, refreshedTitle)
    assert.equal(refreshedTitle.value, '正在输入的任务标题')
    assert.equal(refreshedDetails.open, true)
    assert.equal(refreshedOutput.scrollTop, 20)
  } finally { context.restore() }
})

test('dispose clears the task polling timer, DOM and pending response ownership', async () => {
  const context = setup()
  context.controller.dispose()
  await context.controller.ready
  assert.equal(context.host.children.length, 0)
  context.restore()
})
