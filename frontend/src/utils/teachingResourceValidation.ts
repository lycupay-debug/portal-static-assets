import { resourceFileExtensions } from './resourceFiles'

const MAX_RESOURCE_FILE_BYTES = 25 * 1024 * 1024

type ResourceFileError = 'format' | 'size' | null

/** Validate the allowed extension and maximum upload size for one local file. */
export function validateResourceFile(file: File): ResourceFileError {
  const extension = file.name.split('.').pop()?.toLowerCase() || ''
  if (!resourceFileExtensions.has(extension)) return 'format'
  if (file.size > MAX_RESOURCE_FILE_BYTES) return 'size'
  return null
}

interface ResourceFormValidationInput {
  title: string
  batchMode: boolean
  editing: boolean
  files: File[]
  url: string
}

/** Validate the resource form before the existing upload/API workflow starts. */
export function validateResourceForm(input: ResourceFormValidationInput): string | null {
  if (!input.title.trim()) return '请填写资源名称。'
  if (input.batchMode && !input.editing && !input.files.length) return '批量上传至少选择一个文件。'
  if (!input.files.length && !input.url.trim() && !input.editing) return '请上传文件或粘贴资源 URL 链接。'
  if (input.url.trim() && !/^https?:\/\//i.test(input.url.trim())) return '资源 URL 必须以 http:// 或 https:// 开头。'
  for (const file of input.files) {
    const error = validateResourceFile(file)
    if (error === 'format') return `文件“${file.name}”格式不受支持。请使用 PPT、PPTX、MP3、PDF、DOC、DOCX 或 TXT。`
    if (error === 'size') return `文件“${file.name}”超过 25 MB。`
  }
  return null
}

/** Partition pasted files without discarding valid selections when another file is rejected. */
export function filterPastedResourceFiles(files: File[]): { accepted: File[]; errors: string[] } {
  const accepted: File[] = []
  const errors: string[] = []
  for (const file of files) {
    const error = validateResourceFile(file)
    if (error === 'format') errors.push(`${file.name}：格式不受支持`)
    else if (error === 'size') errors.push(`${file.name}：超过 25 MB`)
    else accepted.push(file)
  }
  return { accepted, errors }
}
