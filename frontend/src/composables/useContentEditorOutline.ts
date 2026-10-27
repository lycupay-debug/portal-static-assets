import { ref, watch, type Ref } from 'vue'
import type { Editor } from '@tiptap/core'

export type ContentEditorOutlineItem = { level: number; text: string; index: number }

export function collectContentEditorOutline(current: Editor | null | undefined): ContentEditorOutlineItem[] {
  if (!current) return []
  const items: ContentEditorOutlineItem[] = []
  let headingIndex = 0
  current.state.doc.descendants((node) => {
    if (node.type.name !== 'heading') return
    const index = headingIndex++
    const text = node.textContent.trim()
    if (text) items.push({ level: Number(node.attrs.level) || 1, text, index })
  })
  return items
}

export function useContentEditorOutline(editor: Ref<Editor | undefined>) {
  const outlineItems = ref<ContentEditorOutlineItem[]>([])

  function refreshOutline(current: Editor | null | undefined = editor.value): void {
    outlineItems.value = collectContentEditorOutline(current)
  }

  function jumpToHeading(index: number): void {
    const root = editor.value?.view.dom
    const heading = root?.querySelectorAll('h1,h2,h3').item(index) as HTMLElement | null | undefined
    heading?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    if (heading) editor.value?.commands.focus()
  }

  watch(editor, refreshOutline, { immediate: true })
  return { outlineItems, refreshOutline, jumpToHeading }
}
