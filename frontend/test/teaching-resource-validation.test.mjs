/**
 * Responsibility: exercise resource upload validation and pasted-file filtering at boundary values.
 * Dependencies: Node test runner and the pure validation utility; no browser, network, or production files.
 * Collaboration: App.vue loads this utility on demand for resource-dialog interactions.
 * Boundaries: client-side checks improve feedback but do not replace Go API validation or authorization.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { filterPastedResourceFiles, validateResourceFile, validateResourceForm } from '../src/utils/teachingResourceValidation.ts'

const file = (name, size = 1) => ({ name, size })

test('resource form validation requires title and valid source for new resources', () => {
  assert.equal(validateResourceForm({ title: ' ', batchMode: false, editing: false, files: [], url: '' }), '请填写资源名称。')
  assert.equal(validateResourceForm({ title: 'Unit 1', batchMode: false, editing: false, files: [], url: '' }), '请上传文件或粘贴资源 URL 链接。')
  assert.equal(validateResourceForm({ title: 'Unit 1', batchMode: false, editing: false, files: [], url: 'javascript:alert(1)' }), '资源 URL 必须以 http:// 或 https:// 开头。')
  assert.equal(validateResourceForm({ title: 'Unit 1', batchMode: false, editing: false, files: [], url: 'https://example.com' }), null)
})

test('batch mode requires files and validates extension and the exact 25 MB boundary', () => {
  const base = { title: 'Resources', batchMode: true, editing: false, url: '' }
  assert.equal(validateResourceForm({ ...base, files: [] }), '批量上传至少选择一个文件。')
  assert.match(validateResourceForm({ ...base, files: [file('run.exe')] }), /run\.exe.*格式不受支持/)
  assert.equal(validateResourceFile(file('large.pdf', 25 * 1024 * 1024 + 1)), 'size')
  assert.equal(validateResourceForm({ ...base, files: [file('limit.pdf', 25 * 1024 * 1024)] }), null)
})

test('existing resource can be updated without a new file or URL', () => {
  assert.equal(validateResourceForm({ title: 'Existing', batchMode: false, editing: true, files: [], url: '' }), null)
})

test('pasted files retain accepted files and report each rejected file', () => {
  const result = filterPastedResourceFiles([file('handout.pdf'), file('unsafe.exe'), file('oversize.txt', 25 * 1024 * 1024 + 1)])
  assert.deepEqual(result.accepted.map(item => item.name), ['handout.pdf'])
  assert.deepEqual(result.errors, ['unsafe.exe：格式不受支持', 'oversize.txt：超过 25 MB'])
})
