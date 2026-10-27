import assert from 'node:assert/strict'
import test from 'node:test'
import { directoryPages, directoryTiles } from '../src/config/publicDirectories.ts'

test('public directory page metadata and tile data use the same route keys', () => {
  assert.deepEqual(Object.keys(directoryPages).sort(), Object.keys(directoryTiles).sort())
  assert.ok(Object.keys(directoryPages).includes('/knowledge'))
  assert.ok(Object.keys(directoryPages).includes('/education-resources'))
  for (const [path, page] of Object.entries(directoryPages)) {
    assert.ok(page.title.length > 0, 'directory has a title')
    assert.ok(page.description.length > 0, 'directory has a description')
    assert.equal(directoryTiles[path].length, 4, 'directory has four tiles')
    for (const tile of directoryTiles[path]) {
      assert.ok(tile.title.length > 0)
      assert.ok(tile.tags.length > 0)
    }
  }
})
