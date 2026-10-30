/**
 * Responsibility: own compliance-center data, request lifecycles, mutation payloads, and selection state.
 * Dependencies: the shared same-origin API client and the existing asset-detail normalizer.
 * Collaboration: the native DOM view renders this state and invokes explicit controller actions; Go remains
 * authoritative for administrator/reviewer authorization, policy publication rules, and audit persistence.
 * Boundaries: no DOM, Vue, routing, or implicit publishing side effects. CSV navigation is exposed as a URL.
 * Side effects: reads on refresh/open; writes only after explicit save/review/evidence/reviewer actions.
 * Async reads are sequence-guarded and dispose invalidates pending response ownership.
 */
import { ApiRequestError, apiRequest } from '../api/client'
import { normalizeComplianceAssetDetail } from '../utils/compliance'

type RecordValue = Record<string, any>
export type ComplianceTab = 'assets' | 'licenses' | 'documents' | 'scans' | 'audit' | 'reviewers'
export type ComplianceState = {
  activeTab: ComplianceTab
  loading: boolean
  saving: boolean
  error: string
  feedback: string
  permissionDenied: boolean
  overview: RecordValue
  assets: RecordValue[]
  assetsTotal: number
  assetOffset: number
  assetLimit: number
  assetType: string
  assetStatus: string
  assetRisk: string
  assetQuery: string
  licenses: RecordValue[]
  documents: RecordValue[]
  scans: RecordValue[]
  auditLogs: RecordValue[]
  reviewerUsers: RecordValue[]
  reviewerUpdateBusy: string | null
  selectedAsset: RecordValue | null
  selectedEvidence: RecordValue[]
  selectedReviews: RecordValue[]
  selectedLicense: RecordValue | null
  selectedDocument: RecordValue | null
  assetForm: RecordValue
  reviewForm: { type: string; decision: string; riskLevel: string; findings: string; actions: string }
  evidenceForm: { type: string; uri: string; text: string; notes: string; verified: boolean; verifiedBy: string }
  licenseForm: RecordValue
  documentForm: RecordValue
}

const emptyAssetForm = () => ({
  reviewStatus: 'pending', riskLevel: 'unknown', ownershipBasis: 'unknown', permissionStatus: 'unknown',
  useScope: 'unknown', distributionStatus: 'unknown', sourceUrl: '', rightsholder: '', licenseId: '', notes: '', active: false,
})
const emptyReviewForm = () => ({ type: 'license', decision: 'pending_evidence', riskLevel: 'high', findings: '', actions: '' })
const emptyEvidenceForm = () => ({ type: 'permission_email', uri: '', text: '', notes: '', verified: false, verifiedBy: '' })

