import { computed } from 'vue'
import type { MutableValue } from '../state/valueContracts'
import type { Content, TeachingResource } from '../api/generated'
import { tableDefinitions, contentStatusLabels, contentTypeLabels } from '../config/accountTables'
import { buildSearchableTableRecords, filterAccountTableRecords } from '../utils/accountTableFiltering'
import { buildAccountTableRecords } from '../utils/accountTableRecords'
import { buildTableColumnTemplate } from '../utils/accountTablePresentation'

type TableRecord = Record<string, string | number>
type ViewMode = 'active' | 'trash'

interface UseAccountTableOptions {
  activeAccountSection: MutableValue<string>
  contentViewMode: MutableValue<ViewMode>
  contentItems: MutableValue<Content[]>
  contentTrashItems: MutableValue<Content[]>
  resourceViewMode: MutableValue<ViewMode>
  teachingResources: MutableValue<TeachingResource[]>
  teachingResourceTrash: MutableValue<TeachingResource[]>
  debouncedTableQuery: MutableValue<string>
  tableScrollTop: MutableValue<number>
  virtualTableViewportWidth: MutableValue<number>
}

const ROW_HEIGHT = 44
const VIEWPORT_HEIGHT = 440
const OVERSCAN = 6

/** Encapsulates account-table record derivation, filtering and virtual-window math. */
export function useAccountTable(options: UseAccountTableOptions) {
  const {
    activeAccountSection, contentViewMode, contentItems, contentTrashItems,
    resourceViewMode, teachingResources, teachingResourceTrash, debouncedTableQuery,
    tableScrollTop, virtualTableViewportWidth,
  } = options
  const activeTableDefinition = computed(() => tableDefinitions[activeAccountSection.value] || null)
  const tableRecords = computed<TableRecord[]>(() => buildAccountTableRecords({
    section: activeAccountSection.value,
    contentViewMode: contentViewMode.value,
    contentItems: contentItems.value,
    contentTrashItems: contentTrashItems.value,
    resourceViewMode: resourceViewMode.value,
    teachingResources: teachingResources.value,
    teachingResourceTrash: teachingResourceTrash.value,
    contentTypeLabels,
    contentStatusLabels,
  }))

  // Normalize searchable text once per data change; UI-only action/status fields are excluded.
  const searchableTableRecords = computed(() => buildSearchableTableRecords(tableRecords.value))
  const filteredTableRecords = computed(() => filterAccountTableRecords(searchableTableRecords.value, debouncedTableQuery.value))
  const virtualTableStart = computed(() => {
    const count = filteredTableRecords.value.length
    // Clamp stale scroll positions after switching to a shorter table.
    const maxStart = Math.max(0, count - Math.ceil(VIEWPORT_HEIGHT / ROW_HEIGHT) - OVERSCAN * 2)
    return Math.min(maxStart, Math.max(0, Math.floor(tableScrollTop.value / ROW_HEIGHT) - OVERSCAN))
  })
  const virtualTableEnd = computed(() => Math.max(
    virtualTableStart.value,
    Math.min(filteredTableRecords.value.length, Math.ceil((tableScrollTop.value + VIEWPORT_HEIGHT) / ROW_HEIGHT) + OVERSCAN),
  ))
  const visibleTableRecords = computed(() => filteredTableRecords.value.slice(virtualTableStart.value, virtualTableEnd.value))
  const tableColumnTemplate = computed(() => buildTableColumnTemplate(
    activeTableDefinition.value?.columns || [],
    filteredTableRecords.value,
    virtualTableViewportWidth.value,
    window.innerWidth * 0.75,
  ))
  const virtualTableTopSpace = computed(() => virtualTableStart.value * ROW_HEIGHT)
  const virtualTableBottomSpace = computed(() => Math.max(0, (filteredTableRecords.value.length - virtualTableEnd.value) * ROW_HEIGHT))

  return {
    activeTableDefinition,
    filteredTableRecords,
    virtualTableStart,
    visibleTableRecords,
    tableColumnTemplate,
    virtualTableTopSpace,
    virtualTableBottomSpace,
  }
}
