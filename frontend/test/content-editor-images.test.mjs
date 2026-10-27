import assert from 'node:assert/strict'
import test from 'node:test'
import { uploadEditorImage, validateEditorImageFile } from '../src/utils/contentEditorImages.ts'

test('image upload validation rejects oversized and unsupported image files', () => {
  assert.deepEqual(validateEditorImageFile({ type: 'image/png', size: 8 * 1024 * 1024 + 1 }), {
    supported: false,
    error: '图片大小不能超过 8 MB。',
  })
  assert.deepEqual(validateEditorImageFile({ type: 'image/svg+xml', size: 100 }), {
    supported: false,
    error: '仅支持 PNG、JPEG、GIF、WebP 或 AVIF 图片。',
  })
  assert.deepEqual(validateEditorImageFile({ type: 'text/plain', size: 100 }), { supported: false })
})

test('image upload validation accepts supported MIME types at the size boundary', () => {
  for (const type of ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/avif']) {
    assert.deepEqual(validateEditorImageFile({ type, size: 8 * 1024 * 1024 }), { supported: true })
  }
})

test('uploads image data with same-origin credentials and returns the stored URL', async () => {
  let requestUrl
  let requestOptions
  const url = await uploadEditorImage('image/png', 'YWJj', async (input, init) => {
    requestUrl = input
    requestOptions = init
    return new Response(JSON.stringify({ url: '/api/uploads/image-1' }), { status: 201 })
  })
  assert.equal(url, '/api/uploads/image-1')
  assert.equal(requestUrl, '/api/uploads')
  assert.equal(requestOptions.credentials, 'same-origin')
  assert.equal(requestOptions.method, 'POST')
  assert.deepEqual(JSON.parse(requestOptions.body), { mime: 'image/png', data: 'YWJj' })
})

test('upload errors preserve server messages and reject missing URLs', async () => {
  await assert.rejects(
    uploadEditorImage('image/png', 'YWJj', async () => new Response(JSON.stringify({ error: '上传被拒绝' }), { status: 413 })),
    /上传被拒绝/,
  )
  await assert.rejects(
    uploadEditorImage('image/png', 'YWJj', async () => new Response(JSON.stringify({}), { status: 200 })),
    /图片上传失败/,
  )
})
