/** Pure heading projection shared by the native editor host and its regression tests. */
import type { Editor } from '@tiptap/core'

export type ContentEditorOutlineItem = { level: number; text: string; index: number }

export function collectContentEditorOutline(current: Editor | null | undefined): ContentEditorOutlineItem[] {
  if (!current) return []
  const items: ContentEditorOutlineItem[] = []
  let headingIndex = 0
  current.state.doc.descendants(node => {
    if (node.type.name !== 'heading') return
    const index = headingIndex++
    const text = node.textContent.trim()
    if (text) items.push({ level: Number(node.attrs.level) || 1, text, index })
  })
  return items
}
