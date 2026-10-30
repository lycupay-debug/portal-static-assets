/**
 * Responsibility: resolve a browser location into the portal's canonical client route and valid knowledge filters.
 * Dependencies: canonical route aliases and the single configured knowledge-category taxonomy.
 * Collaboration: portalApplication.ts applies the returned route/filter state before its navigation/session guards load data.
 * Boundaries: pure URL interpretation only; this module performs no history mutation, network request, DOM access,
 * session validation, or state mutation. Invalid category/subcategory values fall back to the documented defaults.
 */
import { knowledgeCategoryMenus } from '../config/knowledgeCategories'
import { routeAliases } from './portalRoutes'

export type ResolvedPortalLocation = {
  route: string
  knowledgeCategory: string
  knowledgeSubcategory: string
}

/** Resolve aliases, trailing slashes, and the optional knowledge filter query without side effects. */
export function resolvePortalLocation(pathname: string, search: string): ResolvedPortalLocation {
  const rawPath = decodeURI(pathname || '/')
  const normalizedPath = rawPath.length > 1 ? rawPath.replace(/\/+$/, '') : rawPath
  const route = routeAliases[normalizedPath] || normalizedPath
  if (route !== '/knowledge') {
    return { route, knowledgeCategory: 'recent', knowledgeSubcategory: '' }
  }

  const params = new URLSearchParams(search)
  const requestedCategory = params.get('category') || 'recent'
  const category = knowledgeCategoryMenus.some(item => item.value === requestedCategory)
    ? requestedCategory
    : 'recent'
  const requestedSubcategory = params.get('subcategory') || ''
  const categoryDefinition = knowledgeCategoryMenus.find(item => item.value === category)
  const subcategory = categoryDefinition?.children.includes(requestedSubcategory) ? requestedSubcategory : ''

  return { route, knowledgeCategory: category, knowledgeSubcategory: subcategory }
}
