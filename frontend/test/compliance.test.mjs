import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
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


test('compliance mutation flows share a finally-protected busy-state boundary', () => {
  const source = readFileSync(new URL('../src/components/ComplianceCenter.vue', import.meta.url), 'utf8')
  for (const name of ['saveAsset', 'saveReview', 'saveEvidence', 'verifySubmittedEvidence', 'saveLicense', 'saveDocument']) {
    const start = source.indexOf(`async function ${name}(`)
    assert.notEqual(start, -1, `${name} should exist`)
    const next = source.indexOf('\nasync function ', start + 1)
    const end = next === -1 ? source.indexOf('\nfunction exportCSV(', start) : next
    const body = source.slice(start, end)
    assert.match(body, /await withSaving\(/, `${name} must use the shared busy-state boundary`)
    assert.doesNotMatch(body, /saving\.value\s*=\s*(true|false)/, `${name} must not manage cleanup manually`)
  }
})

test('post-save refresh failures are reported without misrepresenting the write as failed', () => {
  const source = readFileSync(new URL('../src/components/ComplianceCenter.vue', import.meta.url), 'utf8')
  assert.match(source, /async function refreshAfterSave\([\s\S]*?catch\s*\(refreshError\)/)
  assert.match(source, /保存已完成，但刷新数据失败/)
  assert.match(source, /可手动刷新台账/)
})
