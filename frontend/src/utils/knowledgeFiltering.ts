import { knowledgeCategoryMenus, knowledgeCategorySearchTerms } from '../config/knowledgeCategories'
import { matchesKnowledgeTaxonomy } from './knowledgeFilters'
import { knowledgeFuzzyMatch } from './knowledgeSearch'

export type KnowledgeContentItem = {
  title: string
  summary?: string | null
  tags?: string[] | null
  parentCategory?: string | null
  childCategory?: string | null
  publishedAt?: string | null
}

export function filterKnowledgeItems<T extends KnowledgeContentItem>(
  items: T[],
  categoryValue: string,
  subcategoryValue: string,
  searchQuery: string,
): T[] {

  let filtered = [...items]
  if (categoryValue !== 'recent') {
    const selected = subcategoryValue || knowledgeCategoryMenus.find(item => item.value === categoryValue)?.label || ''
    const keywords = (subcategoryValue ? [subcategoryValue] : [selected, ...(knowledgeCategorySearchTerms[categoryValue] || [])]).filter(Boolean).map(value => value.toLocaleLowerCase())
    filtered = filtered.filter(item => {
      const category = knowledgeCategoryMenus.find(category => category.value === categoryValue)
      const categoryLabel = category?.label || ''
      if (matchesKnowledgeTaxonomy(item, categoryLabel, category?.children || [], subcategoryValue)) return true
      if (item.tags?.length || item.parentCategory || item.childCategory) return false
      return keywords.some(keyword => knowledgeFuzzyMatch(`${item.title} ${item.summary || ''}`, keyword))
    })
  }
  const searchTerm = searchQuery.trim().toLocaleLowerCase()
  if (searchTerm) filtered = filtered.filter(item => knowledgeFuzzyMatch(`${item.title} ${item.summary || ''}`, searchTerm))
  return filtered.sort((a, b) => new Date(b.publishedAt || 0).getTime() - new Date(a.publishedAt || 0).getTime())
}
