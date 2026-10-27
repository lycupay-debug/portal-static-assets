import { knowledgeFuzzyMatch } from './knowledgeSearch'
import { educationResourceSearchRank } from './resourceSearch'

type ResourceFilterItem = {
  title: string
  description?: string | null
  type: string
  grade?: string | null
  filename?: string | null
  createdAt: string
  updatedAt?: string | null
}

export function filterEducationResources<T extends ResourceFilterItem>(
  items: T[],
  category: string,
  grade: string,
  searchQuery: string,
): T[] {
  const term = searchQuery.trim()
  return items.filter(item => {
    if (category !== '全部资源' && item.type !== category) return false
    if (grade !== '全部年级' && item.grade !== grade) return false
    return !term || knowledgeFuzzyMatch(
      `${item.title} ${item.description || ''} ${item.type} ${item.grade || ''} ${item.filename || ''}`,
      term,
    )
  }).sort((a, b) => {
    if (term) {
      const relevance = educationResourceSearchRank(a, term) - educationResourceSearchRank(b, term)
      if (relevance !== 0) return relevance
    }
    return new Date(b.updatedAt || b.createdAt).getTime() - new Date(a.updatedAt || a.createdAt).getTime()
  })
}
