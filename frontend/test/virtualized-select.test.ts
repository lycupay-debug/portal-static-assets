/**
 * Regression tests for the native searchable/windowed select.
 * Dependencies: Node's built-in test/assert modules and jsdom; no network or service access.
 * Collaboration: protects the public DOM and callback contract used by App.vue and the resource dialog.
 * Boundaries: tests only local DOM behavior, not real-browser layout or keyboard rendering.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountVirtualizedSelect, type VirtualizedSelectOption } from '../src/components/virtualizedSelect'

const options: VirtualizedSelectOption[] = [
  { label: '仅自己可见', value: 'private' },
  { label: '公开', value: 'public' },
  { label: '<img src=x onerror=alert(1)>', value: '' },
]
function setup(value: string | null = 'private') {
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div><button id="outside">outside</button></body></html>', { url: 'https://portal.example.test/' })
  const root = dom.window.document.querySelector<HTMLElement>('#host')!
  const changes: string[] = []
  const select = mountVirtualizedSelect(root, { label: '可见范围', options, value, onChange: next => changes.push(next) })
  return { dom, root, changes, select }
}

test('renders selected label and safely displays labels as text', () => {
  const { dom, root, select } = setup('')
  assert.equal(root.querySelector('.virtualized-select-trigger span')?.textContent, '<img src=x onerror=alert(1)>')
  assert.equal(root.querySelector('img'), null)
  select.update({ value: 'public' })
  assert.equal(root.querySelector('.virtualized-select-trigger span')?.textContent, '公开')
  select.dispose()
  dom.window.close()
})

test('opens, filters options and forwards the selected empty-string value', () => {
  const { dom, root, changes, select } = setup('private')
  root.querySelector<HTMLButtonElement>('.virtualized-select-trigger')!.click()
  assert.equal(root.querySelector('[role="listbox"]')?.getAttribute('aria-label'), '可见范围')
  const search = root.querySelector<HTMLInputElement>('.virtualized-select-search')!
  search.value = '公开'
  search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.deepEqual(Array.from(root.querySelectorAll('[role="option"]')).map(item => item.textContent?.trim()), ['公开'])
  search.value = '<img'
  search.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.equal(root.querySelectorAll('img').length, 0)
  assert.equal(root.querySelectorAll('[role="option"]').length, 1)
  root.querySelector<HTMLButtonElement>('[role="option"]')!.click()
  assert.deepEqual(changes, [''])
  assert.equal(root.querySelector('.virtualized-select-popover'), null)
  assert.equal(root.querySelector('.virtualized-select-trigger span')?.textContent, '<img src=x onerror=alert(1)>')
  select.dispose()
  dom.window.close()
})

test('Escape and outside click close the popover; disabled state blocks opening', () => {
  const { dom, root, select } = setup()
  const document = dom.window.document
  root.querySelector<HTMLButtonElement>('.virtualized-select-trigger')!.click()
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(root.querySelector('.virtualized-select-popover'), null)
  root.querySelector<HTMLButtonElement>('.virtualized-select-trigger')!.click()
  document.querySelector<HTMLButtonElement>('#outside')!.dispatchEvent(new dom.window.MouseEvent('mousedown', { bubbles: true }))
  assert.equal(root.querySelector('.virtualized-select-popover'), null)
  select.update({ disabled: true })
  const trigger = root.querySelector<HTMLButtonElement>('.virtualized-select-trigger')!
  assert.equal(trigger.disabled, true)
  trigger.click()
  assert.equal(root.querySelector('.virtualized-select-popover'), null)
  select.dispose()
  dom.window.close()
})

test('virtualizes long option lists and resets the scroll position when data changes', () => {
  const { dom, root, select } = setup()
  const many = Array.from({ length: 1000 }, (_, index) => ({ label: `Option ${index}`, value: String(index) }))
  select.update({ options: many, value: '0' })
  root.querySelector<HTMLButtonElement>('.virtualized-select-trigger')!.click()
  assert.ok(root.querySelectorAll('[role="option"]').length < 20)
  const viewport = root.querySelector<HTMLElement>('.virtualized-select-viewport')!
  viewport.scrollTop = 720
  viewport.dispatchEvent(new dom.window.Event('scroll'))
  assert.equal(root.querySelector('.virtualized-select-viewport'), viewport, 'scroll should update the virtual window without replacing its viewport')
  assert.ok(root.querySelector('[role="option"]')?.textContent?.includes('Option 18'))
  select.update({ options: [{ label: 'Only option', value: 'one' }], value: 'one' })
  assert.equal(root.querySelectorAll('[role="option"]').length, 1)
  assert.equal(root.querySelector('[role="option"]')?.textContent?.trim(), 'Only option')
  select.dispose()
  dom.window.close()
})

test('dispose removes rendered nodes and prevents later updates from remounting', () => {
  const { dom, root, select } = setup()
  select.dispose()
  select.dispose()
  select.update({ value: 'public' })
  assert.equal(root.children.length, 0)
  dom.window.document.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  assert.equal(root.children.length, 0)
  dom.window.close()
})
