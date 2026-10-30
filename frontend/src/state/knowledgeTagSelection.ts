/**
 * Responsibility: manage the knowledge-tag picker draft, hierarchy normalization and focus restoration.
 * Dependencies: structural state-value factory, render scheduler and pure taxonomy helpers.
 * Collaboration: portalApplication.ts currently supplies signal-backed state cells as the state adapter; a native DOM host can supply observable cells.
 * Boundaries: does not render DOM, perform API requests or own component lifecycle; focus targets and scheduling are injected.
 */
import { normalizeKnowledgeTags, taxonomyParentNames } from '../utils/knowledgeTags'
import type { MutableValue, ReadonlyValue } from './valueContracts'

type TagForm = { tags: string[] }
type StateFactory = <T>(initial: T) => MutableValue<T>

export function createKnowledgeTagSelection(
  contentForm: MutableValue<TagForm>,
  contentBusy: ReadonlyValue<boolean>,
  dependencies: { createValue: StateFactory; afterRender: () => Promise<unknown> },
) {
  const { createValue, afterRender } = dependencies
  const dialogOpen = createValue(false)
  const trigger = createValue<HTMLButtonElement | null>(null)
  const dialog = createValue<HTMLElement | null>(null)
  const draft = createValue<string[]>([])
  const expandedGroups = createValue<string[]>(['教育'])
  const selectedCount: ReadonlyValue<number> = { get value() { return contentForm.value.tags.filter(tag => !taxonomyParentNames.has(tag)).length } }
  const draftCount: ReadonlyValue<number> = { get value() { return draft.value.filter(tag => !taxonomyParentNames.has(tag)).length } }
  const isSelected = (tag: string) => draft.value.includes(tag)

  async function open() {
    if (contentBusy.value) return
    draft.value = normalizeKnowledgeTags(contentForm.value.tags)
    dialogOpen.value = true
    await afterRender()
    dialog.value?.focus()
  }

  function close(apply = false) {
    if (apply) contentForm.value.tags = normalizeKnowledgeTags(draft.value)
    dialogOpen.value = false
    void afterRender().then(() => trigger.value?.focus())
  }

  function toggleGroup(category: string) {
    expandedGroups.value = expandedGroups.value.includes(category)
      ? expandedGroups.value.filter(item => item !== category)
      : [...expandedGroups.value, category]
  }

  function toggleTag(tag: string, checked: boolean) {
    const selected = draft.value.filter(item => !taxonomyParentNames.has(item))
    draft.value = normalizeKnowledgeTags(checked ? [...selected, tag] : selected.filter(item => item !== tag))
  }

  function clearDraft() { draft.value = [] }
  function removeSelected(tag: string) { contentForm.value.tags = normalizeKnowledgeTags(contentForm.value.tags.filter(item => item !== tag)) }
  function clearSelected() { contentForm.value.tags = [] }

  return { dialogOpen, trigger, dialog, draft, expandedGroups, selectedCount, draftCount, isSelected, open, close, toggleGroup, toggleTag, clearDraft, removeSelected, clearSelected }
}
