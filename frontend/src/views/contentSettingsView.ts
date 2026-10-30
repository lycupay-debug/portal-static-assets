/**
 * Responsibility: render the content workspace metadata/settings panel with native DOM controls.
 * Dependencies: current content draft, visibility options and explicit state-transition callbacks.
 * Collaboration: App.vue owns draft persistence and validation; knowledgeTagSelection owns tag draft/commit rules;
 * the shared virtualized select owns its own option popover and keyboard/outside-click lifecycle.
 * Boundaries: all user-facing strings are assigned with textContent/value; this view never calls the API.
 * Lifecycle: update mutates existing nodes to preserve typing/focus; dispose releases the select and host references.
 */
import { mountVirtualizedSelect, type VirtualizedSelectOption } from '../components/virtualizedSelect'

export type ContentSettingsState = {
  title: string
  slug: string
  summary: string
  visibility: string
  tags: string[]
  busy: boolean
  tagDialogOpen: boolean
}

type FieldName = 'title' | 'slug' | 'summary'

export function mountContentSettingsView(
  host: HTMLElement,
  options: {
    getState: () => ContentSettingsState
    visibilityOptions: VirtualizedSelectOption[]
    taxonomyParentNames: ReadonlySet<string>
    setField: (field: FieldName, value: string) => void
    setVisibility: (value: string) => void
    openTagPicker: () => void
    setTagTrigger: (element: HTMLButtonElement | null) => void
    removeTag: (tag: string) => void
    clearTags: () => void
    cancel: () => void
    save: () => void
  },
) {
  const document = host.ownerDocument
  const details = document.createElement('details'); details.className = 'workspace-fold workspace-settings-fold'; details.open = true
  const summary = document.createElement('summary')
  const sectionLabel = document.createElement('span'); const summaryTitle = document.createElement('strong'); summaryTitle.textContent = '创建内容'; sectionLabel.append(summaryTitle)
  const chevron = document.createElement('span'); chevron.className = 'workspace-fold-chevron bi bi-chevron-down'; chevron.setAttribute('aria-hidden', 'true'); summary.append(sectionLabel, chevron)
  const properties = document.createElement('section'); properties.className = 'workspace-properties'

  function createInput(labelText: string, field: FieldName, name: string, placeholder: string, maxLength: number, required = false) {
    const label = document.createElement('label'); const caption = document.createElement('span'); caption.textContent = labelText
    const input = document.createElement('input'); input.className = 'form-control'; input.name = name; input.placeholder = placeholder; input.maxLength = maxLength; input.required = required
    if (field === 'slug') input.pattern = '[a-z0-9]+(-[a-z0-9]+)*'
    input.dataset.field = field
    input.addEventListener('input', () => options.setField(field, input.value))
    label.append(caption, input)
    return { label, input }
  }

  const title = createInput('内容标题', 'title', 'contentTitle', '输入内容标题', 200, true)
  const slug = createInput('URL 别名', 'slug', 'contentSlug', '例如：go-language-notes', 100, true)
  const summaryLabel = document.createElement('label'); const summaryCaption = document.createElement('span'); summaryCaption.textContent = '摘要'
  const summaryInput = document.createElement('textarea'); summaryInput.className = 'form-control'; summaryInput.name = 'contentSummary'; summaryInput.maxLength = 500; summaryInput.rows = 3; summaryInput.placeholder = '简要介绍这篇内容…'; summaryInput.dataset.field = 'summary'
  summaryInput.addEventListener('input', () => options.setField('summary', summaryInput.value)); summaryLabel.append(summaryCaption, summaryInput)

  const tagField = document.createElement('fieldset'); tagField.className = 'workspace-tag-field'
  const tagLegend = document.createElement('legend'); tagLegend.append(document.createTextNode('知识广场标签 ')); const tagHint = document.createElement('small'); tagHint.textContent = '可多选'; tagLegend.append(tagHint)
  const tagTrigger = document.createElement('button'); tagTrigger.type = 'button'; tagTrigger.className = 'workspace-tag-picker-trigger'; tagTrigger.setAttribute('aria-haspopup', 'dialog')
  const tagIcon = document.createElement('span'); tagIcon.className = 'bi bi-diagram-3'; tagIcon.setAttribute('aria-hidden', 'true')
  const tagLabel = document.createElement('span')
  const tagChevron = document.createElement('span'); tagChevron.className = 'bi bi-chevron-down'; tagChevron.setAttribute('aria-hidden', 'true')
  tagTrigger.append(tagIcon, tagLabel, tagChevron); tagTrigger.addEventListener('click', options.openTagPicker)
  options.setTagTrigger(tagTrigger)
  const tagPreview = document.createElement('div'); tagPreview.className = 'workspace-tag-selected-preview'
  const tagEmpty = document.createElement('p'); tagEmpty.className = 'workspace-tag-empty-hint'; tagEmpty.textContent = '尚未选择标签'
  tagField.append(tagLegend, tagTrigger, tagPreview, tagEmpty)

  const visibilityLabel = document.createElement('label'); const visibilityCaption = document.createElement('span'); visibilityCaption.textContent = '可见范围'
  const visibilityHost = document.createElement('div'); visibilityLabel.append(visibilityCaption, visibilityHost)
  const privacy = document.createElement('p'); privacy.className = 'workspace-privacy-note'; const privacyMark = document.createElement('span'); privacyMark.textContent = '◈'; privacy.append(privacyMark, document.createTextNode(' 内容默认保存为草稿；保存后可在内容列表单独发布。'))
  properties.append(title.label, tagField, visibilityLabel, slug.label, summaryLabel, privacy)
  details.append(summary, properties)

  const actions = document.createElement('div'); actions.className = 'workspace-create-actions'
  const cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'btn content-secondary-button'; cancel.textContent = '取消'; cancel.addEventListener('click', options.cancel)
  const save = document.createElement('button'); save.type = 'button'; save.className = 'btn btn-dark workspace-save-draft'; save.addEventListener('click', options.save)
  actions.append(cancel, save)
  const select = mountVirtualizedSelect(visibilityHost, {
    label: '可见范围', options: options.visibilityOptions, value: options.getState().visibility,
    onChange: options.setVisibility,
  })
  host.replaceChildren(details, actions)

  function update(state: ContentSettingsState) {
    const active = document.activeElement
    if (active !== title.input && title.input.value !== state.title) title.input.value = state.title
    if (active !== slug.input && slug.input.value !== state.slug) slug.input.value = state.slug
    if (active !== summaryInput && summaryInput.value !== state.summary) summaryInput.value = state.summary
    for (const input of [title.input, slug.input, summaryInput]) input.disabled = state.busy
    tagTrigger.disabled = state.busy
    tagTrigger.setAttribute('aria-expanded', String(state.tagDialogOpen))
    tagLabel.textContent = state.tags.filter(tag => !options.taxonomyParentNames.has(tag)).length ? `已选择 ${state.tags.filter(tag => !options.taxonomyParentNames.has(tag)).length} 个标签` : '点击选择标签'
    const selected = state.tags.filter(tag => !options.taxonomyParentNames.has(tag))
    tagPreview.hidden = selected.length === 0
    tagEmpty.hidden = selected.length !== 0
    tagPreview.replaceChildren()
    for (const tag of selected) {
      const chip = document.createElement('span'); chip.className = 'workspace-tag-selected-chip'; chip.append(document.createTextNode(tag))
      const remove = document.createElement('button'); remove.type = 'button'; remove.disabled = state.busy; remove.setAttribute('aria-label', `移除标签 ${tag}`); remove.textContent = '×'; remove.addEventListener('click', () => options.removeTag(tag))
      chip.append(remove); tagPreview.append(chip)
    }
    if (selected.length) {
      const clear = document.createElement('button'); clear.type = 'button'; clear.className = 'workspace-tag-clear'; clear.disabled = state.busy; clear.textContent = '清空'; clear.addEventListener('click', options.clearTags); tagPreview.append(clear)
    }
    cancel.disabled = state.busy
    save.disabled = state.busy
    save.textContent = state.busy ? '正在保存…' : '保存草稿'
    select.update({ value: state.visibility, disabled: state.busy })
  }

  update(options.getState())
  return {
    update,
    dispose() { select.dispose(); options.setTagTrigger(null); host.replaceChildren() },
  }
}
