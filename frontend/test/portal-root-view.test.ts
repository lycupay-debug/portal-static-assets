/**
 * Regression tests for the native root shell and its route-specific mount points.
 * Dependencies: jsdom and the root view's explicit state/callback contract.
 * Collaboration: App.vue supplies route state and owns each child view lifecycle; this module owns shell DOM only.
 * Boundaries: all dynamic text uses textContent, no Vue/runtime imports, API calls or browser globals.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountPortalRootView, type PortalRootHostName, type PortalRootState } from '../src/views/portalRootView'

function mount(initial: PortalRootState) {
  const dom = new JSDOM('<!doctype html><div id="root"></div>')
  const host = dom.window.document.querySelector<HTMLElement>('#root')!
  let state = initial
  const assigned = new Map<PortalRootHostName, HTMLElement | null>()
  const view = mountPortalRootView(host, {
    getState: () => state,
    navigate: () => undefined,
    scrollToTop: () => undefined,
    setHost: (name, element) => assigned.set(name, element),
  })
  return { dom, host, assigned, view, update(next: PortalRootState) { state = next; view.update(state) } }
}

const base = (route: string, extra: Partial<PortalRootState> = {}): PortalRootState => ({
  route, contentEditorPage: false, accountPage: false, authPage: false, publicContentPage: false,
  legalPage: false, accountSection: 'overview', accountTableMode: false, ...extra,
})

test('public route shell mounts header, route content, footer and global overlays', () => {
  const ctx = mount(base('/'))
  try {
    assert.ok(ctx.host.querySelector('.site-shell > .site-header'))
    assert.ok(ctx.assigned.get('homePageHost')?.isConnected)
    assert.ok(ctx.host.querySelector('.site-shell > .site-footer'))
    assert.ok(ctx.assigned.get('knowledgeTagDialogHost')?.isConnected)
    assert.ok(ctx.assigned.get('teachingResourceDrawerHost')?.isConnected)
    assert.ok(ctx.host.querySelector('.scroll-to-top'))
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('account route selects the correct native host and table layout modifier', () => {
  const ctx = mount(base('/account/content', { accountPage: true, accountSection: 'content', accountTableMode: true }))
  try {
    assert.ok(ctx.assigned.get('accountSidebarHost')?.isConnected)
    assert.ok(ctx.assigned.get('accountTableViewHost')?.isConnected)
    assert.ok(ctx.host.querySelector('.account-content-table-mode'))
    const originalSidebar = ctx.assigned.get('accountSidebarHost')
    const originalTable = ctx.assigned.get('accountTableViewHost')
    ctx.update(base('/account/resources', { accountPage: true, accountSection: 'resources', accountTableMode: false }))
    assert.equal(ctx.assigned.get('accountSidebarHost'), originalSidebar)
    assert.equal(ctx.assigned.get('accountTableViewHost'), originalTable)
    assert.equal(ctx.host.querySelector('.account-content-table-mode'), null)
    assert.equal(ctx.assigned.get('siteHeaderHost'), null)
    assert.equal(ctx.assigned.get('siteFooterHost'), null)
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('account identity sections retain the same native host while switching overview and profile', () => {
  const ctx = mount(base('/account', { accountPage: true, accountSection: 'overview' }))
  try {
    const identity = ctx.assigned.get('accountIdentityHost')
    ctx.update(base('/account/profile', { accountPage: true, accountSection: 'profile' }))
    assert.equal(ctx.assigned.get('accountIdentityHost'), identity)
    assert.equal(identity?.isConnected, true)
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('editor route preserves topbar, editor and settings workspace structure', () => {
  const ctx = mount(base('/content-management/new', { contentEditorPage: true }))
  try {
    assert.ok(ctx.host.querySelector('.content-workspace-page .content-workspace-topbar'))
    assert.ok(ctx.assigned.get('contentEditorHost')?.isConnected)
    assert.ok(ctx.assigned.get('contentSettingsHost')?.isConnected)
    assert.equal(ctx.assigned.get('siteHeaderHost'), null)
    assert.equal(ctx.assigned.get('siteFooterHost'), null)
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('route updates release old hosts before mounting new page and dispose clears all ownership', () => {
  const ctx = mount(base('/'))
  try {
    const oldHome = ctx.assigned.get('homePageHost')
    ctx.update(base('/login', { authPage: true }))
    assert.equal(oldHome?.isConnected, false)
    assert.ok(ctx.assigned.get('authPageHost')?.isConnected)
    assert.ok(ctx.host.querySelector('.route-main'))
    const loginHost = ctx.assigned.get('authPageHost')
    ctx.update(base('/register', { authPage: true }))
    assert.equal(ctx.assigned.get('authPageHost'), loginHost)
    ctx.view.dispose()
    assert.equal(ctx.host.childElementCount, 0)
    for (const value of ctx.assigned.values()) assert.equal(value, null)
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})
