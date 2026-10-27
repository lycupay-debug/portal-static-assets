export function knowledgeEditDistance(left: string, right: string) {
  const leftChars = Array.from(left)
  const rightChars = Array.from(right)
  let previous = Array.from({ length: rightChars.length + 1 }, (_, index) => index)
  for (let i = 1; i <= leftChars.length; i++) {
    const current = [i]
    for (let j = 1; j <= rightChars.length; j++) {
      current[j] = Math.min(
        (current[j - 1] ?? i) + 1,
        (previous[j] ?? j) + 1,
        (previous[j - 1] ?? i + j) + (leftChars[i - 1] === rightChars[j - 1] ? 0 : 1),
      )
    }
    previous = current
  }
  return previous[rightChars.length] ?? rightChars.length
}

function matchesWithinEditDistance(
  term: string[],
  source: string[],
  start: number,
  size: number,
  tolerance: number,
): boolean {
  if (Math.abs(term.length - size) > tolerance) return false
  const unreachable = tolerance + 1
  let previous = Array.from({ length: size + 1 }, (_, index) => index <= tolerance ? index : unreachable)

  for (let i = 1; i <= term.length; i++) {
    const current = Array.from({ length: size + 1 }, () => unreachable)
    if (i <= tolerance) current[0] = i
    const first = Math.max(1, i - tolerance)
    const last = Math.min(size, i + tolerance)
    let rowMinimum = unreachable

    for (let j = first; j <= last; j++) {
      const substitution = (previous[j - 1] ?? unreachable) + (term[i - 1] === source[start + j - 1] ? 0 : 1)
      current[j] = Math.min(
        (current[j - 1] ?? unreachable) + 1,
        (previous[j] ?? unreachable) + 1,
        substitution,
      )
      rowMinimum = Math.min(rowMinimum, current[j] ?? unreachable)
    }
    if (rowMinimum > tolerance) return false
    previous = current
  }
  return (previous[size] ?? unreachable) <= tolerance
}

export function knowledgeFuzzyMatch(source: string, query: string) {
  const text = source.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  const term = query.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  if (!term || text.includes(term)) return true

  // Character-order matching helps Chinese partial phrases with omitted characters.
  let position = 0
  let orderedMatch = true
  for (const character of term) {
    position = text.indexOf(character, position)
    if (position < 0) {
      orderedMatch = false
      break
    }
    position++
  }
  if (orderedMatch) return true

  // Use a banded edit-distance window: the tolerance is at most two, so long
  // source text is scanned in bounded work per candidate instead of allocating
  // and evaluating a full edit-distance matrix for every substring.
  const chars = Array.from(text)
  const termChars = Array.from(term)
  if (termChars.length >= 4) {
    const tolerance = termChars.length >= 7 ? 2 : 1
    const minSize = Math.max(1, termChars.length - tolerance)
    const maxSize = termChars.length + tolerance
    for (let start = 0; start < chars.length; start++) {
      for (let size = minSize; size <= maxSize && start + size <= chars.length; size++) {
        if (matchesWithinEditDistance(termChars, chars, start, size, tolerance)) return true
      }
    }
  }
  return false
}

export type KnowledgeSearchCategory = {
  label: string
  children: string[]
}

export type KnowledgeArticleSuggestion = {
  label: string
  slug: string
  summary?: string | null
}

export type KnowledgeSearchSuggestion =
  | { label: string; kind: 'theme' }
  | { label: string; kind: 'article'; slug: string; summary: string }

export function buildKnowledgeSearchSuggestions(
  categories: KnowledgeSearchCategory[],
  articles: KnowledgeArticleSuggestion[],
  query: string,
): KnowledgeSearchSuggestion[] {
  const term = query.trim().toLocaleLowerCase()
  if (!term) return []
  const themes = categories.flatMap(category => [
    { label: category.label, kind: 'theme' as const },
    ...category.children.map(child => ({ label: child, kind: 'theme' as const })),
  ])
  const articleSuggestions = articles.map(item => ({ label: item.label, kind: 'article' as const, slug: item.slug, summary: item.summary || '' }))
  const matches = [...themes, ...articleSuggestions].filter(item => knowledgeFuzzyMatch(`${item.label} ${'summary' in item ? item.summary : ''}`, term))
  return matches.filter((item, index) => matches.findIndex(other => other.label === item.label) === index).slice(0, 8)
}
