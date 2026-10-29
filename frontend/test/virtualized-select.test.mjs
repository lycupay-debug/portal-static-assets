/**
 * Responsibility: exercise VirtualizedSelect's user-visible behavior in a DOM-backed Vue mount.
 * Dependencies: the existing jsdom, Vite SFC loader, and Vue runtime; no new test framework is added.
 * Collaboration: the harness supplies caller-owned options/model state and records update:modelValue events.
 * Limits: jsdom verifies component DOM/events but does not replace cross-browser visual or assistive-technology testing.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from '@vue/compiler-sfc'
import { transform } from 'esbuild'
import { readFileSync, writeFileSync, unlinkSync } from 'node:fs'
import { resolve } from 'node:path'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' })
for (const key of ['window', 'document', 'Node', 'Element', 'HTMLElement', 'SVGElement', 'MouseEvent', 'KeyboardEvent']) {
  globalThis[key] = key === 'window' ? dom.window : key === 'document' ? dom.window.document : dom.window[key]
}

const componentPath = new URL('../src/components/VirtualizedSelect.vue', import.meta.url)
const componentSource = readFileSync(componentPath, 'utf8')
const { descriptor, errors } = parse(componentSource, { filename: 'VirtualizedSelect.vue' })
if (errors.length) throw errors[0]
const compiled = compileScript(descriptor, { id: 'virtualized-select-mounted-test', inlineTemplate: true })
const compiledPath = resolve(new URL('..', import.meta.url).pathname, `.virtualized-select-test-${process.pid}.mjs`)
const transformed = await transform(compiled.content, { sourcefile: compiledPath, loader: 'ts', format: 'esm' })
writeFileSync(compiledPath, transformed.code)
let VirtualizedSelect
try { ({ default: VirtualizedSelect } = await import(`file://${compiledPath}?test=${Date.now()}`)) }
finally { unlinkSync(compiledPath) }
const { createApp, defineComponent, h, nextTick, reactive } = await import('vue')

function mountSelect(overrides = {}) {
  const state = reactive({
    modelValue: 'a',
    options: [{ label: 'Alpha', value: 'a' }, { label: 'Beta', value: 'b' }, { label: 'Gamma', value: 'c' }],
    disabled: false,
    ...overrides,
  })
  const updates = []
  const host = document.createElement('div')
  document.body.append(host)
  const app = createApp(defineComponent({
    setup() {
      return () => h(VirtualizedSelect, {
        modelValue: state.modelValue,
        options: state.options,
        disabled: state.disabled,
        label: 'Test options',
        placeholder: 'Choose one',
        'onUpdate:modelValue': value => { updates.push(value); state.modelValue = value },
      })
    },
  }))
  app.mount(host)
  return { state, updates, host, app, trigger: () => host.querySelector('.virtualized-select-trigger'), popover: () => host.querySelector('.virtualized-select-popover'), cleanup() { app.unmount(); host.remove() } }
}

 test('opens, filters options, emits selection, and resets the query', async () => {
  const view = mountSelect()
  try {
    assert.equal(view.trigger().textContent.trim().startsWith('Alpha'), true)
    view.trigger().click()
    await nextTick()
    assert.ok(view.popover())
    const search = view.host.querySelector('input[name="optionSearch"]')
    search.value = 'bet'
    search.dispatchEvent(new window.Event('input', { bubbles: true }))
    await nextTick()
    assert.deepEqual([...view.host.querySelectorAll('[role="option"] span')].map(el => el.textContent), ['Beta'])
    view.host.querySelector('[role="option"]').click()
    await nextTick()
    assert.deepEqual(view.updates, ['b'])
    assert.equal(view.state.modelValue, 'b')
    assert.equal(view.popover(), null)
    view.trigger().click()
    await nextTick()
    assert.equal(view.host.querySelector('input[name="optionSearch"]').value, '')
  } finally { view.cleanup() }
})

test('shows the empty-results message and closes on Escape from the search input', async () => {
  const view = mountSelect()
  try {
    view.trigger().click()
    await nextTick()
    const search = view.host.querySelector('input[name="optionSearch"]')
    search.value = 'no-match'
    search.dispatchEvent(new window.Event('input', { bubbles: true }))
    await nextTick()
    assert.equal(view.host.querySelector('.virtualized-select-empty')?.textContent, '没有匹配的选项')
    search.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    assert.equal(view.popover(), null)
  } finally { view.cleanup() }
})


test('resets the virtualized viewport when filtering narrows a scrolled option list', async () => {
  const options = Array.from({ length: 80 }, (_, index) => ({ label: `Option ${index}`, value: String(index) }))
  options[79] = { label: 'Unique target', value: 'target' }
  const view = mountSelect({ options, modelValue: '0' })
  try {
    view.trigger().click()
    await nextTick()
    const viewport = view.host.querySelector('.virtualized-select-viewport')
    viewport.scrollTop = 1800
    viewport.dispatchEvent(new window.Event('scroll', { bubbles: true }))
    await nextTick()
    assert.ok(view.host.querySelectorAll('[role="option"]').length > 0)
    const search = view.host.querySelector('input[name="optionSearch"]')
    search.value = 'Unique target'
    search.dispatchEvent(new window.Event('input', { bubbles: true }))
    await nextTick()
    assert.equal(viewport.scrollTop, 0)
    assert.deepEqual([...view.host.querySelectorAll('[role="option"] span')].map(el => el.textContent), ['Unique target'])
  } finally { view.cleanup() }
})

test('closes on outside pointer input and does not open while disabled', async () => {
  const view = mountSelect()
  try {
    view.trigger().click()
    await nextTick()
    document.body.dispatchEvent(new window.MouseEvent('mousedown', { bubbles: true }))
    await nextTick()
    assert.equal(view.popover(), null)
    view.state.disabled = true
    await nextTick()
    assert.equal(view.trigger().disabled, true)
    view.trigger().click()
    await nextTick()
    assert.equal(view.popover(), null)
  } finally { view.cleanup() }
})

test('reacts to option updates and cleans up document listeners when unmounted', async () => {
  const view = mountSelect()
  try {
    view.state.options = [{ label: 'Replacement', value: 'a' }]
    await nextTick()
    assert.equal(view.trigger().textContent.trim().startsWith('Replacement'), true)
    view.trigger().click()
    await nextTick()
    assert.equal(view.popover() !== null, true)
    view.cleanup()
    document.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    document.body.dispatchEvent(new window.MouseEvent('mousedown', { bubbles: true }))
    assert.equal(document.querySelector('.virtualized-select-popover'), null)
  } finally { if (view.host.isConnected) view.cleanup() }
})

 test('retains source-level contract for empty labels and caller-owned v-model', async () => {
  const { readFileSync } = await import('node:fs')
  const source = readFileSync(new URL('../src/components/VirtualizedSelect.vue', import.meta.url), 'utf8')
  assert.match(source, /selected\?\.label\s*\?\?\s*placeholder/)
  assert.match(source, /modelValue: string \| null/)
  assert.match(source, /emit\('update:modelValue', value\)/)
  assert.match(source, /@keydown\.stop="onSearchKeys"/)
  assert.match(source, /watch\(filtered,/)
})

test('test harness releases jsdom resources', () => { dom.window.close() })
