import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useScrollRestoration } from '../src/composables/useScrollRestoration.ts'

function installBrowserMocks() {
  const values = new Map()
  const elements = new Map([
    ['.account-content', { scrollTop: 12 }],
    ['.account-sidebar', { scrollTop: 34 }],
    ['.virtual-table-scroll', { scrollTop: 56 }],
  ])
  const original = {
    window: globalThis.window,
    document: globalThis.document,
    sessionStorage: globalThis.sessionStorage,
  }
  const scrollCalls = []
  globalThis.window = { scrollY: 78, scrollTo: value => scrollCalls.push(value) }
  globalThis.document = { querySelector: selector => elements.get(selector) || null }
  globalThis.sessionStorage = {
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  }
  return {
    values, elements, scrollCalls,
    restore() {
      for (const key of Object.keys(original)) {
        if (original[key] === undefined) delete globalThis[key]
        else globalThis[key] = original[key]
      }
    },
  }
}

test('scroll restoration stores and restores per-route window and nested scroll offsets', async () => {
  const browser = installBrowserMocks()
  try {
    const route = ref('/account')
    const scroll = useScrollRestoration(route)
    scroll.saveScrollPosition()
    const saved = JSON.parse(browser.values.get('portal:scroll:/account'))
    assert.deepEqual(saved, { windowY: 78, accountContent: 12, accountSidebar: 34, table: 56 })
    browser.elements.get('.account-content').scrollTop = 0
    await scroll.restoreScrollPosition()
    assert.deepEqual(browser.scrollCalls, [{ top: 78, behavior: 'auto' }])
    assert.equal(browser.elements.get('.account-content').scrollTop, 12)
    assert.equal(browser.elements.get('.account-sidebar').scrollTop, 34)
    assert.equal(browser.elements.get('.virtual-table-scroll').scrollTop, 56)
  } finally { browser.restore() }
})

test('scroll restoration ignores malformed saved state without throwing', async () => {
  const browser = installBrowserMocks()
  try {
    browser.values.set('portal:scroll:/knowledge', '{invalid')
    await useScrollRestoration(ref('/knowledge')).restoreScrollPosition()
    assert.deepEqual(browser.scrollCalls, [])
  } finally { browser.restore() }
})
