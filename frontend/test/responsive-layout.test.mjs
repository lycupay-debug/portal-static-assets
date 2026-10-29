import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/utils/responsiveLayout.ts', import.meta.url), 'utf8')

test('responsive layout initialization exposes deterministic listener and request cleanup', () => {
  assert.match(source, /export function initializeResponsiveLayout\(\): \(\) => void/)
  assert.match(source, /window\.removeEventListener\('resize', scheduleResponsivePolicy\)/)
  assert.match(source, /window\.removeEventListener\('orientationchange', scheduleResponsivePolicy\)/)
  assert.match(source, /activeRequest \+= 1/)
  assert.match(source, /requestId !== activeRequest/)
})

test('resize debounce clears any defined timer handle and resets its ownership after firing', () => {
  const start = source.indexOf('function scheduleResponsivePolicy()')
  const end = source.indexOf("\n}\n\n/** Register responsive listeners", start) + 2
  const body = source.slice(start, end)
  assert.match(body, /resizeTimer !== undefined/)
  assert.match(body, /resizeTimer = undefined/)
  assert.match(body, /clearTimeout\(resizeTimer\)/)
})

test('responsive policy continues to restrict server-controlled CSS writes', () => {
  assert.match(source, /policy\.version !== 1/)
  assert.match(source, /name\.startsWith\('--layout-'\)/)
  assert.match(source, /typeof value === 'string'/)
  assert.match(source, /credentials: 'same-origin'/)
})
