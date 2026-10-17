<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { normalizeComplianceAssetDetail } from '../utils/compliance.ts'

type AnyRecord = Record<string, any>
const props = defineProps<{ isAdmin?: boolean }>()
type Tab = 'assets' | 'licenses' | 'documents' | 'complaints' | 'scans' | 'audit' | 'reviewers'
const tabs: { id: Tab; label: string }[] = [
  { id: 'assets', label: '资产台账' }, { id: 'licenses', label: '许可证目录' }, { id: 'documents', label: '政策文件' }, { id: 'complaints', label: '版权投诉' }, { id: 'scans', label: '扫描记录' }, { id: 'audit', label: '操作日志' }, { id: 'reviewers', label: '审核人员权限' },
]
const activeTab = ref<Tab>('assets')
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const permissionDenied = ref(false)
const overview = ref<AnyRecord>({})
const assets = ref<AnyRecord[]>([])
const assetsTotal = ref(0)
const assetOffset = ref(0)
const assetLimit = 100
const assetType = ref('')
const assetStatus = ref('')
const assetRisk = ref('')
const assetQuery = ref('')
const licenses = ref<AnyRecord[]>([])
const documents = ref<AnyRecord[]>([])
const complaints = ref<AnyRecord[]>([])
const scans = ref<AnyRecord[]>([])
const auditLogs = ref<AnyRecord[]>([])
const reviewerUsers = ref<AnyRecord[]>([])
const reviewerUpdateBusy = ref<string | null>(null)
const selectedAsset = ref<AnyRecord | null>(null)
const selectedEvidence = ref<AnyRecord[]>([])
const selectedReviews = ref<AnyRecord[]>([])
const selectedLicense = ref<AnyRecord | null>(null)
const selectedDocument = ref<AnyRecord | null>(null)
const selectedComplaint = ref<AnyRecord | null>(null)
const assetForm = ref<AnyRecord>({})
const reviewForm = ref({ type: 'copyright', decision: 'pending_evidence', riskLevel: 'high', findings: '', actions: '' })
const evidenceForm = ref({ type: 'permission_email', uri: '', text: '', notes: '', verified: false, verifiedBy: '' })
const licenseForm = ref<AnyRecord>({})
const documentForm = ref<AnyRecord>({})
const complaintForm = ref<AnyRecord>({})
const feedback = ref('')
const visibleTabs = computed(() => props.isAdmin ? tabs : tabs.filter(tab => tab.id !== 'complaints' && tab.id !== 'reviewers'))
const filteredAssets = computed(() => assets.value)
function reviewPreviewUrl(asset: AnyRecord): string | null {
  const path = String(asset.path || '')
  const content = path.match(/^db:\/\/portal\.portal_content\/([0-9a-f-]{36})$/i)
  if (content) return `/account/compliance-preview/content/${content[1]}`
  const resource = path.match(/^db:\/\/portal\.teaching_resources\/([0-9a-f-]{36})$/i)
  if (resource) return `/account/compliance-preview/resource/${resource[1]}`
  return null
}

