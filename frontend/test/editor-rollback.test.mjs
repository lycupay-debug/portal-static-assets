import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const caddy = readFileSync(new URL('../../deploy/freebsd/Caddyfile', import.meta.url), 'utf8')
const deploy = readFileSync(new URL('../../deploy/freebsd/deploy.sh', import.meta.url), 'utf8')
const portal = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const editorComponent = readFileSync(new URL('../src/components/ContentDocumentEditor.vue', import.meta.url), 'utf8')
const packageJson = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8'))

test('content management uses the integrated Tiptap editor without redirecting to a separate SPA', () => {
  assert.match(editorComponent, /from '@tiptap\/vue-3'/)
  assert.match(editorComponent, /EditorContent/)
  assert.match(portal, /window\.location\.assign\('\/content-management\/new'\)/)
  assert.match(portal, /window\.location\.assign\('\/content-management\/' \+ encodeURIComponent\(id\)\)/)
  assert.doesNotMatch(portal, /window\.location\.replace\('\/editor\//)
})

test('legacy editor URLs redirect into the portal and standalone assets are disabled', () => {
  assert.match(caddy, /redir https:\/\/\{http\.request\.host\}\/content-management 308/)
  assert.match(caddy, /handle_path \/editor\/\*/)
  assert.match(caddy, /respond 410/)
  assert.doesNotMatch(caddy, /try_files \{path\} \/editor\/index\.html/)
})

test('production build and deploy no longer publish the standalone editor SPA', () => {
  assert.doesNotMatch(packageJson.scripts.build, /build:editor/)
  assert.equal(packageJson.scripts['build:editor'], undefined)
  assert.doesNotMatch(deploy, /dist-editor/)
  assert.doesNotMatch(deploy, /test-editor-static-rollback/)
})
