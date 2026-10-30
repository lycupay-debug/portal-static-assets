/** Native DOM tests for the accessible knowledge-tag picker; no Vue runtime or API requests. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountKnowledgeTagDialogView, type KnowledgeTagDialogState } from '../src/views/knowledgeTagDialogView'

const categories = [{ value: '教育', children: ['小学', '初中'] }, { value: '技术', children: ['Go', 'TypeScript'] }]
function setup() {
  const dom = new JSDOM('<!doctype html><body><div id="host"></div></body>')
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  let dialog: HTMLElement | null = null
  let state: KnowledgeTagDialogState = { open: true, busy: false, categories, expandedGroups: ['教育'], selectedTags: ['小学'], draftCount: 1 }
  const view = mountKnowledgeTagDialogView(host, { getState: () => state, close: apply => calls.push(`close:${apply}`), toggleGroup: value => calls.push(`group:${value}`), toggleTag: (tag, checked) => calls.push(`tag:${tag}:${checked}`), clear: () => calls.push('clear'), setDialogElement: value => { dialog = value } })
  return { dom, host, calls, view, get dialog(){return dialog}, update(patch:Partial<KnowledgeTagDialogState>){state={...state,...patch};view.update(state)} }
}

test('renders accessible taxonomy groups and restores focused checkbox after update', () => {
  const ctx = setup()
  try {
    assert.equal(ctx.dialog?.getAttribute('role'), 'dialog')
    assert.equal(ctx.host.querySelectorAll('.knowledge-tag-tree-group').length, 2)
    const input = ctx.host.querySelector<HTMLInputElement>('input[type=checkbox]')!
    input.focus()
    ctx.update({ selectedTags: ['小学'], draftCount: 1 })
    assert.equal(ctx.dom.window.document.activeElement?.getAttribute('data-focus-key'), 'tag:小学')
    assert.equal(ctx.host.querySelector('.knowledge-tag-dialog-footer strong')?.textContent, '1')
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('delegates selection, group, clear, confirm and escape actions', () => {
  const ctx = setup()
  try {
    ctx.host.querySelector<HTMLButtonElement>('[data-focus-key="group:技术"]')!.click()
    ctx.host.querySelector<HTMLInputElement>('input[type=checkbox]')!.click()
    ctx.host.querySelector<HTMLButtonElement>('[data-focus-key="clear"]')!.click()
    ctx.host.querySelector<HTMLButtonElement>('[data-focus-key="confirm"]')!.click()
    assert.ok(ctx.calls.includes('group:技术'))
    assert.ok(ctx.calls.includes('tag:小学:false'))
    assert.ok(ctx.calls.includes('clear'))
    assert.ok(ctx.calls.includes('close:true'))
    ctx.dialog?.dispatchEvent(new ctx.dom.window.KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    assert.ok(ctx.calls.includes('close:false'))
  } finally { ctx.view.dispose(); ctx.dom.window.close() }
})

test('busy state disables selection and dispose removes DOM references', () => {
  const ctx = setup()
  ctx.update({ busy: true })
  assert.equal(ctx.host.querySelector<HTMLInputElement>('input[type=checkbox]')?.disabled, true)
  ctx.view.dispose()
  assert.equal(ctx.host.childElementCount, 0)
  assert.equal(ctx.dialog, null)
  ctx.dom.window.close()
})
