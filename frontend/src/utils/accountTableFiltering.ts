export type AccountTableRecord = Record<string, string | number>
export type SearchableAccountTableRecord = AccountTableRecord & { _searchText: string }

export function buildSearchableTableRecords(records: AccountTableRecord[]): SearchableAccountTableRecord[] {
  return records.map(record => ({
    ...record,
    _searchText: Object.entries(record)
      .filter(([key, value]) => key !== 'actions' && (typeof value === 'string' || typeof value === 'number'))
      .map(([, value]) => String(value))
      .join(' ')
      .toLowerCase(),
  }))
}

export function filterAccountTableRecords<T extends SearchableAccountTableRecord>(
  records: T[],
  searchQuery: string,
): T[] {
  const keyword = searchQuery.toLowerCase().trim()
  if (!keyword) return records
  const compactKeyword = keyword.replace(/\s/g, '')
  return records.filter(record => {
    const text = record._searchText
    return text.includes(keyword) || text.replace(/\s/g, '').includes(compactKeyword)
  })
}
