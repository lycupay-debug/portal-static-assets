import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const app = readFileSync(new URL('../editor-app/EditorApp.vue', import.meta.url), 'utf8')
const caddy = readFileSync(new URL('../../deploy/freebsd/Caddyfile', import.meta.url), 'utf8')
const deploy = readFileSync(new URL('../../deploy/freebsd/deploy.sh', import.meta.url), 'utf8')
const portal = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const editorComponent = readFileSync(new URL('../src/components/ContentDocumentEditor.vue', import.meta.url), 'utf8')

test('independent editor uses same-origin authenticated Go APIs', () => {
  assert.match(app, /requestJSON<ApiResult<PortalUser>>\('\/api\/auth\/me'\)/)
  assert.match(app, /credentials: 'same-origin'/)
  assert.match(app, /\/api\/content\//)
  assert.match(app, /\/api\/content'/)
  assert.match(editorComponent, /\/api\/uploads/)
  assert.match(app, /window\.location\.replace\('\/login'\)/)
})

test('editor guards unsaved changes and provides explicit keyboard save', () => {
  assert.match(app, /window\.addEventListener\('beforeunload', onBeforeUnload\)/)
  assert.match(app, /event\.key\.toLowerCase\(\) === 's'/)
  assert.match(app, /if \(dirty\.value && !window\.confirm/)
  assert.match(app, /visibility: 'private'/)
})

test('editor route is a separate SPA and deployment includes its build', () => {
  assert.match(caddy, /handle \/editor\//)
  assert.match(caddy, /try_files \{path\} \/editor\/index\.html/)
  assert.match(deploy, /npm run build/)
  assert.match(deploy, /\$repo\/frontend\/dist-editor\/.?/)
  assert.match(portal, /window\.location\.assign\('\/editor\/new'\)/)
  assert.match(portal, /window\.location\.assign\('\/editor\/' \+ encodeURIComponent\(id\)\)/)
})

test('editor exposes approved tags, normalizes parent categories, and blocks unknown legacy tags', () => {
  assert.match(app, /const tagOptions = \[/)
  assert.match(app, /if \(!form\.value\.tags\.length\)/)
  assert.match(app, /const slugIsValid = computed\(\(\) => \/\^\[a-z0-9\]/)
  assert.match(app, /tags: normalizeTags\(form\.value\.tags\), parentCategory: null, childCategory: null/)
  assert.match(app, /const unsupportedTags = form\.value\.tags\.filter\(tag => !tagOptions\.includes\(tag\)\)/)
})
