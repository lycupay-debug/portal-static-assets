/**
 * Responsibility: validate, read, upload and insert one selected editor image.
 * Dependencies: the shared image validation/upload API and a framework-independent Tiptap Editor instance.
 * Collaboration: the native editor view supplies state callbacks; portal notification reports user-facing errors.
 * Boundaries: no Vue refs or lifecycle dependency; an upload never runs without a live editor and valid image.
 */
import type { Editor } from '@tiptap/core'
import { portalNotify } from './notification'
import { uploadEditorImage, validateEditorImageFile } from './contentEditorImages'

export async function uploadAndInsertEditorImage(
  file: File,
  editor: Editor,
  onError: (message: string) => void,
): Promise<boolean> {
  const validation = validateEditorImageFile(file)
  if (!validation.supported) {
    if (validation.error) { onError(validation.error); portalNotify.error(validation.error, '图片上传失败') }
    return false
  }
  try {
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => typeof reader.result === 'string' ? resolve(reader.result) : reject(new Error('无法读取图片文件。'))
      reader.onerror = () => reject(new Error('读取图片文件失败。'))
      reader.readAsDataURL(file)
    })
    const url = await uploadEditorImage(file.type, dataUrl.split(',')[1] || '')
    editor.chain().focus().setImage({ src: url, alt: file.name }).run()
    onError('')
    return true
  } catch (cause) {
    const message = cause instanceof Error ? cause.message : '图片上传失败，请重试。'
    onError(message)
    portalNotify.error(message, '图片上传失败')
    return false
  }
}
