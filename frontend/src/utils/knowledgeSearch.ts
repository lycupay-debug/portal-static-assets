export function knowledgeEditDistance(left: string, right: string) {
  const previous = Array.from({ length: right.length + 1 }, (_, index) => index)
  for (let i = 1; i <= left.length; i++) {
    const current = [i]
    for (let j = 1; j <= right.length; j++) current[j] = Math.min((current[j - 1] ?? i) + 1, (previous[j] ?? j) + 1, (previous[j - 1] ?? i + j) + ((left[i - 1] ?? '') === (right[j - 1] ?? '') ? 0 : 1))
    for (let j = 0; j < current.length; j++) previous[j] = current[j] ?? j
  }
  return previous[right.length] ?? right.length
}
export function knowledgeFuzzyMatch(source: string, query: string) {
  const text = source.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  const term = query.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  if (!term || text.includes(term)) return true
  // Character-order matching helps Chinese partial phrases with omitted characters.
  let position = 0
  for (const character of term) {
    position = text.indexOf(character, position)
    if (position < 0) break
    position++
  }
  if (position > 0) {
    let check = 0
    let matched = true
    for (const character of term) { check = text.indexOf(character, check); if (check < 0) { matched = false; break } check++ }
    if (matched) return true
  }
  // Allow small spelling mistakes in longer queries using a bounded-length window.
  const chars = Array.from(text)
  const termChars = Array.from(term)
  if (termChars.length >= 4) {
    const tolerance = termChars.length >= 7 ? 2 : 1
    for (let start = 0; start < chars.length; start++) {
      for (let size = Math.max(1, termChars.length - tolerance); size <= termChars.length + tolerance && start + size <= chars.length; size++) {
        if (knowledgeEditDistance(termChars.join(''), chars.slice(start, start + size).join('')) <= tolerance) return true
      }
    }
  }
  return false
}
