/**
 * Responsibility: render and operate the compliance center with standard DOM APIs.
 * Dependencies: the framework-independent compliance controller and its typed state/actions.
 * Collaboration: App.vue owns this mount's lifetime and supplies the authenticated admin flag; Go remains the
 * authority for every permission check and mutation. The controller owns API calls and request sequencing.
 * Boundaries: no Vue/runtime dependency; all server-provided values are escaped before insertion into fixed
 * markup, URLs are protocol-checked, and writes occur only from explicit user actions.
 * Cleanup: dispose removes delegated listeners, invalidates controller requests, and clears the host.
 */
import { createComplianceCenterController, type ComplianceState, type ComplianceTab } from './complianceCenterController'

const tabs: Array<{ id: ComplianceTab; label: string }> = [
  { id: 'assets', label: '门户资源台账' }, { id: 'licenses', label: '软件许可证目录' },
  { id: 'documents', label: '内部政策文件' }, { id: 'scans', label: '开发资源扫描' },
  { id: 'audit', label: '操作日志' }, { id: 'reviewers', label: '审核人员权限' },
]
const escapeHtml = (value: unknown): string => String(value ?? '—').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!)
const dateText = (value?: string | null): string => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { hour12: false })
}
const riskText = (value: string): string => ({ critical: '严重', high: '高', medium: '中', low: '低', unknown: '未知' }[value] || value || '未知')
const statusText = (value: string): string => ({ pending: '待审核', in_review: '审核中', approved: '已通过', approved_with_conditions: '附条件通过', replace_or_remove: '建议替换/下架', blocked: '已阻止', unreviewed: '未审核', reviewed: '已审核', restricted: '受限', needs_counsel: '需法律审核', draft: '草稿', in_review_doc: '审核中', published: '已发布', archived: '已归档' }[value] || value || '—')
const cell = (value: unknown): string => `<td>${escapeHtml(value)}</td>`
const button = (action: string, label: string, extra = ''): string => `<button type="button" class="cc-text-button" data-action="${action}" ${extra}>${escapeHtml(label)}</button>`
const field = (label: string, name: string, value: unknown, type = 'text', options?: Array<[string, string]>): string => {
  const control = options
    ? `<select name="${name}">${options.map(([v, text]) => `<option value="${escapeHtml(v)}" ${String(value) === v ? 'selected' : ''}>${escapeHtml(text)}</option>`).join('')}</select>`
    : type === 'textarea'
      ? `<textarea name="${name}" rows="3">${escapeHtml(value)}</textarea>`
      : `<input name="${name}" type="${type}" value="${escapeHtml(value)}">`
  return `<label>${escapeHtml(label)}${control}</label>`
}
const selectOptions = (values: string[], labels: string[]): Array<[string, string]> => values.map((value, index) => [value, labels[index] || value])

