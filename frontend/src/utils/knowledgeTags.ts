import { portalTaxonomy } from './taxonomy'

export const removedGradeTags = new Set(['年级', '一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '初一', '初二', '初三', '高一', '高二', '高三', '本科', '硕士'])

export const knowledgeTagOptions = portalTaxonomy
  .filter(category => category.value !== '年级')
  .map(category => ({ ...category, children: category.children.filter(tag => !removedGradeTags.has(tag)) }))

export const taxonomyParentNames = new Set<string>(knowledgeTagOptions.map(category => category.value))

export const normalizeKnowledgeTags = (tags: string[]) => {
  const selectedChildren = [...new Set(tags.filter(tag => tag && !removedGradeTags.has(tag) && !taxonomyParentNames.has(tag)))]
  const selectedParents = knowledgeTagOptions
    .filter(category => category.children.some(child => selectedChildren.includes(child)))
    .map(category => category.value)
  return [...selectedParents, ...selectedChildren]
}

export const mergeLegacyTaxonomyIntoTags = (tags: string[], parentCategory?: string | null, childCategory?: string | null) =>
  normalizeKnowledgeTags([...tags, parentCategory || '', childCategory || ''])
