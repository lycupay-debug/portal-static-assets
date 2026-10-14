import assert from 'node:assert/strict'
import test from 'node:test'
import { existsSync } from 'node:fs'
import { createVditorOptions, VDITOR_CDN_PATH, VDITOR_TOOLBAR } from '../src/utils/vditorEditor.ts'

test('Vditor is configured for Chinese WYSIWYG Markdown editing with local assets', () => {
  const options = createVditorOptions('# 标题', () => {})
  assert.equal(options.lang, 'zh_CN')
  assert.equal(options.mode, 'wysiwyg')
  assert.equal(options.value, '# 标题')
  assert.equal(options.cdn, VDITOR_CDN_PATH)
  assert.equal(options.cache?.enable, false)
  assert.equal(options.toolbarConfig?.pin, true)
})

test('Vditor toolbar includes native task lists, tables, formatting, history, and mode switching', () => {
  for (const item of ['emoji', 'check', 'table', 'headings', 'bold', 'italic', 'link', 'code', 'undo', 'redo', 'both', 'edit-mode', 'insert-before', 'insert-after']) {
    assert.ok(VDITOR_TOOLBAR.includes(item), `missing Vditor toolbar action: ${item}`)
  }
  assert.ok(!VDITOR_TOOLBAR.includes('upload'), 'file upload must not be enabled without an upload API')
})

test('Vditor changes are returned as Markdown and browser cache is disabled', () => {
  const markdown = '- [ ] 待办事项\n\n| 列一 | 列二 |\n| --- | --- |\n| A | B |'
  let emitted = ''
  const options = createVditorOptions('', value => { emitted = value })
  options.input?.(markdown)
  assert.equal(emitted, markdown)
  assert.equal(options.cache?.enable, false)
  assert.equal(options.upload?.url, '')
})


test('Vditor npm package contains required runtime files for local builds', () => {
  for (const path of [
    'node_modules/vditor/dist/js/lute/lute.min.js',
    'node_modules/vditor/dist/js/highlight.js/highlight.min.js',
    'node_modules/vditor/dist/css/content-theme/light.css',
    'node_modules/vditor/dist/images/emoji/vditor.png',
  ]) {
    assert.ok(existsSync(new URL('../' + path, import.meta.url)), 'missing local Vditor asset: ' + path)
  }
})
