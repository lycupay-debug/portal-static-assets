/**
 * Responsibility: protect the frontend asset-graph contract against accidental eager loading or public CDN use.
 * Dependencies: Node test runner and source configuration files.
 * Collaboration: complements bundle-budget checks by ensuring editor and Markdown features remain route-lazy
 * and that the production static-origin build points at the owned asset service.
 * Boundaries: static source assertions only; generated output is validated by the build scripts.
 */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const viteConfig = readFileSync(new URL('../vite.config.ts', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const articleLoaderSource = readFileSync(new URL('../src/composables/loadPublicArticle.ts', import.meta.url), 'utf8')
const legalLoaderSource = readFileSync(new URL('../src/composables/loadLegalDocument.ts', import.meta.url), 'utf8')
const budgetScript = readFileSync(new URL('../scripts/check-bundle-budget.mjs', import.meta.url), 'utf8')
const packageJson = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8'))

test('editor and Markdown dependencies remain behind lazy feature entry points', () => {
  assert.match(appSource, /defineAsyncComponent\(\(\) => import\(['"]\.\/components\/ContentDocumentEditor\.vue['"]\)\)/)
  assert.match(articleLoaderSource + legalLoaderSource, /import\(['"]\.\.\/utils\/markdown['"]\)/)
  assert.doesNotMatch(viteConfig, /codeSplitting:\s*\{\s*groups:/)
})

test('bundle budgets count initial modulepreloads and preserve existing feature limits', () => {
  assert.match(budgetScript, /modulepreload/)
  assert.match(budgetScript, /65 \* 1024/)
  assert.match(budgetScript, /230 \* 1024/)
  assert.match(budgetScript, /30 \* 1024/)
})

test('static-origin build uses the owned asset service, not a public CDN', () => {
  assert.match(packageJson.scripts['build:static-origin'], /https:\/\/assets\.lycupaynotion\.top\//)
  assert.doesNotMatch(viteConfig, /https?:\/\/(?:cdn\.|unpkg\.|jsdelivr\.|cdnjs\.)/i)
})

test('all lazily initialized App.vue data modules can recover from rejected imports', () => {
  for (const modulePath of [
    './composables/loadContentList',
    './composables/loadTeachingResourceList',
    './composables/useContentManagementActions',
    './composables/loadPublicDirectoryData',
    './composables/loadPublicArticle',
  ]) {
    const escapedPath = modulePath.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    assert.match(appSource, new RegExp(`createRetryableLazyLoader<[\\s\\S]*?import\\(['"]${escapedPath}['"]\\)`), `${modulePath} should use retryable lazy initialization`)
  }
  assert.match(appSource, /function loadPublicContents\([\s\S]*?\.catch\(\(error: unknown\) => \{[\s\S]*?publicContentLoading\.value = false/)
  assert.match(appSource, /function loadPublicTeachingResources\([\s\S]*?\.catch\(\(error: unknown\) => \{[\s\S]*?educationResourceLoading\.value = false/)
})
