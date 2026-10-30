/**
 * Framework-independent navigation state-machine regression tests.
 * Dependencies: Node test APIs, jsdom and the route resolver.
 * Collaboration: App.vue supplies route/auth/data callbacks; this controller owns browser history sequencing.
 * Boundaries: no Vue runtime, API credentials, network requests or production browser state.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { createPortalNavigationController } from '../src/navigation/portalNavigationController'

function setup(path = '/') {
  const dom = new JSDOM('<!doctype html><main class="account-content"><div class="virtual-table-scroll"></div></main>', { url: `https://portal.example.test${path}` })
  const browser = dom.window as unknown as Window
  const calls: string[] = []
  let route = '/'
  let activeSection = 'overview'
  let activeSidebar = 'overview'
  let sequence = 0
  let protectedPage = false
  let accountPage = false
  let allowed = true
  const controller = createPortalNavigationController({
    browser,
    document: dom.window.document,
    routeSections: { '/account': 'overview', '/account/compliance': 'copyright-compliance' },
    sectionRoutes: { overview: '/account', 'copyright-compliance': '/account/compliance' },
    nextNavigationId: () => ++sequence,
    isNavigationCurrent: id => id === sequence,
    getRoute: () => route,
    setRoute: value => { route = value; protectedPage = value.startsWith('/account') || value.startsWith('/content-management'); accountPage = value.startsWith('/account') },
    isProtectedPage: () => protectedPage,
    isAccountPage: () => accountPage,
    setKnowledgeLocation: value => calls.push(`knowledge:${value.knowledgeCategory}:${value.knowledgeSubcategory}`),
    setAccountSection: value => { activeSection = value; calls.push(`section:${value}`) },
    setActiveSidebarButton: value => { activeSidebar = value },
    ensureRouteAccess: async id => { calls.push(`guard:${id}`); return allowed },
    saveScrollPosition: () => calls.push('save-scroll'),
    restoreScrollPosition: () => calls.push('restore-scroll'),
    closeMenu: () => calls.push('close-menu'),
    clearAuthError: () => calls.push('clear-error'),
    clearAuthNotice: () => calls.push('clear-notice'),
    loadPublishedLegalDocuments: () => calls.push('legal-list'),
    loadContentList: () => calls.push('content-list'),
    loadTeachingResources: () => calls.push('resource-list'),
    loadRouteData: () => calls.push(`route-data:${route}`),
    loadSession: () => calls.push('load-session'),
    observeVirtualTableWidth: () => calls.push('observe-table'),
    onScrollCapture: () => calls.push('scroll'),
    afterRender: async () => { calls.push('after-render') },
  })
  Object.defineProperty(browser, 'scrollTo', { configurable: true, value: () => calls.push('window-scroll-top') })
  return { dom, browser, calls, controller, get route(){return route}, get activeSection(){return activeSection}, get activeSidebar(){return activeSidebar}, setAllowed(value:boolean){allowed=value}, get sequence(){return sequence} }
}

test('start resolves the initial route, guards protected pages and restores route state', async () => {
  const ctx = setup('/account/compliance')
  try {
    await ctx.controller.start()
    assert.equal(ctx.route, '/account/compliance')
    assert.equal(ctx.activeSection, 'copyright-compliance')
    assert.equal(ctx.activeSidebar, 'copyright-compliance')
    assert.ok(ctx.calls.includes('guard:1'))
    assert.ok(ctx.calls.includes('route-data:/account/compliance'))
    assert.ok(ctx.calls.includes('restore-scroll'))
    assert.ok(ctx.calls.includes('observe-table'))
  } finally { ctx.controller.dispose(); ctx.dom.window.close() }
})

test('navigate loads route-specific lists before page data and resets account scroll after render', async () => {
  const ctx = setup('/login')
  const content = ctx.dom.window.document.querySelector<HTMLElement>('.account-content')!
  const table = ctx.dom.window.document.querySelector<HTMLElement>('.virtual-table-scroll')!
  content.scrollTop = 120; table.scrollTop = 90
  try {
    await ctx.controller.navigate('/account/compliance')
    assert.equal(ctx.route, '/account/compliance')
    assert.equal(ctx.activeSection, 'copyright-compliance')
    assert.equal(content.scrollTop, 0)
    assert.equal(table.scrollTop, 0)
    assert.ok(ctx.calls.indexOf('route-data:/account/compliance') > ctx.calls.indexOf('guard:1'))
    assert.ok(ctx.calls.includes('legal-list'))
  } finally { ctx.controller.dispose(); ctx.dom.window.close() }
})

test('denied protected navigation does not load protected route data', async () => {
  const ctx = setup('/login')
  ctx.setAllowed(false)
  try {
    await ctx.controller.navigate('/content-management')
    assert.equal(ctx.route, '/content-management')
    assert.equal(ctx.calls.some(call => call === 'route-data:/content-management'), false)
    assert.equal(ctx.calls.includes('content-list'), false)
  } finally { ctx.controller.dispose(); ctx.dom.window.close() }
})

test('popstate restores the route and dispose removes owned browser listeners', async () => {
  const ctx = setup('/login')
  try {
    await ctx.controller.start()
    ctx.browser.history.pushState({}, '', '/account')
    ctx.browser.dispatchEvent(new ctx.dom.window.PopStateEvent('popstate'))
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.equal(ctx.route, '/account')
    assert.ok(ctx.calls.includes('route-data:/account'))
    const count = ctx.calls.length
    ctx.controller.dispose()
    ctx.browser.history.pushState({}, '', '/account/compliance')
    ctx.browser.dispatchEvent(new ctx.dom.window.PopStateEvent('popstate'))
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.equal(ctx.calls.length, count)
  } finally { ctx.controller.dispose(); ctx.dom.window.close() }
})
