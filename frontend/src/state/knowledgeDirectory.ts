/**
 * Responsibility: manage knowledge-directory filtering, autocomplete, URL synchronization and infinite-list paging.
 * Dependencies: framework-neutral value contracts, pure search/filter helpers, and injected browser/document adapters.
 * Collaboration: the the application wraps derived getters with computed(); a native shell can render their values directly.
 * Boundaries: article navigation is delegated; URL changes only update filter query state; content fetching stays in list loaders.
 */
import { knowledgeCategoryMenus } from '../config/knowledgeCategories'
import { buildKnowledgeSearchSuggestions, knowledgeFuzzyMatch, type KnowledgeSearchSuggestion } from '../utils/knowledgeSearch'
import { filterKnowledgeItems, type KnowledgeContentItem } from '../utils/knowledgeFiltering'
import type { MutableValue, ReadonlyValue } from './valueContracts'

type DirectoryArticle = KnowledgeContentItem & { id: string; slug: string }
type StateFactory = <T>(initial: T) => MutableValue<T>
type BrowserAdapter = Pick<Window, 'location' | 'history' | 'innerHeight'>
type DocumentAdapter = Pick<Document, 'querySelector'>

export function createKnowledgeDirectoryController<T extends DirectoryArticle>(
  route: ReadonlyValue<string>,
  directoryContentItems: ReadonlyValue<T[]>,
  filteredEducationResources: ReadonlyValue<unknown[]>,
  navigate: (path: string) => unknown,
  dependencies: { createValue: StateFactory; browser: BrowserAdapter; document: DocumentAdapter },
) {
  const knowledgeVisibleCount = dependencies.createValue(10)
  const knowledgePageSize = 10
  const knowledgeCategory = dependencies.createValue('recent')
  const knowledgeSubcategory = dependencies.createValue('')
  const knowledgeSearchQuery = dependencies.createValue('')
  const highlightedKnowledgeSuggestion = dependencies.createValue(-1)
  const knowledgeFilteredItems: ReadonlyValue<T[]> = { get value() { return filterKnowledgeItems(directoryContentItems.value, knowledgeCategory.value, knowledgeSubcategory.value, knowledgeSearchQuery.value) as T[] } }
  const knowledgeSearchSuggestions: ReadonlyValue<KnowledgeSearchSuggestion[]> = { get value() {
    return buildKnowledgeSearchSuggestions(knowledgeCategoryMenus, directoryContentItems.value.map(item => ({ label: item.title, slug: item.slug, summary: item.summary || '' })), knowledgeSearchQuery.value)
  } }
  const visibleDirectoryContentItems: ReadonlyValue<T[]> = { get value() { return knowledgeFilteredItems.value.slice(0, knowledgeVisibleCount.value) } }

  function chooseKnowledgeSuggestion(suggestion: KnowledgeSearchSuggestion) {
    if (suggestion.kind === 'article') { navigate(`/content/${suggestion.slug}`); return }
    const childCategory = knowledgeCategoryMenus.find(category => category.children.includes(suggestion.label))
    const parentCategory = knowledgeCategoryMenus.find(category => category.label === suggestion.label)
    if (childCategory) selectKnowledgeCategory(childCategory.value, suggestion.label)
    else if (parentCategory) selectKnowledgeCategory(parentCategory.value)
    knowledgeSearchQuery.value = ''
    knowledgeVisibleCount.value = knowledgePageSize
  }

  function queryKnowledgeSearch(queryString: string, callback: (results: Array<Record<string, unknown>>) => void) {
    const results = knowledgeSearchSuggestions.value.map(suggestion => ({ ...suggestion, value: suggestion.label }))
      .filter(suggestion => !queryString.trim() || knowledgeFuzzyMatch(`${suggestion.value} ${'summary' in suggestion ? suggestion.summary : ''}`, queryString))
    callback(results)
  }

  function handleKnowledgeAutocompleteSelect(item: { value: string; kind: 'theme' | 'article' }) {
    const suggestion = knowledgeSearchSuggestions.value.find(candidate => candidate.label === item.value && candidate.kind === item.kind)
    if (suggestion) chooseKnowledgeSuggestion(suggestion)
    highlightedKnowledgeSuggestion.value = -1
  }

  function onKnowledgeSearchKeydown(event: KeyboardEvent) {
    const suggestions = knowledgeSearchSuggestions.value
    if (event.key === 'ArrowDown' && suggestions.length) { event.preventDefault(); highlightedKnowledgeSuggestion.value = (highlightedKnowledgeSuggestion.value + 1) % suggestions.length }
    else if (event.key === 'ArrowUp' && suggestions.length) { event.preventDefault(); highlightedKnowledgeSuggestion.value = highlightedKnowledgeSuggestion.value <= 0 ? suggestions.length - 1 : highlightedKnowledgeSuggestion.value - 1 }
    else if (event.key === 'Enter' && highlightedKnowledgeSuggestion.value >= 0 && suggestions[highlightedKnowledgeSuggestion.value]) { event.preventDefault(); chooseKnowledgeSuggestion(suggestions[highlightedKnowledgeSuggestion.value]!); highlightedKnowledgeSuggestion.value = -1 }
    else if (event.key === 'Escape') highlightedKnowledgeSuggestion.value = -1
  }

  function submitKnowledgeSearch() { knowledgeVisibleCount.value = knowledgePageSize }

  function syncKnowledgeFilterToUrl() {
    if (route.value !== '/knowledge') return
    const url = new URL(dependencies.browser.location.href)
    if (knowledgeCategory.value === 'recent') url.searchParams.delete('category')
    else url.searchParams.set('category', knowledgeCategory.value)
    if (knowledgeSubcategory.value) url.searchParams.set('subcategory', knowledgeSubcategory.value)
    else url.searchParams.delete('subcategory')
    dependencies.browser.history.replaceState(dependencies.browser.history.state, '', `${url.pathname}${url.search}${url.hash}`)
  }

  function selectKnowledgeCategory(category: string, subcategory = '') {
    knowledgeCategory.value = category
    knowledgeSubcategory.value = subcategory
    knowledgeVisibleCount.value = knowledgePageSize
    syncKnowledgeFilterToUrl()
  }

  const selectedKnowledgeCategoryLabel: ReadonlyValue<string> = { get value() {
    if (knowledgeSubcategory.value) return knowledgeSubcategory.value
    return knowledgeCategoryMenus.find(category => category.value === knowledgeCategory.value)?.label || '最近更新'
  } }
  function chooseKnowledgeCategory(categoryValue: string, subcategory = '') { selectKnowledgeCategory(categoryValue, subcategory) }

  function scrollKnowledgeCategories(direction: number) {
    const carousel = dependencies.document.querySelector<HTMLElement>('.directory-tile-carousel')
    if (carousel) carousel.scrollBy({ left: direction * Math.max(260, carousel.clientWidth * 0.78), behavior: 'smooth' })
  }

  function loadMoreKnowledge() {
    if (route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length)
      knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, knowledgeFilteredItems.value.length)
    else if (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length)
      knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, filteredEducationResources.value.length)
  }

  function isKnowledgeListNearViewportEnd() {
    if (route.value !== '/knowledge' && route.value !== '/education-resources') return false
    const list = dependencies.document.querySelector<HTMLElement>('.knowledge-infinite-list')
    return !!list && list.getBoundingClientRect().bottom <= dependencies.browser.innerHeight + 400
  }

  function onKnowledgePageScroll() {
    if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length)
      || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length)) && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
  }

  function ensureKnowledgeListFilled() {
    if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length)
      || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length)) && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
  }

  return { knowledgeVisibleCount, knowledgePageSize, knowledgeCategory, knowledgeSubcategory, knowledgeSearchQuery, highlightedKnowledgeSuggestion,
    knowledgeFilteredItems, knowledgeSearchSuggestions, visibleDirectoryContentItems, selectedKnowledgeCategoryLabel, chooseKnowledgeSuggestion,
    queryKnowledgeSearch, handleKnowledgeAutocompleteSelect, onKnowledgeSearchKeydown, submitKnowledgeSearch, selectKnowledgeCategory,
    chooseKnowledgeCategory, scrollKnowledgeCategories, loadMoreKnowledge, onKnowledgePageScroll, ensureKnowledgeListFilled }
}
