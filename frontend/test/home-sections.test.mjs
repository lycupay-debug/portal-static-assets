import assert from 'node:assert/strict'
import test from 'node:test'
import { homeSections } from '../src/config/homeSections.ts'

test('home page sections have unique ids, ordered numbers and searchable metadata', () => {
  assert.equal(homeSections.length, 4)
  assert.equal(new Set(homeSections.map(section => section.id)).size, homeSections.length)
  assert.deepEqual(homeSections.map(section => section.number), ['01', '02', '03', '04'])
  for (const section of homeSections) {
    assert.ok(section.title.length > 0)
    assert.ok(section.subtitle.length > 0)
    assert.ok(section.description.length > 0)
    assert.ok(section.tags.length > 0)
  }
})