export function mountComplianceCenter(host: HTMLElement, isAdmin: boolean): { dispose: () => void } {
  const controller = createComplianceCenterController({ isAdmin })
  const { state } = controller
  let disposed = false

  const renderAssets = (): string => `<section class="cc-panel"><div class="cc-panel-heading"><div><h3>门户开发资源台账</h3><p>更新审核状态不会自动更改线上内容。</p></div><span class="cc-count">${state.assetsTotal} 条记录</span></div>
    <form class="cc-filters" data-form="filters"><input name="assetQuery" type="search" placeholder="搜索名称、路径或来源 URL" value="${escapeHtml(state.assetQuery)}"><select name="assetType">${[['','全部类型'],['software_dependency','软件依赖'],['icon','图标'],['image','图片'],['font','字体'],['audio','音频'],['source_file','源文件']].map(([v,t])=>`<option value="${v}" ${state.assetType===v?'selected':''}>${t}</option>`).join('')}</select><select name="assetRisk">${[['','全部风险'],['critical','严重'],['high','高'],['medium','中'],['low','低'],['unknown','未知']].map(([v,t])=>`<option value="${v}" ${state.assetRisk===v?'selected':''}>${t}</option>`).join('')}</select><select name="assetStatus">${[['','全部状态'],['pending','待审核'],['in_review','审核中'],['approved','已通过'],['approved_with_conditions','附条件通过'],['replace_or_remove','建议替换/下架'],['blocked','已阻止']].map(([v,t])=>`<option value="${v}" ${state.assetStatus===v?'selected':''}>${t}</option>`).join('')}</select><button class="cc-button secondary">筛选</button></form>
    <div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>资产</th><th>类型</th><th>授权状态</th><th>风险</th><th>审核状态</th><th>公开/分发</th><th>操作</th></tr></thead><tbody>${state.assets.map(item=>`<tr><td class="asset-name"><strong>${escapeHtml(item.name)}</strong><small>${escapeHtml(item.path)}</small>${safeLink(item.sourceUrl)}</td>${cell(item.type)}${cell(item.permissionStatus)}<td><span class="cc-risk ${escapeHtml(item.riskLevel)}">${escapeHtml(riskText(item.riskLevel))}</span></td>${cell(statusText(item.reviewStatus))}${cell(item.active?item.distributionStatus:'非活动/源文件')}<td>${button('asset', '审核详情', `data-id="${escapeHtml(item.id)}"`)}</td></tr>`).join('') || `<tr><td colspan="7" class="cc-empty">${state.loading?'正在读取…':'没有符合条件的资产。'}</td></tr>`}</tbody></table></div>
    <div class="cc-pagination"><span>显示 ${state.assets.length} / ${state.assetsTotal} 条</span><div><button class="cc-button secondary" data-action="prev" ${state.assetOffset<=0?'disabled':''}>上一页</button><button class="cc-button secondary" data-action="next" ${state.assetOffset+state.assets.length>=state.assetsTotal?'disabled':''}>下一页</button></div></div></section>`
  const safeLink = (value?: string): string => {
    if (!value) return ''
    try { const url = new URL(value, document.baseURI); return ['http:', 'https:'].includes(url.protocol) ? `<a href="${escapeHtml(url.href)}" target="_blank" rel="noopener noreferrer">查看来源 ↗</a>` : '' } catch { return '' }
  }
  const renderTable = (title: string, description: string, count: number, headers: string[], rows: string): string => `<section class="cc-panel"><div class="cc-panel-heading"><div><h3>${escapeHtml(title)}</h3><p>${escapeHtml(description)}</p></div><span class="cc-count">${count} 条</span></div><div class="cc-table-wrap"><table class="cc-table"><thead><tr>${headers.map(h=>`<th>${escapeHtml(h)}</th>`).join('')}</tr></thead><tbody>${rows || `<tr><td colspan="${headers.length}" class="cc-empty">暂无记录</td></tr>`}</tbody></table></div></section>`
  const renderPanel = (): string => {
    if (state.activeTab === 'assets') return renderAssets()
    if (state.activeTab === 'licenses') return renderTable('许可证目录','来源包括 package-lock、包内许可证文件及第三方声明。',state.licenses.length,['许可证','类型','商业使用标记','文本收录','审核',''],state.licenses.map(item=>`<tr><td><strong>${escapeHtml(item.name)}</strong>${item.canonicalUrl?`<small>${safeLink(item.canonicalUrl)}</small>`:''}</td>${cell(item.family)}${cell(item.commercialUseAllowed===true?'元数据标记允许':item.commercialUseAllowed===false?'元数据标记受限':'未判定')}${cell(item.licenseTextCaptured?'已收录文本':'缺少文本')}${cell(statusText(item.reviewStatus))}<td>${button('license','复核',`data-id="${escapeHtml(item.id)}"`)}</td></tr>`).join(''))
    if (state.activeTab === 'documents') return renderTable('政策文件','缺少主体、联系渠道或实际数据处理事实时，请保持草稿。',state.documents.length,['政策','辖区','版本','状态','缺失事实',''],state.documents.map(item=>`<tr><td><strong>${escapeHtml(item.title)}</strong><small>/legal/${escapeHtml(item.slug)}</small></td>${cell(item.jurisdiction)}${cell(`v${item.version}`)}${cell(statusText(item.status))}${cell((item.missingInformation||[]).length)}<td>${button('document','编辑/审核',`data-id="${escapeHtml(item.id)}"`)}</td></tr>`).join(''))
    if (state.activeTab === 'scans') return renderTable('扫描记录','保留每次扫描的范围、版本、计数和报告位置。',state.scans.length,['类型','状态','资产数','发现数','版本','开始时间','报告'],state.scans.map(item=>`<tr>${cell(item.scanType)}${cell(item.status)}${cell(item.assetsSeen)}${cell(item.findingsCount)}${cell(item.sourceRevision)}${cell(dateText(item.startedAt))}${cell(item.reportPath)}</tr>`).join(''))
    if (state.activeTab === 'audit') return renderTable('操作审计日志','记录资产、证据、审核、许可证和内部政策变更。',state.auditLogs.length,['时间','操作者','动作','对象','变更快照'],state.auditLogs.map(item=>`<tr>${cell(dateText(item.createdAt))}${cell(item.actor)}${cell(item.action)}<td><strong>${escapeHtml(item.objectType)}</strong><small>${escapeHtml(item.objectId)}</small></td><td><details><summary>查看前后状态</summary><pre class="cc-audit-json">前：${escapeHtml(JSON.stringify(item.beforeState,null,2))}\n后：${escapeHtml(JSON.stringify(item.afterState,null,2))}</pre></details></td></tr>`).join(''))
    return renderTable('内部合规审核人员权限','只有超管可以分配或撤销；服务端执行最终授权。',state.reviewerUsers.length,['账户','账户权限','审核专员','创建时间'],state.reviewerUsers.map(item=>`<tr><td><strong>${escapeHtml(item.displayName||'未设置昵称')}</strong><small>${escapeHtml(item.email)}</small></td>${cell(item.isAdmin?'超管':'普通账户')}<td><label class="cc-check"><input type="checkbox" data-action="reviewer" data-id="${escapeHtml(item.id)}" ${item.isCopyrightReviewer?'checked':''} ${item.isAdmin||state.reviewerUpdateBusy===item.id?'disabled':''}><span>${item.isAdmin?'超管权限':item.isCopyrightReviewer?'已授权':'未授权'}</span></label></td>${cell(dateText(item.createdAt))}</tr>`).join(''))
  }
  const renderAssetModal = (): string => {
    const item=state.selectedAsset, f=state.assetForm, r=state.reviewForm, e=state.evidenceForm
    if (!item) return ''
    return `<div class="cc-backdrop" data-action="backdrop"><section class="cc-modal wide" role="dialog" aria-modal="true" aria-label="资产审核详情"><header><div><p class="compliance-eyebrow">ASSET REVIEW</p><h3>${escapeHtml(item.asset_name || item.name)}</h3><small>${escapeHtml(item.asset_path || item.path)}</small></div><button class="cc-close" data-action="close" aria-label="关闭">×</button></header><div class="cc-modal-content"><div class="cc-detail-summary"><span>资产 ID</span><code>${escapeHtml(item.id)}</code><span>来源</span><code>${escapeHtml(item.source_url)}</code></div><form data-form="asset" class="cc-form-grid">${field('审核状态','reviewStatus',f.reviewStatus,'',selectOptions(['pending','in_review','approved','approved_with_conditions','replace_or_remove','blocked'],['待审核','审核中','已通过','附条件通过','建议替换/下架','阻止使用']))}${field('风险等级','riskLevel',f.riskLevel,'',selectOptions(['critical','high','medium','low','unknown'],['严重','高','中','低','未知']))}${field('权属依据','ownershipBasis',f.ownershipBasis)}${field('授权状态','permissionStatus',f.permissionStatus)}${field('使用范围','useScope',f.useScope)}${field('分发状态','distributionStatus',f.distributionStatus)}${field('来源 URL','sourceUrl',f.sourceUrl)}${field('权利人','rightsholder',f.rightsholder)}${field('许可证 ID','licenseId',f.licenseId)}${field('备注','notes',f.notes,'textarea')}<label class="cc-check"><input name="active" type="checkbox" ${f.active?'checked':''}> 当前公开/活动</label><div class="cc-actions"><button class="cc-button primary" ${state.saving?'disabled':''}>保存资产台账</button></div></form>
    <h4>新增证据</h4><form data-form="evidence" class="cc-form-grid">${field('证据类型','type',e.type)}${field('证据位置','uri',e.uri)}${field('证据内容','text',e.text,'textarea')}${field('备注','notes',e.notes,'textarea')}<div class="cc-actions"><button class="cc-button secondary" ${state.saving?'disabled':''}>保存证据</button></div></form>
    <h4>新增审核记录</h4><form data-form="review" class="cc-form-grid">${field('审核类型','type',r.type,'',selectOptions(['license','content','distribution','legal_basis','recheck'],['许可证','内容','分发','法律依据','复查']))}${field('审核结论','decision',r.decision,'',selectOptions(['pending_evidence','approved','approved_with_conditions','replace','remove','blocked','escalate'],['待补证据','通过','附条件通过','建议替换','建议下架','阻止使用','升级法律审核']))}${field('风险等级','riskLevel',r.riskLevel,'',selectOptions(['critical','high','medium','low','unknown'],['严重','高','中','低','未知']))}${field('判断依据','findings',r.findings,'textarea')}${field('后续动作（每行一项）','actions',r.actions,'textarea')}<div class="cc-actions"><button class="cc-button primary" ${state.saving?'disabled':''}>保存审核记录</button></div></form>
    <h4>证据历史</h4><ul class="cc-history">${state.selectedEvidence.map(ev=>`<li><strong>${escapeHtml(ev.type)}</strong><span>${escapeHtml(ev.uri||'未提供位置')}</span><p>${escapeHtml(ev.text||ev.notes)}</p><small>${escapeHtml(dateText(ev.capturedAt))} · ${ev.verified?'已核验':'未核验'}</small><button class="cc-button secondary" data-action="verify" data-id="${escapeHtml(ev.id)}" data-value="${!ev.verified}" ${state.saving?'disabled':''}>${ev.verified?'撤销核验':'核验证据'}</button></li>`).join('')||'<li>暂无证据记录</li>'}</ul><h4>历史审核</h4><ul class="cc-history">${state.selectedReviews.map(rv=>`<li><strong>${escapeHtml(rv.decision)} · ${escapeHtml(riskText(rv.riskLevel))}</strong><p>${escapeHtml(rv.findings)}</p><small>${escapeHtml(rv.reviewer)} · ${escapeHtml(dateText(rv.reviewedAt))}</small></li>`).join('')||'<li>暂无审核记录</li>'}</ul></div></section></div>`
  }
  const renderLicenseModal = (): string => {
    const item=state.selectedLicense,f=state.licenseForm;if(!item)return ''
    return `<div class="cc-backdrop" data-action="backdrop"><section class="cc-modal" role="dialog" aria-modal="true" aria-label="许可证审核"><header><div><p class="compliance-eyebrow">LICENSE REVIEW</p><h3>${escapeHtml(item.name)}</h3><small>${escapeHtml(item.spdxId)} ${escapeHtml(item.version)}</small></div><button class="cc-close" data-action="close" aria-label="关闭">×</button></header><div class="cc-modal-content"><h4>收录的许可证原文</h4><pre class="cc-license-text">${escapeHtml(item.fullText||'当前没有捕获到许可证全文。')}</pre><h4>关联资产与依赖实例</h4><div class="cc-table-wrap"><table class="cc-table"><thead><tr><th>资产</th><th>类型</th><th>授权状态</th><th>审核</th><th>风险</th></tr></thead><tbody>${(item.linkedAssets||[]).map((a:any)=>`<tr><td>${escapeHtml(a.name)}<small>${escapeHtml(a.path)}</small></td>${cell(a.assetType)}${cell(a.permissionStatus)}${cell(statusText(a.reviewStatus))}${cell(riskText(a.riskLevel))}</tr>`).join('')||'<tr><td colspan="5">没有直接关联的资产记录</td></tr>'}</tbody></table></div><form data-form="license" class="cc-form-grid">${field('审核状态','reviewStatus',f.reviewStatus,'',selectOptions(['unreviewed','reviewed','restricted','needs_counsel'],['未审核','已审核','受限','需法律审核']))}${field('许可证类型','family',f.family,'',selectOptions(['permissive','copyleft','weak_copyleft','proprietary','public_domain','custom','other'],['宽松型','强 Copyleft','弱 Copyleft','专有','公有领域','自定义','其他/组合许可']))}${field('复核备注','notes',f.notes,'textarea')}${['commercialUseAllowed','modificationAllowed','redistributionAllowed','attributionRequired','sourceCodeOfferRequired'].map((key,i)=>`<label class="cc-check"><input type="checkbox" name="${key}" ${f[key]?'checked':''}>${['允许商业使用','允许修改','允许再分发','需要署名/保留声明','需要提供对应源代码'][i]}</label>`).join('')}<div class="cc-actions"><button class="cc-button primary">保存许可证审核</button></div></form></div></section></div>`
  }
  const renderDocumentModal = (): string => {
    const item=state.selectedDocument,f=state.documentForm;if(!item)return ''
    return `<div class="cc-backdrop" data-action="backdrop"><section class="cc-modal wide" role="dialog" aria-modal="true" aria-label="政策文件编辑"><header><div><p class="compliance-eyebrow">LEGAL DOCUMENT DRAFT</p><h3>${escapeHtml(item.title)}</h3><small>${escapeHtml(item.slug)} · 当前版本 v${escapeHtml(item.version)}</small></div><button class="cc-close" data-action="close" aria-label="关闭">×</button></header><div class="cc-modal-content"><p class="cc-alert warning">政策发布会公开面向用户。运营主体、联系渠道、数据处理事实或法律审核未完成时，请保持草稿/审核中。</p><form data-form="document" class="cc-form-grid">${field('状态','status',f.status,'',selectOptions(['draft','in_review','published','archived'],['草稿','审核中','发布','归档']))}${field('审核人/批准人','approvedBy',f.approvedBy)}${field('法律审核时间','legalReviewAt',f.legalReviewAt,'datetime-local')}${field('生效时间','effectiveAt',f.effectiveAt,'datetime-local')}${field('正文 Markdown','bodyMarkdown',f.bodyMarkdown,'textarea')}${field('尚缺事实（每行一项）','missingInformation',f.missingInformation,'textarea')}${field('事实假设与核验说明（每行一项）','factualAssumptions',f.factualAssumptions,'textarea')}${field('版本变更说明','changeNote',f.changeNote)}<div class="cc-actions"><button class="cc-button secondary" data-save-draft="true">保存为草稿</button><button class="cc-button primary">保存/提交状态</button></div></form></div></section></div>`
  }
  const render = (): void => {
    if (disposed) return
    const visibleTabs=tabs.filter(tab=>isAdmin||tab.id!=='reviewers')
    host.innerHTML=`<section class="compliance-center"><div class="compliance-heading"><div><p class="compliance-eyebrow">RIGHTS & POLICY GOVERNANCE</p><h2>门户开发资源合规中心</h2><p>资产盘点、许可证证据、审核记录和网站政策草稿。自动扫描结果不等于法律审核结论。</p></div><div class="compliance-heading-actions"><button class="cc-button secondary" data-action="refresh" ${state.loading?'disabled':''}>${state.loading?'正在刷新…':'刷新台账'}</button><a class="cc-button primary" href="${controller.exportCsvUrl}" target="_blank" rel="noopener noreferrer">导出台账 CSV</a></div></div>${state.error?`<p class="cc-alert error" role="alert">${escapeHtml(state.error)}</p>`:''}${state.feedback?`<p class="cc-alert success" role="status">${escapeHtml(state.feedback)}</p>`:''}${state.permissionDenied?'<div class="cc-permission"><strong>当前账户尚未获得内部资源合规管理权限。</strong><p>台账数据已由服务端进行管理员权限校验。请勿通过公开注册账户访问这些数据。</p></div>':`<div class="cc-metrics"><article><span>资产记录</span><strong>${escapeHtml(state.overview.assetCount??'—')}</strong><small>门户自身文件、静态资源与软件依赖</small></article><article><span>待审核资产</span><strong>${escapeHtml(state.overview.pendingAssetCount??'—')}</strong><small>待补证据或待复核</small></article><article class="risk"><span>高风险待核验</span><strong>${escapeHtml(state.overview.highRiskCount??'—')}</strong><small>风险提示仅用于人工复核</small></article><article><span>许可证候选</span><strong>${escapeHtml(state.overview.licenseCount??'—')}</strong><small>${escapeHtml(state.overview.unreviewedLicenseCount??'—')} 条尚未人工审核</small></article></div><nav class="cc-tabs" aria-label="内部资源合规管理模块">${visibleTabs.map(tab=>`<button type="button" class="${state.activeTab===tab.id?'active':''}" data-tab="${tab.id}">${tab.label}</button>`).join('')}</nav>${renderPanel()}${renderAssetModal()}${renderLicenseModal()}${renderDocumentModal()}`}</section>`
  }
  const readForm = (form: HTMLFormElement): Record<string, any> => {
    const values: Record<string, any> = {}
    for (const [key,value] of new FormData(form)) values[key]=value
    for (const checkbox of form.querySelectorAll<HTMLInputElement>('input[type="checkbox"][name]')) values[checkbox.name]=checkbox.checked
    return values
  }
  const onSubmit = (event: Event): void => {
    const form=event.target
    if (!(form instanceof HTMLFormElement)) return
    event.preventDefault()
    const values=readForm(form)
    const kind=form.dataset.form
    void (async()=>{
      if(kind==='filters'){Object.assign(state,{assetQuery:values.assetQuery||'',assetType:values.assetType||'',assetRisk:values.assetRisk||'',assetStatus:values.assetStatus||'',assetOffset:0});await controller.loadAssets(true)}
      if(kind==='asset') {Object.assign(state.assetForm,values);await controller.saveAsset()}
      if(kind==='evidence') {Object.assign(state.evidenceForm,values);await controller.saveEvidence()}
      if(kind==='review') {Object.assign(state.reviewForm,values);await controller.saveReview()}
      if(kind==='license') {Object.assign(state.licenseForm,values);await controller.saveLicense()}
      if(kind==='document') {Object.assign(state.documentForm,values);const submitter=(event as SubmitEvent).submitter;if(submitter instanceof HTMLElement && submitter.hasAttribute('data-save-draft'))state.documentForm.status='draft';await controller.saveDocument()}
      render()
    })()
  }
  const onClick = (event: Event): void => {
    const target=event.target instanceof Element?event.target.closest<HTMLElement>('[data-action],[data-tab]'):null
    if(!target)return
    if(target.dataset.tab){state.activeTab=target.dataset.tab as ComplianceTab;render();return}
    const action=target.dataset.action
    if(action==='backdrop'&&event.target!==target)return
    if(action==='refresh')void controller.refreshAll().then(render)
    else if(action==='close'||action==='backdrop'){controller.closeSelections();render()}
    else if(action==='asset')void controller.openAsset(state.assets.find(item=>String(item.id)===target.dataset.id)!).then(render)
    else if(action==='license')void controller.openLicense(state.licenses.find(item=>String(item.id)===target.dataset.id)!).then(render)
    else if(action==='document'){const item=state.documents.find(value=>String(value.id)===target.dataset.id);if(item)controller.openDocument(item);render()}
    else if(action==='verify'){const item=state.selectedEvidence.find(value=>String(value.id)===target.dataset.id);if(item)void controller.verifyEvidence(item,target.dataset.value==='true').then(render)}
    else if(action==='prev'||action==='next'){state.assetOffset=Math.max(0,state.assetOffset+(action==='prev'?-state.assetLimit:state.assetLimit));void controller.loadAssets().then(render)}
  }
  const onKeyDown = (event: KeyboardEvent): void => {
    if (event.key === 'Escape' && (state.selectedAsset || state.selectedLicense || state.selectedDocument)) { controller.closeSelections(); render() }
  }
  const onChange = (event: Event): void => {
    const target=event.target
    if(target instanceof HTMLInputElement&&target.dataset.action==='reviewer'){
      const item=state.reviewerUsers.find(user=>String(user.id)===target.dataset.id)
      if(item)void controller.setReviewerRole(item,target.checked).then(render)
    }
  }
  host.addEventListener('submit',onSubmit)
  host.addEventListener('click',onClick)
  host.addEventListener('change',onChange)
  host.ownerDocument.addEventListener('keydown',onKeyDown)
  render()
  void controller.ready.then(render)
  return { dispose:()=>{disposed=true;host.removeEventListener('submit',onSubmit);host.removeEventListener('click',onClick);host.removeEventListener('change',onChange);host.ownerDocument.removeEventListener('keydown',onKeyDown);controller.dispose();host.replaceChildren()} }
}
