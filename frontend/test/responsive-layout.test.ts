import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { JSDOM } from 'jsdom'
import { initializeResponsiveLayout } from '../src/utils/responsiveLayout'

const originalWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
const originalFetch = Object.getOwnPropertyDescriptor(globalThis, 'fetch')
let activeDOM: JSDOM | undefined

function setup(fetcher: typeof fetch) {
  activeDOM = new JSDOM('<!doctype html><html><head></head><body></body></html>', { url: 'https://portal.example/' })
  Object.defineProperty(globalThis, 'window', { configurable: true, value: activeDOM.window })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: activeDOM.window.document })
  Object.defineProperty(globalThis, 'fetch', { configurable: true, value: fetcher })
  return activeDOM
}

afterEach(() => {
  activeDOM?.window.close()
  activeDOM = undefined
  for (const [key, descriptor] of [['window', originalWindow], ['document', originalDocument], ['fetch', originalFetch]] as const) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor)
    else Reflect.deleteProperty(globalThis, key)
  }
})

test('responsive layout dispose removes resize/orientation listeners and cancels pending debounce', async () => {
  let fetchCalls = 0
  const dom = setup(async () => {
    fetchCalls += 1
    return new Response(JSON.stringify({ version: 1, profile: 'desktop', cssVariables: {} }), { status: 200 })
  })
  const dispose = initializeResponsiveLayout()
  await new Promise(resolve => setTimeout(resolve, 0))
  assert.equal(fetchCalls, 1)

  dom.window.dispatchEvent(new dom.window.Event('resize'))
  dom.window.dispatchEvent(new dom.window.Event('orientationchange'))
  dispose()
  await new Promise(resolve => setTimeout(resolve, 200))
  assert.equal(fetchCalls, 1, 'pending debounce must not issue a request after disposal')

  dom.window.dispatchEvent(new dom.window.Event('resize'))
  dom.window.dispatchEvent(new dom.window.Event('orientationchange'))
  await new Promise(resolve => setTimeout(resolve, 200))
  assert.equal(fetchCalls, 1, 'disposed listeners must not schedule new requests')
})

test('responsive layout dispose invalidates an in-flight response before CSS mutation', async () => {
  let resolveResponse!: (value: Response) => void
  const dom = setup(() => new Promise<Response>(resolve => { resolveResponse = resolve }))
  const dispose = initializeResponsiveLayout()
  await new Promise(resolve => setTimeout(resolve, 0))
  dispose()
  resolveResponse(new Response(JSON.stringify({
    version: 1,
    profile: 'stale-response',
    cssVariables: { '--layout-gutter-x': '99vw' },
  }), { status: 200 }))
  await new Promise(resolve => setTimeout(resolve, 0))
  assert.equal(dom.window.document.documentElement.dataset.layoutProfile, undefined)
  assert.equal(dom.window.document.documentElement.style.getPropertyValue('--layout-gutter-x'), '')
})
