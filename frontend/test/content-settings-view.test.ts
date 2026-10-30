/** Native DOM tests for content metadata controls; no Vue runtime, network, or persisted data. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountContentSettingsView, type ContentSettingsState } from '../src/views/contentSettingsView'

function setup() {
  const dom = new JSDOM('<!doctype html><body><aside id="host"></aside></body>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let trigger: HTMLButtonElement | null = null
  let state: ContentSettingsState = { title: 'Initial title', slug: 'initial-title', summary: 'Summary', visibility: 'private', tags: ['教育', '小学教育'], busy: false, tagDialogOpen: false }
  const view = mountContentSettingsView(host, {
    getState: () => state,
    visibilityOptions: [{ label: '仅自己可见', value: 'private' }, { label: '公开', value: 'public' }],
    taxonomyParentNames: new Set(['教育']),
    setField: (field, value) => { calls.push(`${field}:${value}`); state = { ...state, [field]: value } },
    setVisibility: value => { calls.push(`visibility:${value}`); state = { ...state, visibility: value } },
    openTagPicker: () => calls.push('open-tags'), setTagTrigger: value => { trigger = value },
    removeTag: tag => calls.push(`remove:${tag}`), clearTags: () => calls.push('clear-tags'),
    cancel: () => calls.push('cancel'), save: () => calls.push('save'),
  })
  return { dom, host, calls, view, get trigger(){return trigger}, update(patch:Partial<ContentSettingsState>){state={...state,...patch};view.update(state)} }
}

test('renders fields, tag chips and safely delegates metadata changes', () => {
  const ctx = setup()
  try {
    const title = ctx.host.querySelector<HTMLInputElement>('input[name="contentTitle"]')!
    title.value = 'Updated title'; title.dispatchEvent(new ctx.dom.window.Event('input', { bubbles: true }))
    ctx.host.querySelector<HTMLInputElement>('input[name="contentSlug"]')!.value = 'updated-title'
    ctx.host.querySelector<HTMLInputElement>('input[name="contentSlug"]')!.dispatchEvent(new ctx.dom.window.Event('input', { bubbles: true }))
    ctx.host.querySelector<HTMLTextAreaElement>('textarea[name="contentSummary"]')!.value = 'New summary'
    ctx.host.querySelector<HTMLTextAreaElement>('textarea[name="contentSummary"]')!.dispatchEvent(new ctx.dom.window.Event('input', { bubbles: true }))
    assert.ok(ctx.calls.includes('title:Updated title'))
    assert.ok(ctx.calls.includes('slug:updated-title'))
    assert.ok(ctx.calls.includes('summary:New summary'))
    assert.equal(ctx.host.querySelectorAll('.workspace-tag-selected-chip').length, 1)
    assert.equal(ctx.host.querySelector('.workspace-tag-selected-chip')?.textContent, '小学教育×')
    assert.ok(ctx.trigger)
    ctx.trigger!.click()
    assert.ok(ctx.calls.includes('open-tags'))
    ctx.host.querySelector<HTMLButtonElement>('[aria-label="移除标签 小学教育"]')!.click()
    ctx.host.querySelector<HTMLButtonElement>('.workspace-tag-clear')!.click()
    assert.ok(ctx.calls.includes('remove:小学教育'))
    assert.ok(ctx.calls.includes('clear-tags'))
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('updates disabled, busy label and visibility selection without losing focused input', () => {
  const ctx = setup()
  try {
    const title = ctx.host.querySelector<HTMLInputElement>('input[name="contentTitle"]')!
    title.focus(); title.setSelectionRange(3, 3)
    ctx.update({ title: 'External update', busy: true })
    assert.equal(ctx.dom.window.document.activeElement, title)
    assert.equal(title.value, 'Initial title')
    assert.equal(title.disabled, true)
    assert.equal(ctx.host.querySelector<HTMLButtonElement>('.workspace-save-draft')?.textContent, '正在保存…')
    ctx.update({ busy: false })
    ctx.host.querySelector<HTMLButtonElement>('.virtualized-select-trigger')?.click()
    const publicOption = ctx.host.querySelector<HTMLButtonElement>('[role="option"]')
    assert.ok(publicOption)
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('save, cancel and dispose are explicit and release trigger reference', () => {
  const ctx = setup()
  ctx.host.querySelector<HTMLButtonElement>('.workspace-save-draft')!.click()
  ctx.host.querySelector<HTMLButtonElement>('.workspace-create-actions button')!.click()
  assert.ok(ctx.calls.includes('save'))
  assert.ok(ctx.calls.includes('cancel'))
  assert.ok(ctx.trigger)
  ctx.view.dispose()
  assert.equal(ctx.host.childElementCount, 0)
  assert.equal(ctx.trigger, null)
  ctx.dom.window.close()
})
