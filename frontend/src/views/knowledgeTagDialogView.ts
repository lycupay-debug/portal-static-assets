/**
 * Responsibility: render the hierarchical knowledge-tag picker as an accessible native DOM dialog.
 * Dependencies: taxonomy options, immutable picker snapshots and explicit selection callbacks.
 * Collaboration: portalApplication.ts owns the draft/commit state machine in knowledgeTagSelection; this view owns only DOM.
 * Boundaries: category/tag values are rendered with textContent, never interpolated into HTML; no API or persistence.
 * Lifecycle: update preserves the focused control across snapshots; dispose clears the host and releases references.
 */
export type KnowledgeTagCategory = { value: string; children: string[] }
export type KnowledgeTagDialogState = {
  open: boolean
  busy: boolean
  categories: KnowledgeTagCategory[]
  expandedGroups: string[]
  selectedTags: string[]
  draftCount: number
}

export function mountKnowledgeTagDialogView(
  host: HTMLElement,
  options: {
    getState: () => KnowledgeTagDialogState
    close: (apply: boolean) => void
    toggleGroup: (category: string) => void
    toggleTag: (tag: string, checked: boolean) => void
    clear: () => void
    setDialogElement: (element: HTMLElement | null) => void
  },
) {
  const document = host.ownerDocument
  let disposed = false

  function render(state: KnowledgeTagDialogState) {
    if (disposed) return
    const active = document.activeElement as HTMLElement | null
    const focusKey = active && host.contains(active) ? active.dataset.focusKey : undefined
    if (!state.open) {
      host.replaceChildren()
      options.setDialogElement(null)
      return
    }

    const backdrop = document.createElement('div')
    backdrop.className = 'knowledge-tag-dialog-backdrop'
    backdrop.addEventListener('click', event => { if (event.target === backdrop) options.close(false) })
    const dialog = document.createElement('section')
    dialog.className = 'knowledge-tag-dialog'
    dialog.tabIndex = -1
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'knowledge-tag-dialog-title')
    dialog.dataset.focusKey = 'dialog'
    dialog.addEventListener('keydown', event => {
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); options.close(false) }
    })

    const header = document.createElement('header'); header.className = 'knowledge-tag-dialog-header'
    const heading = document.createElement('div')
    const eyebrow = document.createElement('span'); eyebrow.className = 'knowledge-tag-dialog-eyebrow'; eyebrow.textContent = 'CONTENT TAXONOMY'
    const title = document.createElement('h2'); title.id = 'knowledge-tag-dialog-title'; title.textContent = '选择知识广场标签'
    const description = document.createElement('p'); description.textContent = '展开分类后选择具体标签；一级分类会随子标签自动关联。'
    heading.append(eyebrow, title, description)
    const close = document.createElement('button'); close.type = 'button'; close.className = 'knowledge-tag-dialog-close'; close.setAttribute('aria-label', '关闭标签选择器'); close.textContent = '×'; close.dataset.focusKey = 'close'; close.addEventListener('click', () => options.close(false))
    header.append(heading, close)

    const body = document.createElement('div'); body.className = 'knowledge-tag-dialog-body'
    for (const category of state.categories) {
      const expanded = state.expandedGroups.includes(category.value)
      const selectedChildren = category.children.filter(child => state.selectedTags.includes(child)).length
      const group = document.createElement('section'); group.className = `knowledge-tag-tree-group${expanded ? ' is-expanded' : ''}`
      const toggle = document.createElement('button'); toggle.type = 'button'; toggle.className = 'knowledge-tag-tree-heading'; toggle.setAttribute('aria-expanded', String(expanded)); toggle.dataset.focusKey = `group:${category.value}`
      const icon = document.createElement('span'); icon.className = `bi ${expanded ? 'bi-chevron-down' : 'bi-chevron-right'}`; icon.setAttribute('aria-hidden', 'true')
      const name = document.createElement('span'); name.className = 'knowledge-tag-tree-title'; name.textContent = category.value
      const note = document.createElement('span'); note.className = 'knowledge-tag-parent-note'; note.textContent = '一级分类 · 自动关联'
      const count = document.createElement('span'); count.className = 'knowledge-tag-tree-count'; count.textContent = `${selectedChildren}/${category.children.length}`
      toggle.append(icon, name, note, count); toggle.addEventListener('click', event => { event.preventDefault(); options.toggleGroup(category.value) })
      group.append(toggle)
      if (expanded) {
        const children = document.createElement('div'); children.className = 'knowledge-tag-tree-children'
        for (const child of category.children) {
          const selected = state.selectedTags.includes(child)
          const label = document.createElement('label'); label.className = `knowledge-tag-tree-option${selected ? ' is-selected' : ''}`
          const input = document.createElement('input'); input.type = 'checkbox'; input.checked = selected; input.disabled = state.busy; input.dataset.focusKey = `tag:${child}`
          input.addEventListener('change', () => options.toggleTag(child, input.checked))
          const check = document.createElement('span'); check.className = 'knowledge-tag-tree-checkbox'
          const checkIcon = document.createElement('span'); checkIcon.className = 'bi bi-check'; checkIcon.setAttribute('aria-hidden', 'true'); check.append(checkIcon)
          const text = document.createElement('span'); text.textContent = child
          label.append(input, check, text); children.append(label)
        }
        group.append(children)
      }
      body.append(group)
    }

    const footer = document.createElement('footer'); footer.className = 'knowledge-tag-dialog-footer'
    const summary = document.createElement('span'); summary.append(document.createTextNode('已选择 '))
    const selectedCount = document.createElement('strong'); selectedCount.textContent = String(state.draftCount); summary.append(selectedCount, document.createTextNode(' 个具体标签'))
    const actions = document.createElement('div')
    const clear = document.createElement('button'); clear.type = 'button'; clear.className = 'knowledge-tag-dialog-clear'; clear.textContent = '清空选择'; clear.disabled = state.busy || state.draftCount === 0; clear.dataset.focusKey = 'clear'; clear.addEventListener('click', options.clear)
    const confirm = document.createElement('button'); confirm.type = 'button'; confirm.className = 'knowledge-tag-dialog-confirm'; confirm.textContent = '完成'; confirm.dataset.focusKey = 'confirm'; confirm.addEventListener('click', () => options.close(true))
    actions.append(clear, confirm); footer.append(summary, actions)
    dialog.append(header, body, footer); backdrop.append(dialog); host.replaceChildren(backdrop)
    options.setDialogElement(dialog)
    const nextFocus = focusKey ? [...host.querySelectorAll<HTMLElement>('[data-focus-key]')].find(element => element.dataset.focusKey === focusKey) || null : null
    if (nextFocus) nextFocus.focus()
  }

  render(options.getState())
  return {
    update(state: KnowledgeTagDialogState) { render(state) },
    dispose() { if (disposed) return; disposed = true; options.setDialogElement(null); host.replaceChildren() },
  }
}
