export type ComplianceAssetDetail = Record<string, unknown> & { id?: string; asset_id?: string }

/**
 * The list endpoint exposes id, while the detail endpoint serializes the
 * database row and exposes asset_id. Normalize both shapes before using
 * the selected record to construct mutation URLs.
 */
export function normalizeComplianceAssetDetail<T extends ComplianceAssetDetail>(item: T): T & { id: string } {
  const id = String(item.id || item.asset_id || '').trim()
  return { ...item, id }
}
