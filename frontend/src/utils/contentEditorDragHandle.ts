import DragHandle from '@tiptap/extension-drag-handle'

/** Build the drag handle with the portal's first-line anchoring behavior. */
export function createContentEditorDragHandle() {
  let dragHandleTarget: HTMLElement | null = null
  return DragHandle.configure({
    render() {
      const button = document.createElement('button')
      button.type = 'button'
      button.className = 'content-editor-drag-handle'
      button.title = '拖动排序'
      button.setAttribute('aria-label', '块操作菜单')
      button.setAttribute('draggable', 'true')
      button.innerHTML = '<svg class="content-editor-drag-icon" aria-hidden="true" viewBox="0 0 20 20" focusable="false"><circle cx="6" cy="4" r="1.65"/><circle cx="14" cy="4" r="1.65"/><circle cx="6" cy="10" r="1.65"/><circle cx="14" cy="10" r="1.65"/><circle cx="6" cy="16" r="1.65"/><circle cx="14" cy="16" r="1.65"/></svg>'
      return button
    },
    computePositionConfig: { placement: 'left-start', strategy: 'absolute' },
    onNodeChange: change => {
      const { editor: currentEditor, node } = change
      // The installed plugin emits `pos` at runtime, although its public option type omits it.
      const pos = (change as typeof change & { pos: number }).pos
      if (!node || typeof pos !== 'number' || pos < 0) {
        dragHandleTarget = null
        return
      }
      const dom = currentEditor.view.nodeDOM(pos)
      dragHandleTarget = dom instanceof HTMLElement ? dom : null
    },
    getReferencedVirtualElement: () => {
      const root = dragHandleTarget
      if (!root) return null
      // Lists and callouts can be taller than one line. Anchor the handle to their first
      // visible text line instead of centering it against the whole block's bounding box.
      const firstLine = root.matches('ul, ol, blockquote, [data-callout]')
        ? root.querySelector(':scope > li > div > p:first-child, :scope > li > p:first-child, :scope > p:first-child, :scope > h1:first-child, :scope > h2:first-child, :scope > h3:first-child')
        : null
      const target = firstLine instanceof HTMLElement ? firstLine : root
      return {
        getBoundingClientRect: () => {
          const rect = target.getBoundingClientRect()
          // Floating UI's `left-start` aligns the handle's top edge with this virtual rect.
          // Shift the anchor by half the height difference so the handle's center sits on
          // the first line's center (text baseline, checkbox, number, or bullet row).
          const handleHeight = 32
          const alignedTop = rect.top + (rect.height - handleHeight) / 2
          if (!root.matches('ul, ol')) return new DOMRect(rect.left, alignedTop, rect.width, rect.height)
          // For lists, anchor horizontally to the list container's outer edge so the
          // handle stays in the gutter, clear of markers and task checkboxes.
          const listRect = root.getBoundingClientRect()
          return new DOMRect(listRect.left, alignedTop, listRect.width, rect.height)
        },
      }
    },
  })
}
