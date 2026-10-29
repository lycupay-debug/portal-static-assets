import assert from 'node:assert/strict'
import test from 'node:test'
import { getResourceUploadFileError, MAX_RESOURCE_UPLOAD_FILE_BYTES, validateResourceUpload } from '../src/utils/resourceUploadValidation.ts'

const file = (name, size = 1024) => ({ name, size })
const input = overrides => ({
  title: '课程资料',
  isBatch: false,
  isEditing: false,
  files: [file('lesson.pdf')],
  url: '',
  ...overrides,
})

test('resource upload validation requires a non-empty title', () => {
  assert.equal(validateResourceUpload(input({ title: '   ' })), '请填写资源名称。')
})

test('batch upload requires at least one file', () => {
  assert.equal(
    validateResourceUpload(input({ isBatch: true, files: [] })),
    '批量上传至少选择一个文件。',
  )
})

test('new resource requires a file or a URL', () => {
  assert.equal(
    validateResourceUpload(input({ files: [] })),
    '请上传文件或粘贴资源 URL 链接。',
  )
})

test('resource URLs must use HTTP or HTTPS', () => {
  assert.equal(
    validateResourceUpload(input({ files: [], url: 'javascript:alert(1)' })),
    '资源 URL 必须以 http:// 或 https:// 开头。',
  )
  assert.equal(validateResourceUpload(input({ files: [], url: 'https://example.com/resource.pdf' })), null)
})

test('unsupported extensions are rejected without case sensitivity', () => {
  assert.match(validateResourceUpload(input({ files: [file('payload.exe')] })), /格式不受支持/)
  assert.equal(validateResourceUpload(input({ files: [file('LESSON.PDF')] })), null)
})

test('file size boundary accepts 25 MiB and rejects larger files', () => {
  assert.equal(validateResourceUpload(input({ files: [file('lesson.pdf', 25 * 1024 * 1024)] })), null)
  assert.match(validateResourceUpload(input({ files: [file('lesson.pdf', 25 * 1024 * 1024 + 1)] })), /超过 25 MB/)
})

test('editing a resource may keep its existing file and URL', () => {
  assert.equal(validateResourceUpload(input({ isEditing: true, files: [], url: '' })), null)
})


test('file selection and paste entry points share extension and size rules', () => {
  assert.equal(getResourceUploadFileError(file('lesson.PDF')), null)
  assert.match(getResourceUploadFileError(file('payload.exe')), /格式不受支持/)
  assert.equal(MAX_RESOURCE_UPLOAD_FILE_BYTES, 25 * 1024 * 1024)
  assert.equal(getResourceUploadFileError(file('lesson.pdf', MAX_RESOURCE_UPLOAD_FILE_BYTES)), null)
  assert.match(getResourceUploadFileError(file('lesson.pdf', MAX_RESOURCE_UPLOAD_FILE_BYTES + 1)), /超过 25 MB/)
})
