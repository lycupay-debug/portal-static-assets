/**
 * Regression tests for the complete framework-free content editor workspace.
 * Dependencies: Node test APIs, jsdom, Tiptap core and the shared portal extension configuration.
 * Collaboration: protects Markdown synchronization, outline, slash tools, menus, read-only state and disposal.
 * Boundaries: synthetic in-memory documents only; no credentials, uploads, or persistent content.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountNativeContentEditorView } from '../src/components/nativeContentEditorView'

function setup(markdown = '# First heading\n\nInitial paragraph') {
  const dom = new JSDOM('<!doctype html><html><body><div id="host"></div></body></html>', { url: 'https://portal.example.test/', pretendToBeVisual: true })
  const win = dom.window
  const globals = ['window','document','navigator','HTMLElement','Element','Node','DOMParser','MutationObserver','getComputedStyle','requestAnimationFrame','cancelAnimationFrame'] as const
  const previous = new Map<string, PropertyDescriptor | undefined>()
  for (const key of globals) {
    previous.set(key,Object.getOwnPropertyDescriptor(globalThis,key))
    Object.defineProperty(globalThis,key,{configurable:true,writable:true,value:key==='window'?win:key==='document'?win.document:key==='navigator'?win.navigator:key==='HTMLElement'?win.HTMLElement:key==='Element'?win.Element:key==='Node'?win.Node:key==='DOMParser'?win.DOMParser:key==='MutationObserver'?win.MutationObserver:key==='getComputedStyle'?win.getComputedStyle.bind(win):key==='requestAnimationFrame'?win.requestAnimationFrame.bind(win):win.cancelAnimationFrame.bind(win)})
  }
  const host=win.document.querySelector<HTMLElement>('#host')!
  const updates:string[]=[]
  const view=mountNativeContentEditorView({host,markdown,onMarkdownChange:value=>updates.push(value)})
  return {dom,host,view,updates,restore(){view.dispose();dom.window.close();for(const key of globals){const descriptor=previous.get(key);if(descriptor)Object.defineProperty(globalThis,key,descriptor);else Reflect.deleteProperty(globalThis,key)}}}
}

test('native workspace mounts core editor, heading outline and existing Markdown schema',()=>{
  const ctx=setup()
  try {
    assert.ok(ctx.host.querySelector('.ProseMirror'))
    assert.equal(ctx.view.editor.getMarkdown(),'# First heading\n\nInitial paragraph')
    assert.equal(ctx.host.querySelectorAll('.content-outline-list button').length,1)
    assert.match(ctx.host.querySelector('.content-outline-list')?.textContent||'',/First heading/)
  } finally {ctx.restore()}
})

test('native workspace synchronizes Markdown, supports programmatic load and read-only transitions',()=>{
  const ctx=setup()
  try {
    ctx.view.editor.commands.setContent('# Updated heading\n\nUpdated body',{contentType:'markdown'})
    assert.equal(ctx.updates.at(-1),'# Updated heading\n\nUpdated body')
    ctx.view.setMarkdown('# Loaded externally')
    assert.equal(ctx.view.editor.getMarkdown().trimEnd(),'# Loaded externally')
    assert.equal(ctx.updates.length,1)
    ctx.view.setEditable(false)
    assert.equal(ctx.view.editor.isEditable,false)
    ctx.view.setEditable(true)
    assert.equal(ctx.view.editor.isEditable,true)
  } finally {ctx.restore()}
})

test('slash menu filters existing editor tools and executes the selected block action',()=>{
  const ctx=setup('')
  try {
    ctx.view.editor.commands.insertContent('/')
    const menu=ctx.host.querySelector<HTMLElement>('.content-editor-slash-menu')!
    assert.equal(menu.hidden,false)
    const search=menu.querySelector<HTMLInputElement>('input')!
    search.value='一级标题'
    search.dispatchEvent(new ctx.dom.window.Event('input',{bubbles:true}))
    assert.equal(menu.querySelectorAll('button[role="menuitem"]').length,1)
    menu.querySelector<HTMLButtonElement>('button[role="menuitem"]')!.dispatchEvent(new ctx.dom.window.MouseEvent('mousedown',{bubbles:true,cancelable:true}))
    assert.equal(ctx.view.editor.state.doc.firstChild?.type.name,'heading')
    assert.equal(menu.hidden,true)
  } finally {ctx.restore()}
})

test('native context menu exposes formatting controls and dispose destroys the editor host',()=>{
  const ctx=setup()
  try {
    const canvas=ctx.host.querySelector<HTMLElement>('.content-editor-canvas')!
    canvas.dispatchEvent(new ctx.dom.window.MouseEvent('contextmenu',{bubbles:true,cancelable:true,clientX:120,clientY:160}))
    const menu=ctx.host.querySelector<HTMLElement>('.content-editor-context-menu')!
    assert.equal(menu.hidden,false)
    assert.ok(menu.querySelector('[data-action="bold"]'))
    assert.ok(menu.querySelector('[data-action="background"]'))
  } finally {ctx.restore()}
  assert.equal(ctx.host.childElementCount,0)
})
