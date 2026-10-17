import assert from 'node:assert/strict'
import test from 'node:test'
import { normalizeComplianceAssetDetail } from '../src/utils/compliance.ts'

test('normalizes the raw asset detail database key to the mutation id', () => {
  const detail = normalizeComplianceAssetDetail({ asset_id: 'asset-123', review_status: 'pending' })
  assert.equal(detail.id, 'asset-123')
  assert.equal(detail.asset_id, 'asset-123')
})

test('preserves list endpoint id and falls back only when it is absent', () => {
  assert.equal(normalizeComplianceAssetDetail({ id: 'list-123', asset_id: 'detail-456' }).id, 'list-123')
  assert.equal(normalizeComplianceAssetDetail({}).id, '')
})
