/**
 * Responsibility: guard the lazy-component boundary for the teaching-resource editor dialog.
 * Dependencies: Node test runner and source files; no network, browser session, or production environment.
 * Collaboration: App.vue owns resource-list refresh and shared busy state; the dialog owns form/upload interaction.
 * Boundaries: these are architecture-contract tests, not substitutes for browser clipboard or live API integration tests.
 */
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const appSource = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
const dialogSource = await readFile(new URL('../src/components/TeachingResourceEditorDialog.vue', import.meta.url), 'utf8')

test('resource editor dialog is loaded asynchronously and only mounted while open', () => {
  assert.match(appSource, /defineAsyncComponent\(\(\) => import\('\.\/components\/TeachingResourceEditorDialog\.vue'\)\)/)
  assert.match(appSource, /<TeachingResourceEditorDialog\s+v-if="resourceDialogOpen"/)
  assert.match(appSource, /@update:busy="resourceBusy = \$event"/)
  assert.match(appSource, /@saved="onResourceDialogSaved"/)
})

test('resource dialog owns the form, paste filtering, progress and completion UI', () => {
  assert.match(dialogSource, /@submit\.prevent="createTeachingResource"/)
  assert.match(dialogSource, /@paste="handleResourcePaste"/)
  assert.match(dialogSource, /xhr\.upload\.onprogress/)
  assert.match(dialogSource, /emit\('update:busy', false\)[\s\S]*if \(shouldRefresh\) emit\('saved'\)/)
  assert.match(dialogSource, /emit\('completed'\)[\s\S]*emit\('close'\)/)
})

test('upload API stays same-origin and validation remains on-demand', () => {
  assert.match(dialogSource, /import\('\.\.\/utils\/teachingResourceValidation'\)/)
  assert.match(dialogSource, /`\/api\/resources\/\$\{encodeURIComponent\(editingResourceId\.value\)\}`/)
  assert.match(dialogSource, /: '\/api\/resources'/)
  assert.doesNotMatch(dialogSource, /https?:\/\/(?:cdn\.|unpkg\.com|cdn\.jsdelivr\.net)/i)
})
