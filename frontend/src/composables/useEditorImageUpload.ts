import type { Editor } from '@tiptap/core'
import type { Ref } from 'vue'
import { portalNotify } from '../utils/notification'
import { uploadEditorImage, validateEditorImageFile } from '../utils/contentEditorImages'

type EditorImageUploadOptions = {
  editor: Ref<Editor | undefined>
  menuOpen: Ref<boolean>
  error: Ref<string>
}

export function useEditorImageUpload({ editor, menuOpen, error }: EditorImageUploadOptions) {
  function onImageFileChange(event: Event): void {
    const input = event.target as HTMLInputElement
    const file = input.files?.[0]
    if (!file || !editor.value) return

    const validation = validateEditorImageFile(file)
    if (!validation.supported) {
      if (validation.error) {
        error.value = validation.error
        portalNotify.error(validation.error, '图片上传失败')
      }
      input.value = ''
      return
    }

    error.value = ''
    const reader = new FileReader()
    reader.onload = async () => {
      if (typeof reader.result !== 'string' || !editor.value) return
      try {
        const data = reader.result.split(',')[1] || ''
        const url = await uploadEditorImage(file.type, data)
        editor.value.chain().focus().setImage({ src: url, alt: file.name }).run()
        menuOpen.value = false
      } catch (cause) {
        error.value = cause instanceof Error ? cause.message : '图片上传失败，请重试。'
        portalNotify.error(error.value, '图片上传失败')
      }
    }
    reader.readAsDataURL(file)
    input.value = ''
  }

  return { onImageFileChange }
}
