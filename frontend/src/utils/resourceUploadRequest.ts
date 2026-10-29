/**
 * 职责：封装教学资源 JSON 写请求的 XMLHttpRequest 上传进度与响应解析。
 * 依赖：生成的 ResourceWrite/TeachingResource API 类型及浏览器 XMLHttpRequest。
 * 协作：教学资源业务流程传入 URL、方法、请求体和进度回调；业务状态与用户提示由调用方管理。
 * 边界：仅负责同源凭据、JSON 请求头、上传进度和 HTTP/JSON 错误转换；不做输入校验、重试或持久化。
 */
import type { ResourceWrite, TeachingResource } from '../api/generated'

export interface ResourceWriteResponse {
  item?: TeachingResource
  error?: string
}

/** 执行单次资源写请求；上传百分比只在浏览器提供可计算进度时更新。 */
export function requestWithUploadProgress(
  url: string,
  method: string,
  body: ResourceWrite,
  onProgress: (percentage: number) => void,
): Promise<ResourceWriteResponse> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, url)
    xhr.withCredentials = true
    xhr.setRequestHeader('Content-Type', 'application/json')
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.upload.onprogress = event => {
      if (event.lengthComputable) onProgress(Math.min(99, Math.round(event.loaded / event.total * 100)))
    }
    xhr.onerror = () => reject(new Error('网络连接失败，资源未能保存。'))
    xhr.onload = () => {
      let result: ResourceWriteResponse = {}
      try { result = JSON.parse(xhr.responseText) as ResourceWriteResponse } catch { /* HTTP status below remains authoritative. */ }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(result.error || '保存资源失败。'))
        return
      }
      onProgress(100)
      resolve(result)
    }
    xhr.send(JSON.stringify(body))
  })
}
