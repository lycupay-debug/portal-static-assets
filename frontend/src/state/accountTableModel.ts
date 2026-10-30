/**
 * Responsibility: derive account-table rows, filtering, responsive columns and virtual-scroll window metrics.
 * Dependencies: structural value contracts and pure record/filter/presentation utilities.
 * Collaboration: App.vue wraps the derived getters in Vue computed refs during migration; native views can consume them directly.
 * Boundaries: does not access DOM, perform requests or mutate source collections; viewport width is injected.
 */
import type { Content, TeachingResource } from '../api/generated'
import { tableDefinitions, contentStatusLabels, contentTypeLabels } from '../config/accountTables'
import { buildSearchableTableRecords, filterAccountTableRecords } from '../utils/accountTableFiltering'
import { buildAccountTableRecords } from '../utils/accountTableRecords'
import { buildTableColumnTemplate } from '../utils/accountTablePresentation'
import type { ReadonlyValue } from './valueContracts'

type TableRecord = Record<string, string | number>
type ViewMode = 'active' | 'trash'
type State<T> = ReadonlyValue<T>

export function createAccountTableModel(options: {
  activeAccountSection: State<string>
  contentViewMode: State<ViewMode>
  contentItems: State<Content[]>
  contentTrashItems: State<Content[]>
  resourceViewMode: State<ViewMode>
  teachingResources: State<TeachingResource[]>
  teachingResourceTrash: State<TeachingResource[]>
  debouncedTableQuery: State<string>
  tableScrollTop: State<number>
  virtualTableViewportWidth: State<number>
  screenWidth: () => number
}) {
  const activeTableDefinition: ReadonlyValue<(typeof tableDefinitions)[string] | null> = { get value() { return tableDefinitions[options.activeAccountSection.value] || null } }
  const tableRecords: ReadonlyValue<TableRecord[]> = { get value() { return buildAccountTableRecords({
    section: options.activeAccountSection.value,
    contentViewMode: options.contentViewMode.value,
    contentItems: options.contentItems.value,
    contentTrashItems: options.contentTrashItems.value,
    resourceViewMode: options.resourceViewMode.value,
    teachingResources: options.teachingResources.value,
    teachingResourceTrash: options.teachingResourceTrash.value,
    contentTypeLabels,
    contentStatusLabels,
  }) } }
  const searchableTableRecords: ReadonlyValue<ReturnType<typeof buildSearchableTableRecords>> = { get value() { return buildSearchableTableRecords(tableRecords.value) } }
  const filteredTableRecords: ReadonlyValue<TableRecord[]> = { get value() { return filterAccountTableRecords(searchableTableRecords.value, options.debouncedTableQuery.value) } }
  const virtualTableStart: ReadonlyValue<number> = { get value() {
    const count = filteredTableRecords.value.length
    const maxStart = Math.max(0, count - Math.ceil(440 / 44) - 12)
    return Math.min(maxStart, Math.max(0, Math.floor(options.tableScrollTop.value / 44) - 6))
  } }
  const virtualTableEnd: ReadonlyValue<number> = { get value() {
    return Math.max(virtualTableStart.value, Math.min(filteredTableRecords.value.length, Math.ceil((options.tableScrollTop.value + 440) / 44) + 6))
  } }
  const visibleTableRecords: ReadonlyValue<TableRecord[]> = { get value() { return filteredTableRecords.value.slice(virtualTableStart.value, virtualTableEnd.value) } }
  const tableColumnTemplate: ReadonlyValue<string> = { get value() {
    return buildTableColumnTemplate(activeTableDefinition.value?.columns || [], filteredTableRecords.value, options.virtualTableViewportWidth.value, options.screenWidth() * 0.75)
  } }
  const virtualTableTopSpace: ReadonlyValue<number> = { get value() { return virtualTableStart.value * 44 } }
  const virtualTableBottomSpace: ReadonlyValue<number> = { get value() { return Math.max(0, (filteredTableRecords.value.length - virtualTableEnd.value) * 44) } }
  return { activeTableDefinition, filteredTableRecords, virtualTableStart, visibleTableRecords, tableColumnTemplate, virtualTableTopSpace, virtualTableBottomSpace }
}
