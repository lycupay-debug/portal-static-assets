/**
 * 职责：集中校验教学资源创建/编辑表单的业务输入，避免校验规则散落在页面事件处理器中。
 * 依赖：资源文件扩展名白名单；调用方提供表单字段和待上传文件元数据。
 * 协作：管理页面在读取文件内容或发起 API 请求前调用本函数；本模块不读文件、不访问网络、不修改状态。
 * 边界：只做确定性的客户端预检；服务端仍必须独立验证权限、URL、文件类型与大小。
 */
import { resourceFileExtensions } from './resourceFiles'

export type ResourceUploadFile = Pick<File, 'name' | 'size'>

export interface ResourceUploadValidationInput {
  title: string
  isBatch: boolean
  isEditing: boolean
  files: ResourceUploadFile[]
  url: string
}

/** 返回第一条可展示的校验错误；返回 null 表示客户端预检通过。 */
export function validateResourceUpload(input: ResourceUploadValidationInput): string | null {
  if (!input.title.trim()) return '请填写资源名称。'
  if (input.isBatch && !input.isEditing && input.files.length === 0) return '批量上传至少选择一个文件。'
  if (input.files.length === 0 && !input.url.trim() && !input.isEditing) return '请上传文件或粘贴资源 URL 链接。'

  const url = input.url.trim()
  const lowerUrl = url.toLowerCase()
  if (url && !lowerUrl.startsWith('http://') && !lowerUrl.startsWith('https://')) return '资源 URL 必须以 http:// 或 https:// 开头。'

  for (const file of input.files) {
    const extension = file.name.split('.').pop()?.toLowerCase() || ''
    if (!resourceFileExtensions.has(extension)) return `文件“${file.name}”格式不受支持。请使用 PPT、PPTX、MP3、PDF、DOC、DOCX 或 TXT。`
    if (file.size > 25 * 1024 * 1024) return `文件“${file.name}”超过 25 MB。`
  }

  return null
}
