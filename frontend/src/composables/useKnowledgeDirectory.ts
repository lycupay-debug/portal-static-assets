import { computed, onUpdated, ref, type ComputedRef, type Ref } from 'vue'
import { knowledgeCategoryMenus } from '../config/knowledgeCategories'
import { buildKnowledgeSearchSuggestions, knowledgeFuzzyMatch, type KnowledgeSearchSuggestion } from '../utils/knowledgeSearch'
import { filterKnowledgeItems, type KnowledgeContentItem } from '../utils/knowledgeFiltering'

type DirectoryArticle = KnowledgeContentItem & {
  id: string
  slug: string
}

export function useKnowledgeDirectory<T extends DirectoryArticle>(
  route: Ref<string>,
  directoryContentItems: ComputedRef<T[]>,
  filteredEducationResources: ComputedRef<unknown[]>,
  navigate: (path: string) => unknown,
) {
  const knowledgeVisibleCount = ref(10)
  const knowledgePageSize = 10
  const knowledgeCategory = ref('recent')
  const knowledgeSubcategory = ref('')
  const knowledgeSearchQuery = ref('')
  const highlightedKnowledgeSuggestion = ref(-1)

  const knowledgeFilteredItems = computed(() => filterKnowledgeItems(
    directoryContentItems.value,
    knowledgeCategory.value,
    knowledgeSubcategory.value,
    knowledgeSearchQuery.value,
  ))
  const knowledgeSearchSuggestions = computed(() => buildKnowledgeSearchSuggestions(
    knowledgeCategoryMenus,
    directoryContentItems.value.map(item => ({ label: item.title, slug: item.slug, summary: item.summary || '' })),
    knowledgeSearchQuery.value,
  ))

  function chooseKnowledgeSuggestion(suggestion: KnowledgeSearchSuggestion) {
    if (suggestion.kind === 'article') {
      navigate(`/content/${suggestion.slug}`)
      return
    }
    const childCategory = knowledgeCategoryMenus.find(category => category.children.includes(suggestion.label))
    const parentCategory = knowledgeCategoryMenus.find(category => category.label === suggestion.label)
    if (childCategory) selectKnowledgeCategory(childCategory.value, suggestion.label)
    else if (parentCategory) selectKnowledgeCategory(parentCategory.value)
    knowledgeSearchQuery.value = ''
    knowledgeVisibleCount.value = knowledgePageSize
  }

  function queryKnowledgeSearch(queryString: string, callback: (results: Array<Record<string, unknown>>) => void) {
    const results = knowledgeSearchSuggestions.value
      .map(suggestion => ({ ...suggestion, value: suggestion.label }))
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
    if (event.key === 'ArrowDown' && suggestions.length) {
      event.preventDefault()
      highlightedKnowledgeSuggestion.value = (highlightedKnowledgeSuggestion.value + 1) % suggestions.length
    } else if (event.key === 'ArrowUp' && suggestions.length) {
      event.preventDefault()
      highlightedKnowledgeSuggestion.value = highlightedKnowledgeSuggestion.value <= 0 ? suggestions.length - 1 : highlightedKnowledgeSuggestion.value - 1
    } else if (event.key === 'Enter' && highlightedKnowledgeSuggestion.value >= 0 && suggestions[highlightedKnowledgeSuggestion.value]) {
      event.preventDefault()
      chooseKnowledgeSuggestion(suggestions[highlightedKnowledgeSuggestion.value]!)
      highlightedKnowledgeSuggestion.value = -1
    } else if (event.key === 'Escape') {
      highlightedKnowledgeSuggestion.value = -1
    }
  }

  function submitKnowledgeSearch() {
    knowledgeVisibleCount.value = knowledgePageSize
  }

  const visibleDirectoryContentItems = computed(() => knowledgeFilteredItems.value.slice(0, knowledgeVisibleCount.value))

  function syncKnowledgeFilterToUrl() {
    if (route.value !== '/knowledge') return
    const url = new URL(window.location.href)
    if (knowledgeCategory.value === 'recent') url.searchParams.delete('category')
    else url.searchParams.set('category', knowledgeCategory.value)
    if (knowledgeSubcategory.value) url.searchParams.set('subcategory', knowledgeSubcategory.value)
    else url.searchParams.delete('subcategory')
    history.replaceState(history.state, '', `${url.pathname}${url.search}${url.hash}`)
  }

  function selectKnowledgeCategory(category: string, subcategory = '') {
    knowledgeCategory.value = category
    knowledgeSubcategory.value = subcategory
    knowledgeVisibleCount.value = knowledgePageSize
    syncKnowledgeFilterToUrl()
  }

  const selectedKnowledgeCategoryLabel = computed(() => {
    if (knowledgeSubcategory.value) return knowledgeSubcategory.value
    return knowledgeCategoryMenus.find(category => category.value === knowledgeCategory.value)?.label || '最近更新'
  })

  function chooseKnowledgeCategory(categoryValue: string, subcategory = '') {
    selectKnowledgeCategory(categoryValue, subcategory)
  }

  function scrollKnowledgeCategories(direction: number) {
    const carousel = document.querySelector('.directory-tile-carousel') as HTMLElement | null
    if (carousel) carousel.scrollBy({ left: direction * Math.max(260, carousel.clientWidth * 0.78), behavior: 'smooth' })
  }

  function loadMoreKnowledge() {
    if (route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length) {
      knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, knowledgeFilteredItems.value.length)
    } else if (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length) {
      knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, filteredEducationResources.value.length)
    }
  }

  function isKnowledgeListNearViewportEnd() {
    if (route.value !== '/knowledge' && route.value !== '/education-resources') return false
    const list = document.querySelector<HTMLElement>('.knowledge-infinite-list')
    if (!list) return false
    return list.getBoundingClientRect().bottom <= window.innerHeight + 400
  }

  function onKnowledgePageScroll() {
    if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length)
      || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length))
      && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
  }

  function ensureKnowledgeListFilled() {
    // Keep the first screen populated without creating a nested scroll container.
    if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length)
      || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length))
      && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
  }
  onUpdated(ensureKnowledgeListFilled)

  return {
    knowledgeVisibleCount,
    knowledgePageSize,
    knowledgeCategory,
    knowledgeSubcategory,
    knowledgeSearchQuery,
    highlightedKnowledgeSuggestion,
    knowledgeFilteredItems,
    knowledgeSearchSuggestions,
    visibleDirectoryContentItems,
    selectedKnowledgeCategoryLabel,
    chooseKnowledgeSuggestion,
    queryKnowledgeSearch,
    handleKnowledgeAutocompleteSelect,
    onKnowledgeSearchKeydown,
    submitKnowledgeSearch,
    selectKnowledgeCategory,
    chooseKnowledgeCategory,
    scrollKnowledgeCategories,
    loadMoreKnowledge,
    onKnowledgePageScroll,
  }
}
