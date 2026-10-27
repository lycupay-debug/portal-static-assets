export type AccountTableColumn = { key: string; label: string }
export type AccountTableDisplayRecord = Record<string, string | number>

export function tableCellValue(record: AccountTableDisplayRecord, columnKey: string): string {
  const value = record[columnKey]
  if (value === undefined || value === null || value === '') {
    return columnKey === 'title' ? '未命名内容' : '—'
  }
  return String(value)
}

export function tableCellPreview(value: string): string {
  const chars = Array.from(value)
  return chars.length > 10 ? chars.slice(0, 10).join('') + '…' : value
}

export function buildTableColumnTemplate(
  columns: AccountTableColumn[],
  records: AccountTableDisplayRecord[],
  viewportWidth: number,
  fallbackViewportWidth: number,
): string {
  if (!columns.length) return '100%'
  // Use a bounded sample so width calculation stays O(columns × 20), not O(rows × columns).
  const sample = records.slice(0, 20)
  const available = Math.max(320, viewportWidth || fallbackViewportWidth)
  const actionIndex = columns.findIndex(column => column.key === 'actions')
  const widths = columns.map(column => {
    if (column.key === 'actions') return 330
    let maxChars = Math.max(1, Array.from(column.label).length)
    for (const record of sample) {
      const preview = tableCellPreview(tableCellValue(record, column.key))
      maxChars = Math.max(maxChars, Array.from(preview).length)
    }
    return Math.max(72, maxChars * 14 + 32)
  })
  const naturalTotal = widths.reduce((sum, width) => sum + width, 0)
  if (naturalTotal > available) {
    const cap = Math.max(72, Math.floor(available * 0.15))
    return widths.map((width, index) => index === actionIndex ? 'minmax(330px, max-content)' : `${Math.min(width, cap)}px`).join(' ')
  }
  const flexibleWidth = Math.max(0, available - (actionIndex >= 0 ? (widths[actionIndex] ?? 0) : 0))
  const naturalFlexibleTotal = widths.reduce((sum, width, index) => sum + (index === actionIndex ? 0 : width), 0)
  return widths.map((width, index) => {
    if (index === actionIndex) return 'minmax(330px, max-content)'
    const proportional = naturalFlexibleTotal ? flexibleWidth * width / naturalFlexibleTotal : flexibleWidth / columns.length
    return `${Math.max(72, Math.floor(proportional))}px`
  }).join(' ')
}
