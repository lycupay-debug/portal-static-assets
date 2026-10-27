export type ResourceSearchFields = {
  filename?: string | null
  title?: string | null
}

export function educationResourceSearchRank(item: ResourceSearchFields, query: string): number {
  const rawTerm = query.normalize('NFKC').toLocaleLowerCase().trim()
  const compact = (value: string) => value.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  const compactTerm = compact(rawTerm)
  const filename = (item.filename || '').normalize('NFKC').toLocaleLowerCase().trim()
  const compactFilename = compact(filename)
  const title = (item.title || '').normalize('NFKC').toLocaleLowerCase().trim()
  const compactTitle = compact(title)
  if (filename === rawTerm) return 0
  if (compactFilename === compactTerm) return 1
  if (title === rawTerm) return 2
  if (filename.includes(rawTerm)) return 3
  if (compactFilename.includes(compactTerm)) return 4
  if (title.includes(rawTerm)) return 5
  if (compactTitle.includes(compactTerm)) return 6
  return 7
}
