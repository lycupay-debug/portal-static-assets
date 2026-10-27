import { computed, nextTick, ref, type Ref } from 'vue'
import { normalizeKnowledgeTags, taxonomyParentNames } from '../utils/knowledgeTags'

type TagForm = { tags: string[] }

/** Owns the knowledge-tag picker draft, hierarchy normalization and focus restoration. */
export function useKnowledgeTagSelection(contentForm: Ref<TagForm>, contentBusy: Ref<boolean>) {
  const dialogOpen = ref(false)
  const trigger = ref<HTMLButtonElement | null>(null)
  const dialog = ref<HTMLElement | null>(null)
  const draft = ref<string[]>([])
  const expandedGroups = ref<string[]>(['教育'])

  const selectedCount = computed(() => contentForm.value.tags.filter(tag => !taxonomyParentNames.has(tag)).length)
  const draftCount = computed(() => draft.value.filter(tag => !taxonomyParentNames.has(tag)).length)
  const isSelected = (tag: string) => draft.value.includes(tag)

  async function open() {
    if (contentBusy.value) return
    draft.value = normalizeKnowledgeTags(contentForm.value.tags)
    dialogOpen.value = true
    await nextTick()
    dialog.value?.focus()
  }

  function close(apply = false) {
    if (apply) contentForm.value.tags = normalizeKnowledgeTags(draft.value)
    dialogOpen.value = false
    nextTick(() => trigger.value?.focus())
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

  function clearDraft() {
    draft.value = []
  }

  function removeSelected(tag: string) {
    contentForm.value.tags = normalizeKnowledgeTags(contentForm.value.tags.filter(item => item !== tag))
  }

  function clearSelected() {
    contentForm.value.tags = []
  }

  return {
    dialogOpen,
    trigger,
    dialog,
    draft,
    expandedGroups,
    selectedCount,
    draftCount,
    isSelected,
    open,
    close,
    toggleGroup,
    toggleTag,
    clearDraft,
    removeSelected,
    clearSelected,
  }
}
