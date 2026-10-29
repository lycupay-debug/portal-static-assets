/**
 * Regression tests for the native permission-management host.
 * Dependencies: Node's test/assert APIs and jsdom; all API responses are locally stubbed.
 * Collaboration: protects safe rendering, selection synchronization, and explicit admin writes.
 * Boundaries: does not validate Go authorization; the server remains the permission authority.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountPermissionManagement } from '../src/components/permissionManagement'

const users = [
  { id: 'admin-id', email: 'lycupay@gmail.com', displayName: '<img src=x onerror=alert(1)>', isAdmin: true, isCopyrightReviewer: false, createdAt: '', roles: ['super_admin'] },
  { id: 'user-1', email: 'person@example.com', displayName: 'Alice', isAdmin: false, isCopyrightReviewer: false, createdAt: '', roles: ['member'] },
]
const roles = [
  { roleKey: 'super_admin', displayName: '超级管理员', description: '完整权限', isSystem: true, permissions: ['content.read'] },
  { roleKey: 'member', displayName: '成员', description: '基础账户角色', isSystem: false, permissions: ['content.read'] },
  { roleKey: 'reviewer', displayName: '审核员', description: '审核内容', isSystem: false, permissions: [] },
]
const permissions = [
  { permissionKey: 'content.read', description: '读取内容' },
  { permissionKey: 'content.review', description: '审核内容' },
]

function setup() {
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', { url: 'https://portal.example.test/' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: Array<{ url: string; method: string; body?: string }> = []
  const originalFetch = globalThis.fetch
  globalThis.fetch = (async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input)
    const method = init.method || 'GET'
    calls.push({ url, method, body: typeof init.body === 'string' ? init.body : undefined })
    const payload = url.endsWith('/admin/users') ? { items: users }
      : url.endsWith('/admin/roles') ? { items: roles }
        : url.endsWith('/admin/permissions') ? { items: permissions } : {}
    return new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })
  }) as typeof fetch
  const controller = mountPermissionManagement(host)
  return {
    dom, host, calls, controller,
    restore() { controller.dispose(); dom.window.close(); globalThis.fetch = originalFetch },
  }
}

test('renders untrusted account labels as text and filters users without losing search focus', async () => {
  const context = setup()
  try {
    await context.controller.ready
    assert.equal(context.host.querySelector('.permission-user-name')?.textContent, '<img src=x onerror=alert(1)>')
    assert.equal(context.host.querySelector('img'), null)
    const search = context.host.querySelector<HTMLInputElement>('[data-focus-key="user-search"]')!
    search.focus()
    search.value = 'Alice'
    search.dispatchEvent(new context.dom.window.Event('input', { bubbles: true }))
    assert.equal(context.host.querySelectorAll('.permission-user').length, 1)
    assert.equal(context.host.querySelector('.permission-user-email')?.textContent, 'person@example.com')
    assert.equal(context.dom.window.document.activeElement, context.host.querySelector('[data-focus-key="user-search"]'))
  } finally { context.restore() }
})

test('keeps super-admin controls immutable and synchronizes selected role permissions', async () => {
  const context = setup()
  try {
    await context.controller.ready
    const adminRole = context.host.querySelector<HTMLInputElement>('input[data-selection="user-role"][value="super_admin"]')!
    const adminPermission = context.host.querySelector<HTMLInputElement>('input[data-selection="role-permission"][value="content.read"]')!
    assert.equal(adminRole.disabled, true)
    assert.equal(adminPermission.disabled, true)
    const select = context.host.querySelector<HTMLSelectElement>('[data-role-select]')!
    select.value = 'member'
    select.dispatchEvent(new context.dom.window.Event('change', { bubbles: true }))
    const memberPermission = context.host.querySelector<HTMLInputElement>('input[data-selection="role-permission"][value="content.read"]')!
    assert.equal(memberPermission.disabled, false)
    assert.equal(memberPermission.checked, true)
  } finally { context.restore() }
})

test('sends role updates only after an explicit save action and refreshes server state', async () => {
  const context = setup()
  try {
    await context.controller.ready
    context.host.querySelector<HTMLButtonElement>('[data-action="choose-user"][data-user-id="user-1"]')!.click()
    const reviewer = context.host.querySelector<HTMLInputElement>('input[data-selection="user-role"][value="reviewer"]')!
    reviewer.checked = true
    reviewer.dispatchEvent(new context.dom.window.Event('change', { bubbles: true }))
    assert.equal(context.calls.some(call => call.method === 'PATCH'), false)
    context.host.querySelector<HTMLButtonElement>('[data-action="save-user-roles"]')!.click()
    for (let attempt = 0; attempt < 10 && !context.calls.some(call => call.method === 'PATCH'); attempt++) {
      await new Promise(resolve => setTimeout(resolve, 0))
    }
    const patch = context.calls.find(call => call.method === 'PATCH')
    assert.equal(patch?.url, '/api/admin/users/user-1/roles')
    assert.deepEqual(JSON.parse(patch?.body || '{}'), { roles: ['member', 'reviewer'] })
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.ok(context.calls.filter(call => call.method === 'GET').length >= 6)
  } finally { context.restore() }
})

test('dispose clears the host and prevents pending refresh responses from remounting it', async () => {
  const context = setup()
  context.controller.dispose()
  await context.controller.ready
  assert.equal(context.host.children.length, 0)
  context.restore()
})
