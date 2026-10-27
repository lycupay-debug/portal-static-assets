export type EditorImageValidation = { supported: true } | { supported: false; error?: string }

const MAX_EDITOR_IMAGE_BYTES = 8 * 1024 * 1024
const SUPPORTED_EDITOR_IMAGE_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
  'image/avif',
])

export function validateEditorImageFile(file: Pick<File, 'type' | 'size'>): EditorImageValidation {
  if (!file.type.startsWith('image/')) return { supported: false }
  if (file.size > MAX_EDITOR_IMAGE_BYTES) {
    return { supported: false, error: '图片大小不能超过 8 MB。' }
  }
  if (!SUPPORTED_EDITOR_IMAGE_TYPES.has(file.type)) {
    return { supported: false, error: '仅支持 PNG、JPEG、GIF、WebP 或 AVIF 图片。' }
  }
  return { supported: true }
}

export async function uploadEditorImage(
  mime: string,
  data: string,
  request: typeof fetch = fetch,
): Promise<string> {
  const response = await request('/api/uploads', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ mime, data }),
  })
  const result = await response.json() as { url?: string; error?: string }
  if (!response.ok || !result.url) throw new Error(result.error || '图片上传失败。')
  return result.url
}