async function api<T = AnyRecord>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api${path}`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
    ...init,
  })
  const contentType = response.headers.get('content-type') || ''
  const data = contentType.includes('application/json') ? await response.json() : null
  if (!response.ok) {
    if (response.status === 403) permissionDenied.value = true
    throw new Error(data?.error || `请求失败（${response.status}）`)
  }
  return data as T
}
async function run<T>(fn: () => Promise<T>) {
  error.value = ''; feedback.value = ''
  try { return await fn() } catch (e) { error.value = e instanceof Error ? e.message : '请求失败'; return null }
}
async function loadOverview() { const r = await api<AnyRecord>('/compliance/overview'); overview.value = r }
async function loadAssets(reset = false) {
  if (reset) assetOffset.value = 0
  const params = new URLSearchParams({ limit: String(assetLimit), offset: String(assetOffset.value) })
  if (assetType.value) params.set('type', assetType.value)
  if (assetStatus.value) params.set('status', assetStatus.value)
  if (assetRisk.value) params.set('risk', assetRisk.value)
  if (assetQuery.value.trim()) params.set('q', assetQuery.value.trim())
  const r = await api<AnyRecord>(`/compliance/assets?${params}`)
  assets.value = r.items || []; assetsTotal.value = r.total || 0
}
async function loadLicenses() { const r = await api<AnyRecord>('/compliance/licenses'); licenses.value = r.items || [] }
async function loadDocuments() { const r = await api<AnyRecord>('/compliance/documents'); documents.value = r.items || [] }
async function loadComplaints() { if (!props.isAdmin) { complaints.value = []; return } const r = await api<AnyRecord>('/compliance/complaints'); complaints.value = r.items || [] }
async function loadScans() { const r = await api<AnyRecord>('/compliance/scans'); scans.value = r.items || [] }
async function loadAuditLogs() { const r = await api<AnyRecord>('/compliance/audit'); auditLogs.value = r.items || [] }
async function loadReviewerUsers() { if (!props.isAdmin) { reviewerUsers.value = []; return } const r = await api<AnyRecord>('/compliance/reviewers'); reviewerUsers.value = r.items || [] }
async function setReviewerRole(item: AnyRecord, enabled: boolean) {
  reviewerUpdateBusy.value = String(item.id)
  const result = await run(() => api<AnyRecord>(`/compliance/reviewers/${item.id}`, { method: 'PATCH', body: JSON.stringify({ isCopyrightReviewer: enabled }) }))
  if (result?.item) { reviewerUsers.value = reviewerUsers.value.map(user => user.id === item.id ? result.item : user); feedback.value = `${item.displayName || item.email} 的版权审核专员权限已${enabled ? '启用' : '撤销'}。` }
  reviewerUpdateBusy.value = null
}
async function refreshAll() {
  loading.value = true
  await run(async () => { await Promise.all([loadOverview(), loadAssets(true), loadLicenses(), loadDocuments(), loadComplaints(), loadScans(), loadAuditLogs(), loadReviewerUsers()]) })
  loading.value = false
}
async function openAsset(asset: AnyRecord) {
  const r = await run(() => api<AnyRecord>(`/compliance/assets/${asset.id}`))
  if (!r) return
  selectedAsset.value = normalizeComplianceAssetDetail(r.item)
  selectedEvidence.value = r.evidence || []
  selectedReviews.value = r.reviews || []
  assetForm.value = {
    reviewStatus: r.item.review_status, riskLevel: r.item.risk_level, ownershipBasis: r.item.ownership_basis,
    permissionStatus: r.item.permission_status, useScope: r.item.use_scope, distributionStatus: r.item.distribution_status,
    sourceUrl: r.item.source_url, rightsholder: r.item.creator_or_rightsholder, licenseId: r.item.license_id || '', notes: r.item.notes, active: r.item.published_or_active,
  }
  reviewForm.value = { type: 'copyright', decision: 'pending_evidence', riskLevel: r.item.risk_level, findings: '', actions: '' }
  evidenceForm.value = { type: 'permission_email', uri: '', text: '', notes: '', verified: false, verifiedBy: '' }
}
async function saveAsset() {
  if (!selectedAsset.value) return
  const id = String(selectedAsset.value.id || selectedAsset.value.asset_id || '')
  if (!id) { error.value = '无法识别资产 ID，请关闭详情并重新打开。'; return }
  saving.value = true
  const r = await run(() => api(`/compliance/assets/${id}`, { method: 'PATCH', body: JSON.stringify(assetForm.value) }))
  if (r) { feedback.value = '资产台账已更新。'; await loadAssets(); await loadOverview(); await openAsset({ id }) }
  saving.value = false
}
async function saveReview() {
  if (!selectedAsset.value) return
  const id = String(selectedAsset.value.id || selectedAsset.value.asset_id || '')
  if (!id) { error.value = '无法识别资产 ID，请关闭详情并重新打开。'; return }
  saving.value = true
  const payload = { type: reviewForm.value.type, decision: reviewForm.value.decision, riskLevel: reviewForm.value.riskLevel, findings: reviewForm.value.findings, requiredActions: reviewForm.value.actions.split('\n').map(x => x.trim()).filter(Boolean) }
  const r = await run(() => api(`/compliance/assets/${id}/reviews`, { method: 'POST', body: JSON.stringify(payload) }))
  if (r) { feedback.value = '审核记录已保存；没有自动删除或下架内容。'; await loadAssets(); await loadOverview(); await openAsset({ id }) }
  saving.value = false
}
async function saveEvidence() {
  if (!selectedAsset.value || !evidenceForm.value.text.trim()) { error.value = '请填写证据内容。'; return }
  const id = String(selectedAsset.value.id || selectedAsset.value.asset_id || '')
  if (!id) { error.value = '无法识别资产 ID，请关闭详情并重新打开。'; return }
  saving.value = true
  const r = await run(() => api(`/compliance/assets/${id}/evidence`, { method: 'POST', body: JSON.stringify(evidenceForm.value) }))
  if (r) { feedback.value = '证据记录已保存。'; await openAsset({ id }) }
  saving.value = false
}
async function verifySubmittedEvidence(item: AnyRecord, verified: boolean) {
  if (!selectedAsset.value) return
  const assetId = String(selectedAsset.value.id || selectedAsset.value.asset_id || '')
  const evidenceId = String(item.id || '')
  if (!assetId || !evidenceId) return
  saving.value = true
  const r = await run(() => api(`/compliance/assets/${assetId}/evidence/${evidenceId}`, { method: 'PATCH', body: JSON.stringify({ verified }) }))
  if (r) { feedback.value = verified ? '证据已标记为核验通过；请继续提交独立审核结论。' : '证据核验标记已撤销。'; await openAsset({ id: assetId }) }
  saving.value = false
}
async function openLicense(item: AnyRecord) {
  const result = await run(() => api<AnyRecord>('/compliance/licenses/' + item.id))
  if (!result) return
  selectedLicense.value = result.item
  licenseForm.value = { reviewStatus: result.item.reviewStatus, family: result.item.family, notes: result.item.notes || '', commercialUseAllowed: result.item.commercialUseAllowed, modificationAllowed: result.item.modificationAllowed, redistributionAllowed: result.item.redistributionAllowed, attributionRequired: result.item.attributionRequired, sourceCodeOfferRequired: result.item.sourceCodeOfferRequired }
}
async function saveLicense() {
  if (!selectedLicense.value) return
  const id = selectedLicense.value.id
  saving.value = true
  const r = await run(() => api(`/compliance/licenses/${id}`, { method: 'PATCH', body: JSON.stringify(licenseForm.value) }))
  if (r) { feedback.value = '许可证审核信息已更新。'; await loadLicenses(); selectedLicense.value = null }
  saving.value = false
}
function toLocalDate(value?: string | null) {
  if (!value) return ''
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return ''
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}
function fromLocalDate(value?: string) { return value ? new Date(value).toISOString() : null }
function openDocument(item: AnyRecord) {
  selectedDocument.value = item
  documentForm.value = { bodyMarkdown: item.bodyMarkdown || '', status: item.status, missingInformation: (item.missingInformation || []).join('\n'), factualAssumptions: (item.factualAssumptions || []).join('\n'), approvedBy: item.approvedBy || '', legalReviewAt: toLocalDate(item.lastLegalReviewAt), effectiveAt: toLocalDate(item.effectiveAt), changeNote: '' }
}
async function saveDocument() {
  if (!selectedDocument.value) return
  const slug = selectedDocument.value.slug
  saving.value = true
  const payload = { bodyMarkdown: documentForm.value.bodyMarkdown, status: documentForm.value.status, missingInformation: String(documentForm.value.missingInformation || '').split('\n').map((x: string) => x.trim()).filter(Boolean), factualAssumptions: String(documentForm.value.factualAssumptions || '').split('\n').map((x: string) => x.trim()).filter(Boolean), approvedBy: documentForm.value.approvedBy, legalReviewAt: fromLocalDate(documentForm.value.legalReviewAt), effectiveAt: fromLocalDate(documentForm.value.effectiveAt), changeNote: documentForm.value.changeNote }
  const r = await run(() => api(`/compliance/documents/${slug}`, { method: 'PATCH', body: JSON.stringify(payload) }))
  if (r) { feedback.value = `政策已保存，版本 ${r.version}。`; await loadDocuments(); await loadOverview(); const updated = documents.value.find(x => x.slug === slug); if (updated) openDocument(updated) }
  saving.value = false
}
function openComplaint(item: AnyRecord) {
  selectedComplaint.value = item
  complaintForm.value = { status: item.status, assignedTo: item.assignedTo || '', resolutionNotes: item.resolutionNotes || '' }
}
async function saveComplaint() {
  if (!selectedComplaint.value) return
  const id = selectedComplaint.value.id
  saving.value = true
  const r = await run(() => api(`/compliance/complaints/${id}`, { method: 'PATCH', body: JSON.stringify(complaintForm.value) }))
  if (r) { feedback.value = '投诉处理记录已更新。'; await loadComplaints(); await loadOverview(); selectedComplaint.value = null }
  saving.value = false
}
function exportCSV() { window.location.href = '/api/compliance/export/assets.csv' }
function closeModals() { selectedAsset.value = null; selectedLicense.value = null; selectedDocument.value = null; selectedComplaint.value = null }
function riskLabel(value: string) { return ({ critical: '严重', high: '高', medium: '中', low: '低', unknown: '未知' } as Record<string, string>)[value] || value || '未知' }
function statusLabel(value: string) { return ({ pending: '待审核', in_review: '审核中', approved: '已通过', approved_with_conditions: '附条件通过', replace_or_remove: '建议替换/下架', blocked: '已阻止', unreviewed: '未审核', reviewed: '已审核', restricted: '受限', needs_counsel: '需法律审核', draft: '草稿', in_review_doc: '审核中', published: '已发布', archived: '已归档', received: '已收到', under_review: '处理中', need_more_information: '待补材料', action_taken: '已处理', rejected: '已驳回', closed: '已关闭' } as Record<string, string>)[value] || value || '—' }
function formatDate(value?: string | null) { if (!value) return '—'; const d = new Date(value); return Number.isNaN(d.getTime()) ? value : d.toLocaleString('zh-CN', { hour12: false }) }
onMounted(() => { void refreshAll() })
</script>

<template>
  <section class="compliance-center">
    <div class="compliance-heading">
      <div><p class="compliance-eyebrow">RIGHTS & POLICY GOVERNANCE</p><h2>版权合规中心</h2><p>资产盘点、许可证证据、审核记录和网站政策草稿。自动扫描结果不等于法律审核结论。</p></div>
      <div class="compliance-heading-actions"><button type="button" class="cc-button secondary" :disabled="loading" @click="refreshAll">{{ loading ? '正在刷新…' : '刷新台账' }}</button><button type="button" class="cc-button primary" @click="exportCSV">导出台账 CSV</button></div>
    </div>
    <p v-if="error" class="cc-alert error" role="alert">{{ error }}</p>
    <p v-if="feedback" class="cc-alert success" role="status">{{ feedback }}</p>
    <div v-if="permissionDenied" class="cc-permission"><strong>当前账户尚未获得版权合规管理权限。</strong><p>台账和投诉数据已由服务端进行管理员权限校验。请勿通过公开注册账户访问这些数据；需要将经确认的站点管理账户授权为管理员后才能使用管理操作。</p></div>
    <template v-else>
      <div class="cc-metrics">
        <article><span>资产记录</span><strong>{{ overview.assetCount ?? '—' }}</strong><small>文件、文章、依赖与外链</small></article>
        <article><span>待审核资产</span><strong>{{ overview.pendingAssetCount ?? '—' }}</strong><small>待补证据或待复核</small></article>
        <article class="risk"><span>高风险待核验</span><strong>{{ overview.highRiskCount ?? '—' }}</strong><small>不代表已认定侵权</small></article>
        <article><span>许可证候选</span><strong>{{ overview.licenseCount ?? '—' }}</strong><small>{{ overview.unreviewedLicenseCount ?? '—' }} 条尚未人工审核</small></article>
      </div>
      <nav class="cc-tabs" aria-label="版权合规管理模块"><button v-for="tab in visibleTabs" :key="tab.id" type="button" :class="{ active: activeTab === tab.id }" @click="activeTab = tab.id">{{ tab.label }}<span v-if="tab.id === 'complaints' && overview.openComplaintCount">{{ overview.openComplaintCount }}</span></button></nav>
      <section v-if="activeTab === 'assets'" class="cc-panel">
        <div class="cc-panel-heading"><div><h3>版权资产台账</h3><p>优先显示严重风险与来源待核实项；更新审核状态不会自动更改线上内容。</p></div><span class="cc-count">{{ assetsTotal }} 条记录</span></div>
        <div class="cc-filters"><input v-model="assetQuery" type="search" placeholder="搜索名称、路径或来源 URL" @keydown.enter="run(() => loadAssets(true))"><select v-model="assetType" aria-label="资产类型"><option value="">全部类型</option><option value="article">文章</option><option value="education_resource">教育资源</option><option value="software_dependency">软件依赖</option><option value="icon">图标</option><option value="image">图片</option><option value="font">字体</option><option value="audio">音频</option><option value="external_link">外部链接</option><option value="source_file">源文件</option></select><select v-model="assetRisk" aria-label="风险等级"><option value="">全部风险</option><option value="critical">严重</option><option value="high">高</option><option value="medium">中</option><option value="low">低</option><option value="unknown">未知</option></select><select v-model="assetStatus" aria-label="审核状态"><option value="">全部状态</option><option value="pending">待审核</option><option value="in_review">审核中</option><option value="approved">已通过</option><option value="approved_with_conditions">附条件通过</option><option value="replace_or_remove">建议替换/下架</option><option value="blocked">已阻止</option></select><button class="cc-button secondary" type="button" @click="run(() => loadAssets(true))">筛选</button></div>
        <div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>资产</th><th>类型</th><th>授权状态</th><th>风险</th><th>审核状态</th><th>公开/分发</th><th>操作</th></tr></thead><tbody><tr v-for="item in filteredAssets" :key="item.id"><td class="asset-name"><strong>{{ item.name }}</strong><small>{{ item.path }}</small><a v-if="reviewPreviewUrl(item)" class="cc-text-button" :href="reviewPreviewUrl(item)!" target="_blank" rel="noopener noreferrer">受限预览 ↗</a><a v-if="item.sourceUrl" :href="item.sourceUrl" target="_blank" rel="noopener noreferrer">查看来源 ↗</a></td><td>{{ item.type }}</td><td>{{ item.permissionStatus }}</td><td><span class="cc-risk" :class="item.riskLevel">{{ riskLabel(item.riskLevel) }}</span></td><td>{{ statusLabel(item.reviewStatus) }}</td><td>{{ item.active ? item.distributionStatus : '非活动/源文件' }}</td><td><button type="button" class="cc-text-button" @click="openAsset(item)">审核详情</button></td></tr><tr v-if="!assets.length"><td colspan="7" class="cc-empty">{{ loading ? '正在读取…' : '没有符合条件的资产。' }}</td></tr></tbody></table></div>
        <div class="cc-pagination"><span>显示 {{ assets.length }} / {{ assetsTotal }} 条</span><div><button class="cc-button secondary" :disabled="assetOffset <= 0" @click="assetOffset = Math.max(0, assetOffset - assetLimit); run(() => loadAssets())">上一页</button><button class="cc-button secondary" :disabled="assetOffset + assets.length >= assetsTotal" @click="assetOffset += assetLimit; run(() => loadAssets())">下一页</button></div></div>
      </section>
      <section v-else-if="activeTab === 'reviewers'" class="cc-panel"><div class="cc-panel-heading"><div><h3>版权审核专员权限</h3><p>只有超管可以分配或撤销。专员可以查看受限内容预览和版权资产记录，但不能查看投诉人的个人信息。</p></div><span class="cc-count">{{ reviewerUsers.filter(item => item.isCopyrightReviewer).length }} 位专员</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>账户</th><th>账户权限</th><th>版权审核专员</th><th>创建时间</th></tr></thead><tbody><tr v-for="item in reviewerUsers" :key="item.id"><td><strong>{{ item.displayName || '未设置昵称' }}</strong><small>{{ item.email }}</small></td><td>{{ item.isAdmin ? '超管' : '普通账户' }}</td><td><label class="cc-check"><input type="checkbox" :checked="item.isCopyrightReviewer" :disabled="item.isAdmin || reviewerUpdateBusy === item.id" @change="setReviewerRole(item, ($event.target as HTMLInputElement).checked)"><span>{{ item.isAdmin ? '超管默认拥有预览权限' : item.isCopyrightReviewer ? '已授权' : '未授权' }}</span></label></td><td>{{ formatDate(item.createdAt) }}</td></tr><tr v-if="!reviewerUsers.length"><td colspan="4" class="cc-empty">暂无账户</td></tr></tbody></table></div></section><section v-else-if="activeTab === 'licenses'" class="cc-panel"><div class="cc-panel-heading"><div><h3>许可证目录</h3><p>来源包括 package-lock、包内许可证文件及现有第三方声明文件。</p></div><span class="cc-count">{{ licenses.length }} 条</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>许可证</th><th>类型</th><th>商业使用标记</th><th>文本收录</th><th>审核</th><th></th></tr></thead><tbody><tr v-for="item in licenses" :key="item.id"><td><strong>{{ item.name }}</strong><small v-if="item.canonicalUrl"><a :href="item.canonicalUrl" target="_blank" rel="noopener noreferrer">许可证参考 ↗</a></small></td><td>{{ item.family }}</td><td>{{ item.commercialUseAllowed === true ? '元数据标记允许' : item.commercialUseAllowed === false ? '元数据标记受限' : '未判定' }}</td><td>{{ item.licenseTextCaptured ? '已收录文本' : '缺少文本' }}</td><td>{{ statusLabel(item.reviewStatus) }}</td><td><button class="cc-text-button" @click="openLicense(item)">复核</button></td></tr><tr v-if="!licenses.length"><td colspan="6" class="cc-empty">暂无许可证记录</td></tr></tbody></table></div></section>
      <section v-else-if="activeTab === 'documents'" class="cc-panel"><div class="cc-panel-heading"><div><h3>政策文件</h3><p>政策当前均为草稿。缺少主体、联系渠道或实际数据处理事实时，系统会阻止发布。</p></div><span class="cc-count">{{ documents.length }} 份</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>政策</th><th>辖区</th><th>版本</th><th>状态</th><th>缺失事实</th><th></th></tr></thead><tbody><tr v-for="item in documents" :key="item.id"><td><strong>{{ item.title }}</strong><small>/legal/{{ item.slug }}</small></td><td>{{ item.jurisdiction }}</td><td>v{{ item.version }}</td><td>{{ statusLabel(item.status) }}</td><td>{{ (item.missingInformation || []).length }}</td><td><button class="cc-text-button" @click="openDocument(item)">编辑/审核</button></td></tr><tr v-if="!documents.length"><td colspan="6" class="cc-empty">暂无政策文档</td></tr></tbody></table></div></section>
      <section v-else-if="activeTab === 'complaints'" class="cc-panel"><div class="cc-panel-heading"><div><h3>版权投诉</h3><p>投诉入口在“版权投诉”政策正式发布前不会接收个人信息。</p></div><span class="cc-count">{{ complaints.length }} 条</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>投诉人</th><th>作品说明</th><th>链接数</th><th>收到时间</th><th>状态</th><th></th></tr></thead><tbody><tr v-for="item in complaints" :key="item.id"><td><strong>{{ item.name }}</strong><small>{{ item.email }}</small></td><td>{{ item.workDescription }}</td><td>{{ (item.infringingUrls || []).length }}</td><td>{{ formatDate(item.receivedAt) }}</td><td>{{ statusLabel(item.status) }}</td><td><button class="cc-text-button" @click="openComplaint(item)">处理</button></td></tr><tr v-if="!complaints.length"><td colspan="6" class="cc-empty">暂无投诉记录</td></tr></tbody></table></div></section>
      <section v-else-if="activeTab === 'audit'" class="cc-panel"><div class="cc-panel-heading"><div><h3>操作审计日志</h3><p>记录资产、证据、审核、许可证、政策与投诉状态的变更；投诉审计快照会排除联系信息和证明正文。</p></div><span class="cc-count">{{ auditLogs.length }} 条</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>时间</th><th>操作者</th><th>动作</th><th>对象</th><th>变更快照</th></tr></thead><tbody><tr v-for="item in auditLogs" :key="item.id"><td>{{ formatDate(item.createdAt) }}</td><td>{{ item.actor }}</td><td>{{ item.action }}</td><td><strong>{{ item.objectType }}</strong><small>{{ item.objectId }}</small></td><td><details><summary>查看前后状态</summary><pre class="cc-audit-json">前：{{ JSON.stringify(item.beforeState, null, 2) }}
后：{{ JSON.stringify(item.afterState, null, 2) }}</pre></details></td></tr><tr v-if="!auditLogs.length"><td colspan="5" class="cc-empty">暂无操作记录</td></tr></tbody></table></div></section>
      <section v-else class="cc-panel"><div class="cc-panel-heading"><div><h3>扫描记录</h3><p>保留每次扫描的范围、版本、计数和报告位置。</p></div><span class="cc-count">{{ scans.length }} 次</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>类型</th><th>状态</th><th>资产数</th><th>发现数</th><th>版本</th><th>开始时间</th><th>报告</th></tr></thead><tbody><tr v-for="item in scans" :key="item.id"><td>{{ item.scanType }}</td><td>{{ item.status }}</td><td>{{ item.assetsSeen }}</td><td>{{ item.findingsCount }}</td><td>{{ item.sourceRevision || '—' }}</td><td>{{ formatDate(item.startedAt) }}</td><td>{{ item.reportPath || '—' }}</td></tr><tr v-if="!scans.length"><td colspan="7" class="cc-empty">暂无扫描记录</td></tr></tbody></table></div></section>
    </template>

    <div v-if="selectedAsset" class="cc-backdrop" @click.self="closeModals" @keydown.esc="closeModals"><section class="cc-modal wide" role="dialog" aria-modal="true" aria-label="资产审核详情"><header><div><p class="compliance-eyebrow">ASSET REVIEW</p><h3>{{ selectedAsset.asset_name }}</h3><small>{{ selectedAsset.asset_path }}</small></div><button class="cc-close" aria-label="关闭" @click="closeModals">×</button></header><div class="cc-modal-content"><div class="cc-detail-summary"><span>SHA-256</span><code>{{ selectedAsset.sha256 || '未记录' }}</code><span>文件大小</span><code>{{ selectedAsset.file_size_bytes ? `${(selectedAsset.file_size_bytes / 1024 / 1024).toFixed(2)} MB` : '—' }}</code><span>发现方式</span><code>{{ selectedAsset.discovered_by }}</code></div><div v-if="selectedAsset.asset_type === 'image' && String(selectedAsset.asset_path || '').startsWith('/uploads/') && selectedAsset.metadata?.uploader_id" class="cc-image-preview"><img :src="selectedAsset.asset_path" alt="版权审核图片预览"><p>该图片仅对登录的上传者和管理员开放预览；公开访问需通过独立权利审核。</p></div><div class="cc-form-grid"><label>审核状态<select v-model="assetForm.reviewStatus"><option value="pending">待审核</option><option value="in_review">审核中</option><option value="approved">已通过</option><option value="approved_with_conditions">附条件通过</option><option value="replace_or_remove">建议替换/下架</option><option value="blocked">已阻止</option></select></label><label>风险等级<select v-model="assetForm.riskLevel"><option value="critical">严重</option><option value="high">高</option><option value="medium">中</option><option value="low">低</option><option value="unknown">未知</option></select></label><label>权利依据<select v-model="assetForm.ownershipBasis"><option value="unknown">未知</option><option value="original">原创</option><option value="commissioned">委托创作</option><option value="licensed">已授权</option><option value="open_license">开放许可证</option><option value="public_domain">公有领域</option><option value="user_submitted">用户提交</option><option value="third_party_permission">第三方许可</option></select></label><label>关联许可证<select v-model="assetForm.licenseId"><option value="">无</option><option v-for="license in licenses" :key="license.id" :value="license.id">{{ license.name }} {{ license.version || '' }} · {{ license.reviewStatus }}</option></select></label><label>授权状态<select v-model="assetForm.permissionStatus"><option value="unknown">未知</option><option value="confirmed">已确认</option><option value="conditional">有条件</option><option value="not_permitted">不允许</option><option value="not_applicable">不适用</option></select></label><label>使用范围<select v-model="assetForm.useScope"><option value="unknown">未知</option><option value="commercial_web">商业网站</option><option value="internal_only">仅内部</option><option value="redistribution">再分发</option><option value="modification">修改</option><option value="multiple">多种用途</option></select></label><label>分发方式<select v-model="assetForm.distributionStatus"><option value="unknown">未知</option><option value="served_publicly">公开提供</option><option value="bundled_in_frontend">前端打包</option><option value="server_side_only">仅服务端</option><option value="internal_only">仅内部</option><option value="not_distributed">未分发</option></select></label><label class="full">来源 URL<input v-model="assetForm.sourceUrl" type="url" placeholder="来源或授权页面"></label><label class="full">权利人/作者<input v-model="assetForm.rightsholder" placeholder="未知时请保持未知，不要猜测"></label><label class="full">审核备注<textarea v-model="assetForm.notes" rows="3"></textarea></label><label class="cc-check full"><input v-model="assetForm.active" type="checkbox"> 台账标记为公开/活动（不更改实际线上状态）</label></div><div class="cc-actions"><button class="cc-button primary" :disabled="saving" @click="saveAsset">保存资产信息</button></div><h4>新增证据</h4><div class="cc-form-grid"><label>证据类型<select v-model="evidenceForm.type"><option value="license_text">许可证原文</option><option value="source_page">来源页面</option><option value="purchase_receipt">购买凭证</option><option value="permission_email">授权邮件</option><option value="contract">合同</option><option value="author_declaration">作者声明</option><option value="screenshot">截图</option><option value="hash_record">哈希记录</option><option value="repository_metadata">仓库元数据</option><option value="legal_review">法律审核</option><option value="other">其他</option></select></label><label>证据位置<input v-model="evidenceForm.uri" placeholder="URL 或内部文件路径"></label><label class="full">证据内容/核验摘要<textarea v-model="evidenceForm.text" rows="3" placeholder="记录实际看到的许可证、授权范围、作者信息等"></textarea></label><label class="full">备注<textarea v-model="evidenceForm.notes" rows="2"></textarea></label><p class="full">新增证据默认标记为未核验。保存后请在证据历史中执行独立核验。</p></div><div class="cc-actions"><button class="cc-button secondary" :disabled="saving" @click="saveEvidence">保存证据</button></div><h4>审核记录</h4><div class="cc-form-grid"><label>审核类型<select v-model="reviewForm.type"><option value="copyright">版权</option><option value="license">许可证</option><option value="content">内容</option><option value="distribution">分发</option><option value="legal_basis">法律依据</option><option value="recheck">复查</option></select></label><label>审核结论<select v-model="reviewForm.decision"><option value="pending_evidence">待补证据</option><option value="approved">通过</option><option value="approved_with_conditions">附条件通过</option><option value="replace">建议替换</option><option value="remove">建议下架</option><option value="blocked">阻止使用</option><option value="escalate">升级法律审核</option></select></label><label>风险等级<select v-model="reviewForm.riskLevel"><option value="critical">严重</option><option value="high">高</option><option value="medium">中</option><option value="low">低</option><option value="unknown">未知</option></select></label><label class="full">发现与判断依据<textarea v-model="reviewForm.findings" rows="3" placeholder="区分已证实事实、疑点和未确认事项"></textarea></label><label class="full">后续动作（每行一项）<textarea v-model="reviewForm.actions" rows="3" placeholder="核对作者 / 补授权 / 替换素材"></textarea></label></div><div class="cc-actions"><button class="cc-button primary" :disabled="saving" @click="saveReview">保存审核记录</button></div><h4>证据历史</h4><ul class="cc-history"><li v-for="item in selectedEvidence" :key="item.id"><strong>{{ item.type }}</strong><a v-if="String(item.uri || '').startsWith('/api/compliance/assets/')" :href="item.uri">下载受限证据附件</a><span v-else>{{ item.uri || '未提供位置' }}</span><p>{{ item.text || item.notes }}</p><small>{{ formatDate(item.capturedAt) }} · {{ item.verified ? '已核验' : '未核验' }}</small><button v-if="!item.verified" type="button" class="cc-button secondary" :disabled="saving" @click="verifySubmittedEvidence(item, true)">核验证据</button><button v-else type="button" class="cc-button secondary" :disabled="saving" @click="verifySubmittedEvidence(item, false)">撤销核验</button></li><li v-if="!selectedEvidence.length">暂无证据记录</li></ul><h4>历史审核</h4><ul class="cc-history"><li v-for="item in selectedReviews" :key="item.id"><strong>{{ item.decision }} · {{ riskLabel(item.riskLevel) }}</strong><p>{{ item.findings }}</p><small>{{ item.reviewer }} · {{ formatDate(item.reviewedAt) }}</small></li><li v-if="!selectedReviews.length">暂无审核记录</li></ul></div></section></div>

    <div v-if="selectedLicense" class="cc-backdrop" @click.self="closeModals"><section class="cc-modal" role="dialog" aria-modal="true" aria-label="许可证审核"><header><div><p class="compliance-eyebrow">LICENSE REVIEW</p><h3>{{ selectedLicense.name }}</h3><small>{{ selectedLicense.spdxId }} {{ selectedLicense.version }}</small></div><button class="cc-close" @click="closeModals">×</button></header><div class="cc-modal-content"><section class="cc-license-text-block"><h4>收录的许可证原文</h4><p v-if="!selectedLicense.fullText">当前没有捕获到许可证全文，需要从对应包的 LICENSE/COPYING 文件补齐。</p><pre v-else class="cc-license-text">{{ selectedLicense.fullText }}</pre><h4>关联资产与依赖实例</h4><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>资产</th><th>类型</th><th>授权状态</th><th>审核</th><th>风险</th></tr></thead><tbody><tr v-for="asset in selectedLicense.linkedAssets || []" :key="asset.id"><td><strong>{{ asset.name }}</strong><small>{{ asset.path }}</small></td><td>{{ asset.assetType }}</td><td>{{ asset.permissionStatus }}</td><td>{{ statusLabel(asset.reviewStatus) }}</td><td>{{ riskLabel(asset.riskLevel) }}</td></tr><tr v-if="!(selectedLicense.linkedAssets || []).length"><td colspan="5" class="cc-empty">没有直接关联的资产记录；请检查许可证表达式和包锁文件映射。</td></tr></tbody></table></div></section><div class="cc-form-grid"><label>审核状态<select v-model="licenseForm.reviewStatus"><option value="unreviewed">未审核</option><option value="reviewed">已审核</option><option value="restricted">受限</option><option value="needs_counsel">需法律审核</option></select></label><label>许可证类型<select v-model="licenseForm.family"><option value="permissive">宽松型</option><option value="copyleft">强 Copyleft</option><option value="weak_copyleft">弱 Copyleft</option><option value="proprietary">专有</option><option value="public_domain">公有领域</option><option value="custom">自定义</option><option value="other">其他/组合许可</option></select></label><label class="full">复核备注<textarea v-model="licenseForm.notes" rows="4"></textarea></label><label class="cc-check"><input v-model="licenseForm.commercialUseAllowed" type="checkbox">确认允许商业使用</label><label class="cc-check"><input v-model="licenseForm.modificationAllowed" type="checkbox">确认允许修改</label><label class="cc-check"><input v-model="licenseForm.redistributionAllowed" type="checkbox">确认允许再分发</label><label class="cc-check"><input v-model="licenseForm.attributionRequired" type="checkbox">需要署名/保留声明</label><label class="cc-check"><input v-model="licenseForm.sourceCodeOfferRequired" type="checkbox">需要提供对应源代码</label></div><div class="cc-actions"><button class="cc-button primary" :disabled="saving" @click="saveLicense">保存许可证审核</button></div></div></section></div>

    <div v-if="selectedDocument" class="cc-backdrop" @click.self="closeModals"><section class="cc-modal wide" role="dialog" aria-modal="true" aria-label="政策文件编辑"><header><div><p class="compliance-eyebrow">LEGAL DOCUMENT DRAFT</p><h3>{{ selectedDocument.title }}</h3><small>{{ selectedDocument.slug }} · 当前版本 v{{ selectedDocument.version }}</small></div><button class="cc-close" @click="closeModals">×</button></header><div class="cc-modal-content"><p class="cc-alert warning">政策发布会公开面向用户。运营主体、联系渠道、数据处理事实、授权事实或法律审核未完成时，请保持草稿/审核中。</p><div class="cc-form-grid"><label>状态<select v-model="documentForm.status"><option value="draft">草稿</option><option value="in_review">审核中</option><option value="published">发布</option><option value="archived">归档</option></select></label><label>审核人/批准人<input v-model="documentForm.approvedBy" placeholder="真实审核人或授权负责人"></label><label>法律审核时间<input v-model="documentForm.legalReviewAt" type="datetime-local"></label><label>生效时间<input v-model="documentForm.effectiveAt" type="datetime-local"></label><label class="full">正文 Markdown<textarea v-model="documentForm.bodyMarkdown" rows="18" spellcheck="false"></textarea></label><label class="full">尚缺事实（每行一项）<textarea v-model="documentForm.missingInformation" rows="4" placeholder="全部核实后才可清空"></textarea></label><label class="full">事实假设与核验说明（每行一项）<textarea v-model="documentForm.factualAssumptions" rows="3"></textarea></label><label class="full">版本变更说明<input v-model="documentForm.changeNote" placeholder="本次修改内容"></label></div><div class="cc-actions"><button class="cc-button secondary" :disabled="saving" @click="documentForm.status = 'draft'; saveDocument()">保存为草稿</button><button class="cc-button primary" :disabled="saving" @click="saveDocument">保存/提交状态</button></div></div></section></div>

    <div v-if="selectedComplaint" class="cc-backdrop" @click.self="closeModals"><section class="cc-modal" role="dialog" aria-modal="true" aria-label="版权投诉处理"><header><div><p class="compliance-eyebrow">COPYRIGHT COMPLAINT</p><h3>{{ selectedComplaint.name }}</h3><small>{{ selectedComplaint.email }} · {{ formatDate(selectedComplaint.receivedAt) }}</small></div><button class="cc-close" @click="closeModals">×</button></header><div class="cc-modal-content"><p><strong>作品说明</strong><br>{{ selectedComplaint.workDescription }}</p><p><strong>权利证明</strong><br>{{ selectedComplaint.proofOfRights || '未提供' }}</p><p><strong>涉及链接</strong></p><ul><li v-for="url in selectedComplaint.infringingUrls" :key="url"><a :href="url" target="_blank" rel="noopener noreferrer">{{ url }}</a></li></ul><div class="cc-form-grid"><label>处理状态<select v-model="complaintForm.status"><option value="received">已收到</option><option value="under_review">处理中</option><option value="need_more_information">待补材料</option><option value="action_taken">已采取措施</option><option value="rejected">驳回</option><option value="closed">关闭</option></select></label><label>负责人<input v-model="complaintForm.assignedTo"></label><label class="full">处理记录<textarea v-model="complaintForm.resolutionNotes" rows="5"></textarea></label></div><div class="cc-actions"><button class="cc-button primary" :disabled="saving" @click="saveComplaint">保存处理记录</button></div></div></section></div>
  </section>
</template>

<style scoped>
.compliance-center{color:#202020;min-width:0}.compliance-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin:0 0 24px}.compliance-heading h2{font-size:26px;font-weight:650;letter-spacing:-.035em;margin:0 0 8px}.compliance-heading p{margin:0;color:#686868;font-size:14px;line-height:1.65;max-width:720px}.compliance-eyebrow{font-size:11px!important;letter-spacing:.12em;color:#777!important;margin:0 0 7px!important;font-weight:650}.compliance-heading-actions{display:flex;gap:8px;flex-shrink:0}.cc-button{border:1px solid #d7d7d7;border-radius:7px;padding:9px 13px;font-size:13px;cursor:pointer;background:#fff;color:#222}.cc-button.primary{background:#222;color:#fff;border-color:#222}.cc-button:disabled{opacity:.45;cursor:not-allowed}.cc-button.secondary{background:#fff}.cc-alert{padding:11px 13px;border-radius:7px;margin:0 0 16px;font-size:13px;line-height:1.6}.cc-alert.error{background:#fff0f0;color:#a32121}.cc-alert.success{background:#f0f7f1;color:#235d2c}.cc-alert.warning{background:#fff8e7;color:#6b4a00}.cc-permission{border:1px solid #e3e3e3;padding:22px;border-radius:9px;margin-bottom:20px}.cc-permission p{color:#666;line-height:1.7;font-size:14px}.cc-metrics{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-bottom:24px}.cc-metrics article{border:1px solid #e8e8e8;border-radius:9px;padding:16px;display:flex;flex-direction:column;gap:7px;background:#fff}.cc-metrics article span{color:#666;font-size:12px}.cc-metrics article strong{font-size:27px;line-height:1.2;font-weight:650;font-variant-numeric:tabular-nums}.cc-metrics article small{font-size:11px;color:#8a8a8a}.cc-metrics article.risk strong{color:#9c3b29}.cc-tabs{display:flex;gap:4px;border-bottom:1px solid #e3e3e3;margin-bottom:18px;overflow-x:auto}.cc-tabs button{white-space:nowrap;background:transparent;border:0;border-bottom:2px solid transparent;padding:12px 14px;color:#666;font-size:13px;cursor:pointer}.cc-tabs button.active{border-bottom-color:#202020;color:#202020;font-weight:650}.cc-tabs button span{margin-left:6px;background:#f2e3df;color:#963e2e;padding:1px 6px;border-radius:10px;font-size:11px}.cc-panel{min-width:0}.cc-panel-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin:0 0 16px}.cc-panel-heading h3{margin:0 0 6px;font-size:18px;font-weight:650}.cc-panel-heading p{margin:0;color:#777;font-size:12px;line-height:1.6}.cc-count{white-space:nowrap;color:#777;font-size:12px}.cc-filters{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:14px}.cc-filters input{flex:1;min-width:200px}.cc-filters input,.cc-filters select,.cc-form-grid input,.cc-form-grid select,.cc-form-grid textarea{font:inherit;font-size:13px;color:#252525;border:1px solid #dedede;border-radius:6px;background:#fff;padding:9px 10px;min-width:0;width:100%;outline-offset:2px}.cc-filters select{width:auto;min-width:120px}.cc-table-wrap{overflow:auto;border-top:1px solid #e6e6e6}.cc-table{width:100%;border-collapse:collapse;font-size:12px;text-align:left}.cc-table th{font-weight:600;color:#777;background:#fafafa;white-space:nowrap}.cc-table th,.cc-table td{padding:12px 10px;border-bottom:1px solid #ececec;vertical-align:top}.cc-table td{color:#444}.cc-table .asset-name{min-width:260px;max-width:440px}.cc-table .asset-name strong,.cc-table td strong{display:block;font-weight:600;color:#222;overflow-wrap:anywhere}.cc-table small{display:block;color:#858585;font-size:11px;margin-top:5px;overflow-wrap:anywhere}.cc-table a{display:inline-block;margin-top:5px;color:#555;text-decoration:underline;text-underline-offset:2px;overflow-wrap:anywhere}.cc-risk{display:inline-block;border-radius:4px;padding:3px 6px;font-size:11px;background:#f0f0f0;color:#555}.cc-risk.high,.cc-risk.critical{background:#f9e6e2;color:#9d3425}.cc-risk.medium{background:#fbf1dc;color:#86600d}.cc-risk.low{background:#e9f3e9;color:#37663a}.cc-text-button{border:0;background:transparent;color:#222;text-decoration:underline;text-underline-offset:3px;white-space:nowrap;font-size:12px;cursor:pointer;padding:2px}.cc-empty{text-align:center!important;color:#888!important;padding:28px!important}.cc-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:13px 0;color:#777;font-size:12px}.cc-pagination div{display:flex;gap:6px}.cc-backdrop{position:fixed;z-index:1000;inset:0;background:#15151570;display:flex;align-items:center;justify-content:center;padding:20px}.cc-modal{background:#fff;width:min(720px,100%);max-height:calc(100vh - 40px);overflow:auto;border-radius:12px;box-shadow:0 20px 70px #0002}.cc-modal.wide{width:min(980px,100%)}.cc-modal>header{position:sticky;top:0;background:#fff;z-index:2;display:flex;justify-content:space-between;gap:20px;align-items:flex-start;padding:20px 24px;border-bottom:1px solid #eee}.cc-modal h3{margin:0 0 5px;font-size:19px;font-weight:650;overflow-wrap:anywhere}.cc-modal header small{color:#888;font-size:11px;overflow-wrap:anywhere}.cc-close{width:32px;height:32px;border:0;border-radius:6px;background:#f5f5f5;color:#444;font-size:24px;line-height:1;cursor:pointer;flex-shrink:0}.cc-modal-content{padding:22px 24px 28px}.cc-modal-content h4{font-size:14px;margin:24px 0 12px;font-weight:650}.cc-detail-summary{display:grid;grid-template-columns:90px minmax(0,1fr);gap:7px 12px;padding:12px;background:#f8f8f8;border-radius:7px;font-size:11px}.cc-detail-summary span{color:#777}.cc-detail-summary code{overflow-wrap:anywhere;white-space:normal;font-family:ui-monospace,monospace}.cc-image-preview{margin-top:14px;padding:12px;background:#f8f8f8;border-radius:7px}.cc-image-preview img{display:block;max-width:100%;max-height:320px;object-fit:contain;margin:0 auto}.cc-image-preview p{margin:9px 0 0;color:#777;font-size:11px}.cc-form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:13px;margin-top:16px}.cc-form-grid label{display:flex;flex-direction:column;gap:6px;font-size:12px;color:#555;min-width:0}.cc-form-grid label.full{grid-column:1/-1}.cc-form-grid textarea{resize:vertical;line-height:1.6}.cc-form-grid .cc-check{display:flex;flex-direction:row;align-items:center}.cc-form-grid .cc-check input{width:auto}.cc-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:12px}.cc-history{list-style:none;padding:0;margin:0;display:grid;gap:8px}.cc-history li{border:1px solid #ececec;border-radius:7px;padding:11px;font-size:12px}.cc-history strong{display:block}.cc-history span,.cc-history small{display:block;color:#777;margin-top:5px;overflow-wrap:anywhere}.cc-history p{margin:7px 0;color:#555;line-height:1.6;overflow-wrap:anywhere}.cc-modal-content p{font-size:13px;line-height:1.65;color:#555;overflow-wrap:anywhere}.cc-modal-content a{color:#333;overflow-wrap:anywhere}.cc-license-text{max-height:280px;overflow:auto;padding:12px;background:#f8f8f8;border:1px solid #e7e7e7;border-radius:7px;white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.6 ui-monospace,monospace;color:#333}.cc-audit-json{max-width:520px;max-height:280px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.5 ui-monospace,monospace}.cc-license-text-block{margin-bottom:18px}.cc-license-text-block h4{margin-top:14px}
@media(max-width:850px){.cc-metrics{grid-template-columns:repeat(2,minmax(0,1fr))}.compliance-heading{flex-direction:column}.compliance-heading-actions{width:100%}.cc-table{min-width:800px}}
@media(max-width:560px){.cc-metrics{grid-template-columns:1fr 1fr;gap:7px}.cc-metrics article{padding:12px}.cc-metrics article strong{font-size:23px}.cc-form-grid{grid-template-columns:1fr}.cc-form-grid label.full{grid-column:auto}.cc-modal-content{padding:16px}.cc-modal>header{padding:16px}.cc-backdrop{padding:8px}.cc-modal{max-height:calc(100vh - 16px)}}
</style>
