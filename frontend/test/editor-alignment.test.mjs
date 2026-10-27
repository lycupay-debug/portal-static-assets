import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'

const app = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
const themeParts = await Promise.all([
  'theme-foundation.css',
  'theme-editor-workspace.css',
  'theme-account-layout.css',
  'theme-public-content.css',
  'theme-component-normalization.css',
  'theme-typography.css',
].map(name => readFile(new URL(`../src/${name}`, import.meta.url), 'utf8')))
const theme = themeParts.join('\n')
const appCss = await readFile(new URL('../src/app-foundation.css', import.meta.url), 'utf8')
const editorTests = await readFile(new URL('./document-editor.test.mjs', import.meta.url), 'utf8')

test('task-list layout aligns checkbox to the first text line without a manual top offset', () => {
  assert.doesNotMatch(appCss, /ul\[data-type=taskList\]>li>label\{margin-top:\s*5px\}/)
  assert.match(theme, /ul\[data-type="taskList"\]\s*>\s*li\s*\{[^}]*display:\s*grid/s)
  assert.match(theme, /grid-template-columns:\s*1\.25em minmax\(0,\s*1fr\)/)
  assert.match(theme, /ul\[data-type="taskList"\]\s*>\s*li\s*>\s*label\s*\{[^}]*align-items:\s*center/s)
  assert.match(theme, /ul\[data-type="taskList"\]\s*>\s*li\s*>\s*label input\[type="checkbox"\]\s*\{[^}]*margin:\s*0/s)
})

test('alignment rules preserve first-line anchoring for multiline and nested list content', () => {
  assert.match(theme, /ul\[data-type="taskList"\]\s*>\s*li\s*>\s*div\s*>\s*p\s*\{[^}]*min-height:\s*calc\(var\(--editor-line-height\) \* 1em\)/s)
  assert.match(theme, /ul\[data-type="taskList"\]\s*>\s*li\s*>\s*div\s*>\s*p:last-child\s*\{[^}]*margin-block-end:\s*0/s)
  assert.match(theme, /@media \(max-width:\s*680px\)/)
  assert.match(editorTests, /nested task lists preserve both checkbox items and ordinary nested bullets/)
})

test('editor block rhythm covers regular lists and common block tools', () => {
  assert.match(theme, /ul:not\(\[data-type="taskList"\]\), ol/)
  assert.match(theme, /content-editor-document blockquote/)
  assert.match(theme, /content-editor-document > \[data-callout=\"true\"\]/)
  assert.match(theme, /figure, img, \[data-callout=\"true\"\]/)
  assert.match(theme, /content-editor-document pre/)
  assert.match(theme, /content-editor-document table :is\(th, td\)/)
})

test('App component CSS lives in a dedicated stylesheet in the original style order', () => {
  assert.match(app, /<style src=\"\.\/app\.css\"><\/style>/)
  assert.match(app, /<style src=\"\.\/theme\.css\"><\/style>/)
  assert.match(appCss, /\.site-shell\s*\{/ )
})