export function createComplianceCenterController(options: { isAdmin?: boolean } = {}) {
  const state: ComplianceState = {
    activeTab: 'assets', loading: false, saving: false, error: '', feedback: '', permissionDenied: false,
    overview: {}, assets: [], assetsTotal: 0, assetOffset: 0, assetLimit: 100,
    assetType: '', assetStatus: '', assetRisk: '', assetQuery: '',
    licenses: [], documents: [], scans: [], auditLogs: [], reviewerUsers: [], reviewerUpdateBusy: null,
    selectedAsset: null, selectedEvidence: [], selectedReviews: [], selectedLicense: null, selectedDocument: null,
    assetForm: emptyAssetForm(), reviewForm: emptyReviewForm(), evidenceForm: emptyEvidenceForm(),
    licenseForm: {}, documentForm: {},
  }
  let disposed = false
  let refreshSequence = 0
  let assetSequence = 0
  let detailSequence = 0

  async function request<T = RecordValue>(path: string, init?: RequestInit): Promise<T> {
    try {
      return await apiRequest<T>(path, { cache: 'no-store', ...init })
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 403) state.permissionDenied = true
      throw error
    }
  }
  async function safely<T>(operation: () => Promise<T>): Promise<T | null> {
    state.error = ''
    state.feedback = ''
    try { return await operation() }
    catch (error) {
      if (!disposed) state.error = error instanceof Error ? error.message : '请求失败'
      return null
    }
  }
  async function withSaving<T>(operation: () => Promise<T>): Promise<T | null> {
    if (state.saving || disposed) return null
    state.saving = true
    try { return await safely(operation) }
    finally { if (!disposed) state.saving = false }
  }
  async function refreshAfterSave(operation: () => Promise<void>): Promise<void> {
    try { await operation() }
    catch (error) {
      if (!disposed) state.error = `保存已完成，但刷新数据失败：${error instanceof Error ? error.message : '请求失败'}。可手动刷新台账。`
    }
  }
  async function loadOverview(): Promise<void> {
    const result = await request('/compliance/overview')
    if (!disposed) state.overview = result
  }
  async function loadAssets(reset = false): Promise<void> {
    if (reset) state.assetOffset = 0
    const sequence = ++assetSequence
    const params = new URLSearchParams({ limit: String(state.assetLimit), offset: String(state.assetOffset) })
    if (state.assetType) params.set('type', state.assetType)
    if (state.assetStatus) params.set('status', state.assetStatus)
    if (state.assetRisk) params.set('risk', state.assetRisk)
    if (state.assetQuery.trim()) params.set('q', state.assetQuery.trim())
    const result = await request<RecordValue>(`/compliance/assets?${params}`)
    if (!disposed && sequence === assetSequence) {
      state.assets = result.items || []
      state.assetsTotal = result.total || 0
    }
  }
  async function loadLicenses(): Promise<void> {
    const result = await request<RecordValue>('/compliance/licenses')
    if (!disposed) state.licenses = result.items || []
  }
  async function loadDocuments(): Promise<void> {
    const result = await request<RecordValue>('/compliance/documents')
    if (!disposed) state.documents = result.items || []
  }
  async function loadScans(): Promise<void> {
    const result = await request<RecordValue>('/compliance/scans')
    if (!disposed) state.scans = result.items || []
  }
  async function loadAuditLogs(): Promise<void> {
    const result = await request<RecordValue>('/compliance/audit')
    if (!disposed) state.auditLogs = result.items || []
  }
  async function loadReviewerUsers(): Promise<void> {
    if (!options.isAdmin) { state.reviewerUsers = []; return }
    const result = await request<RecordValue>('/compliance/reviewers')
    if (!disposed) state.reviewerUsers = result.items || []
  }
  async function refreshAll(): Promise<void> {
    const sequence = ++refreshSequence
    state.loading = true
    await safely(async () => {
      await Promise.all([loadOverview(), loadAssets(true), loadLicenses(), loadDocuments(), loadScans(), loadAuditLogs(), loadReviewerUsers()])
    })
    if (!disposed && sequence === refreshSequence) state.loading = false
  }
  async function openAsset(asset: RecordValue): Promise<void> {
    const sequence = ++detailSequence
    const result = await safely(() => request<RecordValue>(`/compliance/assets/${encodeURIComponent(String(asset.id))}`))
    if (!result || disposed || sequence !== detailSequence) return
    const item = normalizeComplianceAssetDetail(result.item)
    state.selectedAsset = item
    state.selectedEvidence = result.evidence || []
    state.selectedReviews = result.reviews || []
    state.assetForm = {
      reviewStatus: item.review_status, riskLevel: item.risk_level, ownershipBasis: item.ownership_basis,
      permissionStatus: item.permission_status, useScope: item.use_scope, distributionStatus: item.distribution_status,
      sourceUrl: item.source_url, rightsholder: item.creator_or_rightsholder, licenseId: item.license_id || '',
      notes: item.notes, active: item.published_or_active,
    }
    state.reviewForm = { ...emptyReviewForm(), riskLevel: item.risk_level }
    state.evidenceForm = emptyEvidenceForm()
  }
  function selectedAssetId(): string {
    return String(state.selectedAsset?.id || state.selectedAsset?.asset_id || '')
  }
  async function saveAsset(): Promise<void> {
    const id = selectedAssetId()
    if (!id) { state.error = '无法识别资产 ID，请关闭详情并重新打开。'; return }
    await withSaving(async () => {
      const result = await request(`/compliance/assets/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(state.assetForm) })
      if (!disposed && result) {
        await refreshAfterSave(async () => { await loadAssets(); await loadOverview(); await openAsset({ id }) })
        state.feedback = '资产台账已更新。'
      }
    })
  }
  async function saveReview(): Promise<void> {
    const id = selectedAssetId()
    if (!id) { state.error = '无法识别资产 ID，请关闭详情并重新打开。'; return }
    const payload = {
      type: state.reviewForm.type, decision: state.reviewForm.decision, riskLevel: state.reviewForm.riskLevel,
      findings: state.reviewForm.findings, requiredActions: state.reviewForm.actions.split('\n').map(value => value.trim()).filter(Boolean),
    }
    await withSaving(async () => {
      await request(`/compliance/assets/${encodeURIComponent(id)}/reviews`, { method: 'POST', body: JSON.stringify(payload) })
      if (!disposed) {
        await refreshAfterSave(async () => { await loadAssets(); await loadOverview(); await openAsset({ id }) })
        state.feedback = '审核记录已保存；没有自动删除或下架内容。'
      }
    })
  }
  async function saveEvidence(): Promise<void> {
    const id = selectedAssetId()
    if (!id) { state.error = '无法识别资产 ID，请关闭详情并重新打开。'; return }
    if (!state.evidenceForm.text.trim()) { state.error = '请填写证据内容。'; return }
    await withSaving(async () => {
      await request(`/compliance/assets/${encodeURIComponent(id)}/evidence`, { method: 'POST', body: JSON.stringify(state.evidenceForm) })
      if (!disposed) { await openAsset({ id }); state.feedback = '证据记录已保存。' }
    })
  }
  async function verifyEvidence(item: RecordValue, verified: boolean): Promise<void> {
    const assetId = selectedAssetId()
    const evidenceId = String(item.id || '')
    if (!assetId || !evidenceId) return
    await withSaving(async () => {
      await request(`/compliance/assets/${encodeURIComponent(assetId)}/evidence/${encodeURIComponent(evidenceId)}`, {
        method: 'PATCH', body: JSON.stringify({ verified }),
      })
      if (!disposed) {
        await openAsset({ id: assetId })
        state.feedback = verified ? '证据已标记为核验通过；请继续提交独立审核结论。' : '证据核验标记已撤销。'
      }
    })
  }
  async function setReviewerRole(item: RecordValue, enabled: boolean): Promise<void> {
    if (!options.isAdmin || item.isAdmin) return
    state.reviewerUpdateBusy = String(item.id)
    await safely(async () => {
      const result = await request<RecordValue>(`/compliance/reviewers/${encodeURIComponent(String(item.id))}`, {
        method: 'PATCH', body: JSON.stringify({ isCopyrightReviewer: enabled }),
      })
      if (!disposed && result.item) {
        state.reviewerUsers = state.reviewerUsers.map(user => user.id === item.id ? result.item : user)
        state.feedback = `${item.displayName || item.email} 的内部合规审核人员权限已${enabled ? '启用' : '撤销'}。`
      }
    })
    if (!disposed) state.reviewerUpdateBusy = null
  }
  async function openLicense(item: RecordValue): Promise<void> {
    const result = await safely(() => request<RecordValue>(`/compliance/licenses/${encodeURIComponent(String(item.id))}`))
    if (!result || disposed) return
    state.selectedLicense = result.item
    state.licenseForm = {
      reviewStatus: result.item.reviewStatus, family: result.item.family, notes: result.item.notes || '',
      commercialUseAllowed: result.item.commercialUseAllowed, modificationAllowed: result.item.modificationAllowed,
      redistributionAllowed: result.item.redistributionAllowed, attributionRequired: result.item.attributionRequired,
      sourceCodeOfferRequired: result.item.sourceCodeOfferRequired,
    }
  }
  async function saveLicense(): Promise<void> {
    if (!state.selectedLicense) return
    const id = String(state.selectedLicense.id)
    await withSaving(async () => {
      await request(`/compliance/licenses/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(state.licenseForm) })
      if (!disposed) {
        state.feedback = '许可证审核信息已更新。'
        await refreshAfterSave(async () => { await loadLicenses(); state.selectedLicense = null })
      }
    })
  }
  function toLocalDate(value?: string | null): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return ''
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
  }
  function fromLocalDate(value?: string): string | null { return value ? new Date(value).toISOString() : null }
  function openDocument(item: RecordValue): void {
    state.selectedDocument = item
    state.documentForm = {
      bodyMarkdown: item.bodyMarkdown || '', status: item.status,
      missingInformation: (item.missingInformation || []).join('\n'),
      factualAssumptions: (item.factualAssumptions || []).join('\n'), approvedBy: item.approvedBy || '',
      legalReviewAt: toLocalDate(item.lastLegalReviewAt), effectiveAt: toLocalDate(item.effectiveAt), changeNote: '',
    }
  }
  async function saveDocument(): Promise<void> {
    if (!state.selectedDocument) return
    const slug = String(state.selectedDocument.slug)
    const payload = {
      bodyMarkdown: state.documentForm.bodyMarkdown, status: state.documentForm.status,
      missingInformation: String(state.documentForm.missingInformation || '').split('\n').map(value => value.trim()).filter(Boolean),
      factualAssumptions: String(state.documentForm.factualAssumptions || '').split('\n').map(value => value.trim()).filter(Boolean),
      approvedBy: state.documentForm.approvedBy, legalReviewAt: fromLocalDate(state.documentForm.legalReviewAt),
      effectiveAt: fromLocalDate(state.documentForm.effectiveAt), changeNote: state.documentForm.changeNote,
    }
    await withSaving(async () => {
      const result = await request<RecordValue>(`/compliance/documents/${encodeURIComponent(slug)}`, { method: 'PATCH', body: JSON.stringify(payload) })
      if (!disposed) {
        state.feedback = `政策已保存，版本 ${result.version}。`
        await refreshAfterSave(async () => {
          await loadDocuments()
          await loadOverview()
          const updated = state.documents.find(item => item.slug === slug)
          if (updated) openDocument(updated)
        })
      }
    })
  }
  function closeSelections(): void {
    ++detailSequence
    state.selectedAsset = null
    state.selectedLicense = null
    state.selectedDocument = null
  }
  function dispose(): void {
    disposed = true
    ++refreshSequence
    ++assetSequence
    ++detailSequence
  }

  const ready = refreshAll()
  return {
    state, ready, refreshAll, loadAssets, openAsset, saveAsset, saveReview, saveEvidence,
    verifyEvidence, setReviewerRole, openLicense, saveLicense, openDocument, saveDocument,
    closeSelections, dispose, exportCsvUrl: '/api/compliance/export/assets.csv',
  }
}
