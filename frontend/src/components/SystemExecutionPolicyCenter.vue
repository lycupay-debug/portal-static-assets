<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

type Mode = 'developer' | 'stable'
type PolicyResponse = { mode: Mode; updatedAt: string; changedBy: string; enforcement: 'policy_source_only' | 'executor_connected' }
const policy = ref<PolicyResponse | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const selectedMode = ref<Mode>('stable')
const isDirty = computed(() => policy.value !== null && selectedMode.value !== policy.value.mode)

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api${path}`, {
    ...init, credentials: 'same-origin', cache: 'no-store',
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body?.error || `请求失败（${response.status}）`)
  return body as T
}
async function loadPolicy() {
  loading.value = true; error.value = ''
  try {
    policy.value = await request<PolicyResponse>('/system-ops/policy')
    selectedMode.value = policy.value.mode
  } catch (e) { error.value = e instanceof Error ? e.message : '读取运行策略失败' }
  finally { loading.value = false }
}
async function saveMode() {
  if (!isDirty.value || saving.value) return
  saving.value = true; error.value = ''; notice.value = ''
  try {
    policy.value = await request<PolicyResponse>('/system-ops/policy', { method: 'PUT', body: JSON.stringify({ mode: selectedMode.value }) })
    selectedMode.value = policy.value.mode
    notice.value = '运行模式已保存，后端已记录本次策略变更。'
  } catch (e) { error.value = e instanceof Error ? e.message : '保存运行策略失败'; await loadPolicy() }
  finally { saving.value = false }
}
function formatTime(value?: string) {
  if (!value) return '尚无变更记录'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
onMounted(() => { void loadPolicy() })
</script>

<template>
  <section class="execution-policy">
    <header class="policy-heading">
      <div><p class="policy-eyebrow">SYSTEM OPERATIONS / POLICY</p><h1 class="account-page-title">运行模式与任务策略</h1><p class="policy-intro">统一管理服务器任务的默认执行策略。此处是独立的系统运维页面，不影响系统警告和系统日志页面。</p></div>
      <button class="policy-button" type="button" :disabled="loading" @click="loadPolicy">刷新状态</button>
    </header>

    <p v-if="error" class="policy-message is-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="policy-message is-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="policy-loading">正在读取服务器运行策略…</div>
    <template v-else-if="policy">
      <section class="policy-current" aria-live="polite">
        <div><span class="policy-kicker">当前持久化模式</span><strong>{{ policy.mode === 'developer' ? '开发者模式' : '稳定运行模式' }}</strong><p>{{ policy.mode === 'developer' ? '以开发效率为优先，在既定的用户授权范围内减少逐项审批。' : '以线上稳定性为优先，按任务风险、路径范围和审批要求执行。' }}</p></div>
        <span class="policy-state-dot" :class="{ 'is-developer': policy.mode === 'developer' }" aria-hidden="true"></span>
      </section>

      <section class="policy-mode-switch-panel" aria-label="运行模式开关">
        <div class="policy-mode-switch-copy"><span class="policy-kicker">切换运行模式</span><strong>{{ selectedMode === 'developer' ? '开发者模式' : '稳定运行模式' }}</strong><p>{{ selectedMode === 'developer' ? '优先开发效率：在明确授权范围内自主连续执行开发、构建、测试和修复。' : '优先运行稳定：按任务风险限制权限，敏感操作需要逐次审批。' }}</p></div>
        <div class="policy-switch-control"><span :class="{ active: selectedMode === 'stable' }">稳定运行</span><button class="policy-switch" type="button" role="switch" :aria-checked="selectedMode === 'developer'" :aria-label="selectedMode === 'developer' ? '切换到稳定运行模式' : '切换到开发者模式'" @click="selectedMode = selectedMode === 'developer' ? 'stable' : 'developer'"><span></span></button><span :class="{ active: selectedMode === 'developer' }">开发者</span></div>
      </section>
      <div class="policy-mode-grid">
        <article class="policy-mode-card" :class="{ selected: selectedMode === 'developer' }"><strong>开发者模式</strong><span class="policy-mode-tag">开发阶段</span><p>在用户授权的开发任务范围内，允许自主规划、连续修改、构建、测试、修复和验证，减少重复确认。</p></article>
        <article class="policy-mode-card" :class="{ selected: selectedMode === 'stable' }"><strong>稳定运行模式</strong><span class="policy-mode-tag">生产阶段</span><p>只读任务可自动执行；文件、测试、构建和代码修改受范围约束；部署、重启、数据库变更及破坏性操作按规则审批。</p></article>
      </div>

      <section class="policy-rules">
        <div class="policy-section-heading"><div><h2>强制确认规则</h2><p>无论选择哪种模式，以下规则都不能被模式开关解除。</p></div><span>始终生效</span></div>
        <div class="policy-rule-row"><span class="policy-rule-icon">01</span><div><strong>重启任何服务或进程</strong><p>必须事先取得用户同意。SSH_MCP 服务由用户本人执行重启。</p></div><span class="policy-lock">需确认</span></div>
        <div class="policy-rule-row"><span class="policy-rule-icon">02</span><div><strong>处理 SSH_MCP 本身</strong><p>涉及 SSH_MCP 的配置、授权、连接机制、服务或进程的变更，必须先征得用户同意。</p></div><span class="policy-lock">需确认</span></div>
      </section>

      <section class="policy-rules">
        <div class="policy-section-heading"><div><h2>任务分类规则</h2><p>策略定义用于统一后端判定；稳定运行模式不会把高风险操作自动降级为普通任务。</p></div></div>
        <div class="policy-task-list">
          <div><strong>只读诊断与文件检查</strong><span>开发者模式：授权范围内自动 · 稳定模式：只读权限与路径范围</span></div>
          <div><strong>测试与构建</strong><span>开发者模式：自主执行并修复 · 稳定模式：登记测试任务、限定构建工作区</span></div>
          <div><strong>代码修改与系统管理</strong><span>开发者模式：按本次开发授权执行 · 稳定模式：限定文件、资源和操作范围</span></div>
          <div><strong>部署、数据库变更、破坏性操作</strong><span>开发者模式：除强制确认项外按明确授权执行 · 稳定模式：逐次审批</span></div>
        </div>
      </section>

      <aside class="policy-boundary"><strong>执行边界说明</strong><p>当前系统已提供持久化策略与后端权限校验入口。现有事件处理器尚未接入通用命令执行器，因此切换模式不会自行启动 Shell 命令或替代 SSH_MCP；后续执行器必须调用统一策略判定，并继续遵守上述强制确认规则。</p></aside>

      <footer class="policy-footer"><div><span>最近变更</span><strong>{{ formatTime(policy.updatedAt) }}</strong><small>操作者：{{ policy.changedBy || '系统初始化' }}</small></div><div><span>策略保存状态</span><strong>{{ policy.enforcement === 'executor_connected' ? '执行器已接入' : '已持久化 · 等待执行器接入' }}</strong><small>当前模式：{{ policy.mode === 'developer' ? '开发者模式' : '稳定运行模式' }}</small></div></footer>
      <div class="policy-actions"><span v-if="isDirty">有尚未保存的模式变更。</span><span v-else>当前选择与服务器保存的模式一致。</span><div><button class="policy-button" type="button" :disabled="saving || !isDirty" @click="selectedMode = policy!.mode">取消更改</button><button class="policy-button is-primary" type="button" :disabled="saving || !isDirty" @click="saveMode">{{ saving ? '正在保存…' : '保存运行模式' }}</button></div></div>
    </template>
  </section>
</template>

<style scoped>
.execution-policy{min-width:0;color:#202020}.policy-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin:0 0 25px}.policy-eyebrow{margin:0 0 7px;color:#777;font-size:11px;font-weight:650;letter-spacing:.12em}.policy-intro{max-width:760px;margin:0;color:#686868;font-size:14px;line-height:1.65}.policy-button{border:1px solid #d8d8d8;border-radius:7px;background:#fff;padding:9px 13px;color:#222;font-size:13px;white-space:nowrap;cursor:pointer}.policy-button:disabled{opacity:.45;cursor:not-allowed}.policy-button.is-primary{background:#202020;color:#fff;border-color:#202020}.policy-message{margin:0 0 16px;padding:11px 13px;border-radius:6px;font-size:13px}.policy-message.is-error{background:#fff0f0;color:#a32121}.policy-message.is-success{background:#f0f7f1;color:#28633b}.policy-loading{padding:35px 0;color:#777;font-size:13px}.policy-current{display:flex;justify-content:space-between;align-items:center;gap:20px;padding:20px 0;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7;margin-bottom:22px}.policy-current>div{min-width:0}.policy-kicker,.policy-current p{display:block;color:#777;font-size:12px}.policy-current strong{display:block;margin:7px 0;font-size:22px;letter-spacing:-.03em}.policy-current p{margin:0;line-height:1.6}.policy-state-dot{width:10px;height:10px;flex:0 0 auto;border-radius:50%;background:#647b65;box-shadow:0 0 0 5px #647b651a}.policy-state-dot.is-developer{background:#3f6e9d;box-shadow:0 0 0 5px #3f6e9d1a}.policy-mode-switch-panel{display:flex;justify-content:space-between;align-items:center;gap:24px;padding:19px 0;border-bottom:1px solid #e7e7e7;margin-bottom:17px}.policy-mode-switch-copy{min-width:0}.policy-mode-switch-copy>strong{display:block;margin:7px 0;font-size:18px}.policy-mode-switch-copy p{max-width:680px;margin:0;color:#777;font-size:12px;line-height:1.7}.policy-switch-control{display:flex;align-items:center;gap:10px;flex:0 0 auto}.policy-switch-control>span{font-size:12px;color:#999;white-space:nowrap}.policy-switch-control>span.active{color:#222;font-weight:650}.policy-switch{display:flex;align-items:center;width:48px;height:28px;padding:3px;border:0;border-radius:999px;background:#c8c8c8;cursor:pointer;transition:background .16s}.policy-switch[aria-checked="true"]{background:#242424}.policy-switch>span{display:block;width:22px;height:22px;border-radius:50%;background:#fff;box-shadow:0 1px 3px #0002;transition:transform .16s}.policy-switch[aria-checked="true"]>span{transform:translateX(20px)}.policy-mode-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;margin-bottom:34px}.policy-mode-card{display:flex;flex-direction:column;align-items:flex-start;gap:10px;padding:17px;border:1px solid #e2e2e2;border-radius:9px;background:rgba(255,255,255,.5)}.policy-mode-card.selected{border-color:#777;background:rgba(245,245,245,.7)}.policy-mode-card>strong{font-size:14px}.policy-mode-card p{margin:0;color:#666;font-size:12px;line-height:1.7}.policy-mode-tag{display:inline-flex;padding:4px 7px;border-radius:4px;background:#f0f0f0;color:#555;font-size:11px}.policy-rules{margin:0 0 30px}.policy-section-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin-bottom:10px}.policy-section-heading h2{margin:0 0 5px;font-size:17px}.policy-section-heading p{margin:0;color:#777;font-size:12px;line-height:1.6}.policy-section-heading>span{font-size:11px;color:#555;white-space:nowrap}.policy-rule-row{display:grid;grid-template-columns:35px minmax(0,1fr) auto;align-items:start;gap:12px;padding:15px 0;border-top:1px solid #e8e8e8}.policy-rule-icon{display:flex;width:28px;height:28px;align-items:center;justify-content:center;border-radius:6px;background:#f1f1f1;color:#555;font-size:11px}.policy-rule-row strong,.policy-task-list strong{font-size:13px}.policy-rule-row p{margin:5px 0 0;color:#777;font-size:12px;line-height:1.6}.policy-lock{font-size:11px;color:#7b4b14;background:#fbf2e4;padding:5px 7px;border-radius:4px;white-space:nowrap}.policy-task-list{border-top:1px solid #e7e7e7}.policy-task-list>div{display:flex;justify-content:space-between;gap:18px;padding:13px 0;border-bottom:1px solid #ededed}.policy-task-list span{max-width:65%;color:#777;font-size:12px;line-height:1.6;text-align:right}.policy-boundary{margin:4px 0 25px;padding:14px 15px;border-left:2px solid #aaa;background:rgba(245,245,245,.55)}.policy-boundary strong{font-size:12px}.policy-boundary p{margin:6px 0 0;color:#666;font-size:12px;line-height:1.75}.policy-footer{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px;padding:18px 0;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7}.policy-footer>div{display:flex;flex-direction:column;gap:5px}.policy-footer span,.policy-footer small{color:#777;font-size:11px}.policy-footer strong{font-size:13px;font-weight:600}.policy-actions{display:flex;justify-content:space-between;align-items:center;gap:14px;padding:18px 0}.policy-actions>span{color:#777;font-size:12px}.policy-actions>div{display:flex;gap:8px}@media(max-width:760px){.policy-heading{flex-direction:column}.policy-mode-switch-panel{align-items:flex-start;flex-direction:column}.policy-mode-grid,.policy-footer{grid-template-columns:1fr}.policy-task-list>div{flex-direction:column;gap:5px}.policy-task-list span{max-width:none;text-align:left}.policy-actions{align-items:flex-start;flex-direction:column}.policy-rule-row{grid-template-columns:30px minmax(0,1fr)}.policy-lock{grid-column:2;justify-self:start}}
</style>
