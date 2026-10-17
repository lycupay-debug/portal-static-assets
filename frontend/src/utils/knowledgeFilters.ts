export type KnowledgeTaxonomyItem = {
  tags?: string[] | null
  parentCategory?: string | null
  childCategory?: string | null
}

/** Match taxonomy filters without allowing an auto-associated parent tag to satisfy a child filter. */
export function matchesKnowledgeTaxonomy(
  item: KnowledgeTaxonomyItem,
  parentLabel: string,
  children: string[],
  selectedChild = '',
): boolean {
  const tags = item.tags || []
  if (selectedChild) {
    return item.childCategory === selectedChild || tags.includes(selectedChild)
  }
  return item.parentCategory === parentLabel
    || Boolean(item.childCategory && children.includes(item.childCategory))
    || tags.includes(parentLabel)
    || tags.some(tag => children.includes(tag))
}
