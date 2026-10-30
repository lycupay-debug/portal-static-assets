/**
 * Responsibility: instantiate and own a framework-independent Tiptap 3 editor in a supplied DOM element.
 * Dependencies: @tiptap/core Editor and the portal's shared Markdown/schema extension configuration.
 * Collaboration: a native DOM host owns the editor element and forwards Markdown updates to its parent.
 * Boundaries: no Vue refs, watchers, lifecycle hooks, or EditorContent wrapper; callers must dispose the instance.
 * Side effects: mounts ProseMirror into the provided element and invokes onMarkdownChange for document updates.
 */
import { Editor } from '@tiptap/core'
import { createContentEditorExtensions } from '../utils/contentEditor'

export type NativeContentEditorOptions = {
  element: HTMLElement
  markdown: string
  editable?: boolean
  onMarkdownChange?: (markdown: string) => void
}

export function createNativeContentEditor(options: NativeContentEditorOptions): Editor {
  return new Editor({
    element: options.element,
    content: options.markdown || '',
    contentType: 'markdown',
    editable: options.editable !== false,
    extensions: createContentEditorExtensions(),
    editorProps: {
      attributes: {
        class: 'content-editor-document',
        'aria-label': '文档正文编辑器',
        spellcheck: 'true',
      },
    },
    onUpdate: ({ editor }) => options.onMarkdownChange?.(editor.getMarkdown()),
  })
}
