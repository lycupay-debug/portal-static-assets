import assert from 'node:assert/strict'
import test from 'node:test'
import { formatFileSize, formatResourceFileSize, resourceFileExtensions } from '../src/utils/resourceFiles.ts'

test('resource detail formatter handles invalid values and unit boundaries', () => {
  assert.equal(formatResourceFileSize(null), '')
  assert.equal(formatResourceFileSize(-1), '')
  assert.equal(formatResourceFileSize(512), '512 B')
  assert.equal(formatResourceFileSize(1024), '1.0 KB')
  assert.equal(formatResourceFileSize(1024 * 1024), '1.0 MB')
})

test('upload formatter preserves current rounded KB and decimal MB presentation', () => {
  assert.equal(formatFileSize(1), '1 KB')
  assert.equal(formatFileSize(1536), '2 KB')
  assert.equal(formatFileSize(1024 * 1024), '1.0 MB')
})

test('supported resource file extensions are explicit and lowercase', () => {
  assert.deepEqual([...resourceFileExtensions].sort(), ['doc', 'docx', 'mp3', 'pdf', 'ppt', 'pptx', 'txt'])
})
