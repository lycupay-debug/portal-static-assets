<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { apiRequest } from '../api/client'

type Mode = 'developer' | 'stable'
type PolicyResponse = { mode: Mode; updatedAt: string; changedBy: string; enforcement: 'policy_source_only' | 'executor_connected' | 'executor_disconnected'; executorStatus: string; executorLastSeen?: string }
type TaskClass = 'read_only' | 'file_inspection' | 'test' | 'build' | 'code_change' | 'system_admin' | 'deploy' | 'restart' | 'database_change' | 'destructive' | 'ssh_mcp'
type CommandTask = { taskId: string; idempotencyKey: string; title: string; taskClass: TaskClass; executable: string; arguments: string[]; workingDirectory: string; timeoutSeconds: number; requestedBy: string; modeAtSubmission: Mode; status: string; approvalRequired: boolean; approvedBy?: string; createdAt: string; updatedAt: string; startedAt?: string; finishedAt?: string; exitCode?: number; stdout: string; stderr: string; attempt: number; failureReason?: string }
const policy = ref<PolicyResponse | null>(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const selectedMode = ref<Mode>('stable')
const tasks = ref<CommandTask[]>([])
const tasksLoading = ref(false)
const tasksError = ref('')
const tasksNotice = ref('')
const taskSubmitting = ref(false)
const taskDraft = ref({ title: '只读诊断：系统版本', taskClass: 'read_only' as TaskClass, executable: '/usr/bin/uname', argumentsText: '["-a"]', workingDirectory: '/opt/personal-portal', timeoutSeconds: 120 })
let taskRefreshTimer: ReturnType<typeof setInterval> | undefined
const isDirty = computed(() => policy.value !== null && selectedMode.value !== policy.value.mode)

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return apiRequest<T>(path, { cache: 'no-store', ...init })
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

async function loadTasks(silent = false) {
  if (!silent) tasksLoading.value = true
  tasksError.value = ''
  try { const result = await request<{ items: CommandTask[] }>('/system-ops/tasks'); tasks.value = result.items }
  catch (e) { tasksError.value = e instanceof Error ? e.message : '读取任务队列失败' }
  finally { tasksLoading.value = false }
}
async function createTask() {
  if (taskSubmitting.value) return
  taskSubmitting.value = true; tasksError.value = ''; tasksNotice.value = ''
  try {
    let args: unknown
    try { args = JSON.parse(taskDraft.value.argumentsText) } catch { throw new Error('参数必须是合法 JSON 数组，例如 ["-a"]') }
    if (!Array.isArray(args) || args.some(value => typeof value !== 'string')) throw new Error('参数必须是字符串数组')
    const result = await request<{ taskId: string; status: string; approvalRequired: boolean; policyDecision: string; mode: Mode }>('/system-ops/tasks', { method: 'POST', body: JSON.stringify({ idempotencyKey: `task-${Date.now()}-${Math.random().toString(36).slice(2, 12)}`, title: taskDraft.value.title, taskClass: taskDraft.value.taskClass, executable: taskDraft.value.executable, arguments: args, workingDirectory: taskDraft.value.workingDirectory, timeoutSeconds: Number(taskDraft.value.timeoutSeconds), shellMode: false }) })
    tasksNotice.value = result.status === 'manual_only' ? '任务已登记为人工处理，执行器不会代为处理 SSH_MCP。' : result.approvalRequired ? '任务已进入待审批队列；审批后才会执行。' : '任务已进入执行队列。'
    await loadTasks(true)
  } catch (e) { tasksError.value = e instanceof Error ? e.message : '提交任务失败' }
  finally { taskSubmitting.value = false }
}
async function approveTask(task: CommandTask) {
  if (!window.confirm(`确认审批并执行任务“${task.title}”？\n分类：${task.taskClass}\n程序：${task.executable}\n参数：${JSON.stringify(task.arguments)}\n工作目录：${task.workingDirectory}`)) return
  tasksError.value = ''; tasksNotice.value = ''
  try { await request(`/system-ops/tasks/${task.taskId}/approve`, { method: 'POST', body: '{}' }); tasksNotice.value = '审批已记录，任务已进入执行队列。'; await loadTasks(true) }
  catch (e) { tasksError.value = e instanceof Error ? e.message : '审批任务失败' }
}
async function cancelTask(task: CommandTask) {
  if (!window.confirm(`确认取消任务“${task.title}”？运行中的任务将尝试终止整个进程组。`)) return
  tasksError.value = ''
  try { await request(`/system-ops/tasks/${task.taskId}/cancel`, { method: 'POST', body: '{}' }); tasksNotice.value = '取消请求已提交。'; await loadTasks(true) }
  catch (e) { tasksError.value = e instanceof Error ? e.message : '取消任务失败' }
}
function taskClassLabel(value: TaskClass) { return ({ read_only: '只读诊断', file_inspection: '文件检查', test: '测试', build: '构建', code_change: '代码修改', system_admin: '系统管理', deploy: '部署', restart: '重启', database_change: '数据库变更', destructive: '破坏性操作', ssh_mcp: 'SSH_MCP 人工处理' } as Record<TaskClass,string>)[value] || value }
function taskStatusLabel(value: string) { return ({ pending_approval: '待审批', queued: '排队中', running: '执行中', completed: '已完成', failed: '失败', cancelled: '已取消', manual_only: '需人工处理', denied: '已拒绝' } as Record<string,string>)[value] || value }
function taskStatusClass(value: string) { return ['completed'].includes(value) ? 'done' : ['failed','denied'].includes(value) ? 'failed' : ['running','queued','pending_approval'].includes(value) ? 'active' : 'neutral' }

function formatTime(value?: string) {
  if (!value) return '尚无变更记录'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}
onMounted(() => { void loadPolicy(); void loadTasks(); taskRefreshTimer = setInterval(() => { if (!document.hidden) void loadTasks(true) }, 5000) })
onBeforeUnmount(() => { if (taskRefreshTimer) clearInterval(taskRefreshTimer) })
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

      <aside class="policy-boundary"><strong>执行边界与权限说明</strong><p>命令任务由独立的 root 执行器领取，但不通过 Shell 拼接执行；请求使用明确的可执行文件与参数数组。开发者模式会以 root 身份执行授权范围内的命令，稳定模式默认降权到 personalportal 账户，仅通过逐次审批及分类白名单的高风险任务会提升权限。可识别的直接命令重启特征会在后端重新分类；SSH_MCP 任务只登记、不代执行。任务队列禁用 Shell 字符串拼接，但脚本、测试和构建本身仍可能产生副作用；不要提交不可信脚本或来源不明的代码。</p></aside>

      <section class="policy-rules task-console">
        <div class="policy-section-heading"><div><h2>服务器任务队列</h2><p>任务通过 PostgreSQL 持久化，沿用现有事件核心进行唤醒，并由独立执行器处理。待审批任务不会自动执行。</p></div><button class="policy-button" type="button" :disabled="tasksLoading" @click="loadTasks()">{{ tasksLoading ? '刷新中…' : '刷新任务' }}</button></div>
        <p v-if="tasksError" class="policy-message is-error" role="alert">{{ tasksError }}</p><p v-if="tasksNotice" class="policy-message is-success" role="status">{{ tasksNotice }}</p>
        <form class="task-create-form" @submit.prevent="createTask">
          <label><span>任务标题</span><input v-model.trim="taskDraft.title" required minlength="3" maxlength="160" placeholder="描述任务目的"></label>
          <label><span>任务分类</span><select v-model="taskDraft.taskClass"><option value="read_only">只读诊断</option><option value="file_inspection">文件检查</option><option value="test">测试</option><option value="build">构建</option><option value="code_change">代码修改</option><option value="system_admin">系统管理</option><option value="deploy">部署</option><option value="restart">重启</option><option value="database_change">数据库变更</option><option value="destructive">破坏性操作</option><option value="ssh_mcp">SSH_MCP 人工处理</option></select></label>
          <label class="task-wide"><span>可执行文件（绝对路径）</span><input v-model.trim="taskDraft.executable" required maxlength="512" placeholder="/usr/bin/uname"></label>
          <label class="task-wide"><span>参数（JSON 字符串数组）</span><textarea v-model="taskDraft.argumentsText" rows="2" required spellcheck="false" placeholder='["-a"]'></textarea></label>
          <label class="task-wide"><span>工作目录（限定工作区）</span><input v-model.trim="taskDraft.workingDirectory" required placeholder="/opt/personal-portal"></label>
          <label><span>超时（秒）</span><input v-model.number="taskDraft.timeoutSeconds" type="number" min="1" max="1800" required></label>
          <div class="task-form-actions"><span>Shell 拼接执行已禁用。开发者模式以 root 执行命令，请确认命令来源与分类。</span><button class="policy-button is-primary" type="submit" :disabled="taskSubmitting">{{ taskSubmitting ? '提交中…' : '提交任务' }}</button></div>
        </form>
        <div v-if="tasks.length" class="task-list">
          <article v-for="task in tasks" :key="task.taskId" class="task-item">
            <div class="task-item-head"><div><strong>{{ task.title }}</strong><p>{{ taskClassLabel(task.taskClass) }} · {{ task.executable }} {{ task.arguments.join(' ') }}</p></div><span class="task-status" :class="taskStatusClass(task.status)">{{ taskStatusLabel(task.status) }}</span></div>
            <div class="task-meta"><span>提交者：{{ task.requestedBy }}</span><span>模式：{{ task.modeAtSubmission === 'developer' ? '开发者' : '稳定运行' }}</span><span>创建：{{ formatTime(task.createdAt) }}</span><span v-if="task.exitCode !== undefined">退出码：{{ task.exitCode }}</span></div>
            <p v-if="task.failureReason" class="task-failure">{{ task.failureReason }}</p>
            <details v-if="task.stdout || task.stderr" class="task-output"><summary>执行输出</summary><pre v-if="task.stdout">{{ task.stdout }}</pre><pre v-if="task.stderr" class="stderr">{{ task.stderr }}</pre></details>
            <div v-if="task.status === 'pending_approval' || task.status === 'queued' || task.status === 'running'" class="task-item-actions"><button v-if="task.status === 'pending_approval'" class="policy-button is-primary" type="button" @click="approveTask(task)">审批并执行</button><button class="policy-button" type="button" @click="cancelTask(task)">{{ task.status === 'running' ? '请求终止' : '取消任务' }}</button></div>
          </article>
        </div><p v-else-if="!tasksLoading" class="task-empty">任务队列为空。提交只读诊断或受限测试任务进行验证。</p>
      </section>

      <footer class="policy-footer"><div><span>最近变更</span><strong>{{ formatTime(policy.updatedAt) }}</strong><small>操作者：{{ policy.changedBy || '系统初始化' }}</small></div><div><span>策略保存状态</span><strong>{{ policy.enforcement === 'executor_connected' ? '执行器在线' : policy.enforcement === 'executor_disconnected' ? '执行器离线' : '执行器尚未接入' }}</strong><small>执行器状态：{{ policy.executorStatus || '未知' }} · 当前模式：{{ policy.mode === 'developer' ? '开发者模式' : '稳定运行模式' }}</small></div></footer>
      <div class="policy-actions"><span v-if="isDirty">有尚未保存的模式变更。</span><span v-else>当前选择与服务器保存的模式一致。</span><div><button class="policy-button" type="button" :disabled="saving || !isDirty" @click="selectedMode = policy!.mode">取消更改</button><button class="policy-button is-primary" type="button" :disabled="saving || !isDirty" @click="saveMode">{{ saving ? '正在保存…' : '保存运行模式' }}</button></div></div>
    </template>
  </section>
</template>

<style scoped>
.execution-policy{min-width:0;color:#202020}.policy-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin:0 0 25px}.policy-eyebrow{margin:0 0 7px;color:#777;font-size:11px;font-weight:650;letter-spacing:.12em}.policy-intro{max-width:760px;margin:0;color:#686868;font-size:14px;line-height:1.65}.policy-button{border:1px solid #d8d8d8;border-radius:7px;background:#fff;padding:9px 13px;color:#222;font-size:13px;white-space:nowrap;cursor:pointer}.policy-button:disabled{opacity:.45;cursor:not-allowed}.policy-button.is-primary{background:#202020;color:#fff;border-color:#202020}.policy-message{margin:0 0 16px;padding:11px 13px;border-radius:6px;font-size:13px}.policy-message.is-error{background:#fff0f0;color:#a32121}.policy-message.is-success{background:#f0f7f1;color:#28633b}.policy-loading{padding:35px 0;color:#777;font-size:13px}.policy-current{display:flex;justify-content:space-between;align-items:center;gap:20px;padding:20px 0;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7;margin-bottom:22px}.policy-current>div{min-width:0}.policy-kicker,.policy-current p{display:block;color:#777;font-size:12px}.policy-current strong{display:block;margin:7px 0;font-size:22px;letter-spacing:-.03em}.policy-current p{margin:0;line-height:1.6}.policy-state-dot{width:10px;height:10px;flex:0 0 auto;border-radius:50%;background:#647b65;box-shadow:0 0 0 5px #647b651a}.policy-state-dot.is-developer{background:#3f6e9d;box-shadow:0 0 0 5px #3f6e9d1a}.policy-mode-switch-panel{display:flex;justify-content:space-between;align-items:center;gap:24px;padding:19px 0;border-bottom:1px solid #e7e7e7;margin-bottom:17px}.policy-mode-switch-copy{min-width:0}.policy-mode-switch-copy>strong{display:block;margin:7px 0;font-size:18px}.policy-mode-switch-copy p{max-width:680px;margin:0;color:#777;font-size:12px;line-height:1.7}.policy-switch-control{display:flex;align-items:center;gap:10px;flex:0 0 auto}.policy-switch-control>span{font-size:12px;color:#999;white-space:nowrap}.policy-switch-control>span.active{color:#222;font-weight:650}.policy-switch{display:flex;align-items:center;width:48px;height:28px;padding:3px;border:0;border-radius:999px;background:#c8c8c8;cursor:pointer;transition:background .16s}.policy-switch[aria-checked="true"]{background:#242424}.policy-switch>span{display:block;width:22px;height:22px;border-radius:50%;background:#fff;box-shadow:0 1px 3px #0002;transition:transform .16s}.policy-switch[aria-checked="true"]>span{transform:translateX(20px)}.policy-mode-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;margin-bottom:34px}.policy-mode-card{display:flex;flex-direction:column;align-items:flex-start;gap:10px;padding:17px;border:1px solid #e2e2e2;border-radius:9px;background:rgba(255,255,255,.5)}.policy-mode-card.selected{border-color:#777;background:rgba(245,245,245,.7)}.policy-mode-card>strong{font-size:14px}.policy-mode-card p{margin:0;color:#666;font-size:12px;line-height:1.7}.policy-mode-tag{display:inline-flex;padding:4px 7px;border-radius:4px;background:#f0f0f0;color:#555;font-size:11px}.policy-rules{margin:0 0 30px}.policy-section-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin-bottom:10px}.policy-section-heading h2{margin:0 0 5px;font-size:17px}.policy-section-heading p{margin:0;color:#777;font-size:12px;line-height:1.6}.policy-section-heading>span{font-size:11px;color:#555;white-space:nowrap}.policy-rule-row{display:grid;grid-template-columns:35px minmax(0,1fr) auto;align-items:start;gap:12px;padding:15px 0;border-top:1px solid #e8e8e8}.policy-rule-icon{display:flex;width:28px;height:28px;align-items:center;justify-content:center;border-radius:6px;background:#f1f1f1;color:#555;font-size:11px}.policy-rule-row strong,.policy-task-list strong{font-size:13px}.policy-rule-row p{margin:5px 0 0;color:#777;font-size:12px;line-height:1.6}.policy-lock{font-size:11px;color:#7b4b14;background:#fbf2e4;padding:5px 7px;border-radius:4px;white-space:nowrap}.policy-task-list{border-top:1px solid #e7e7e7}.policy-task-list>div{display:flex;justify-content:space-between;gap:18px;padding:13px 0;border-bottom:1px solid #ededed}.policy-task-list span{max-width:65%;color:#777;font-size:12px;line-height:1.6;text-align:right}.policy-boundary{margin:4px 0 25px;padding:14px 15px;border-left:2px solid #aaa;background:rgba(245,245,245,.55)}.policy-boundary strong{font-size:12px}.policy-boundary p{margin:6px 0 0;color:#666;font-size:12px;line-height:1.75}.policy-footer{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px;padding:18px 0;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7}.policy-footer>div{display:flex;flex-direction:column;gap:5px}.policy-footer span,.policy-footer small{color:#777;font-size:11px}.policy-footer strong{font-size:13px;font-weight:600}.policy-actions{display:flex;justify-content:space-between;align-items:center;gap:14px;padding:18px 0}.policy-actions>span{color:#777;font-size:12px}.policy-actions>div{display:flex;gap:8px}.task-create-form{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;padding:16px 0 20px;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7}.task-create-form label{display:flex;flex-direction:column;gap:6px;min-width:0}.task-create-form label>span{color:#666;font-size:12px}.task-create-form input,.task-create-form select,.task-create-form textarea{width:100%;min-width:0;border:1px solid #dcdcdc;border-radius:6px;background:rgba(255,255,255,.8);padding:9px 10px;color:#222;font:inherit;font-size:12px;outline-offset:2px}.task-create-form textarea{resize:vertical;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.task-wide{grid-column:1/-1}.task-form-actions{grid-column:1/-1;display:flex;align-items:center;justify-content:space-between;gap:12px}.task-form-actions>span{max-width:70%;color:#777;font-size:11px;line-height:1.6}.task-list{display:flex;flex-direction:column}.task-item{padding:16px 0;border-bottom:1px solid #e7e7e7}.task-item-head{display:flex;align-items:flex-start;justify-content:space-between;gap:14px}.task-item-head strong{font-size:13px}.task-item-head p{margin:5px 0 0;color:#777;font-size:11px;line-height:1.6;overflow-wrap:anywhere}.task-status{flex:0 0 auto;border-radius:4px;padding:4px 7px;font-size:11px;background:#eee;color:#555}.task-status.done{background:#edf6ee;color:#2e683c}.task-status.failed{background:#fff0f0;color:#a32121}.task-status.active{background:#f8f1e5;color:#81551c}.task-meta{display:flex;flex-wrap:wrap;gap:7px 14px;margin-top:9px;color:#888;font-size:11px}.task-failure{margin:8px 0 0;color:#a32121;font-size:12px;line-height:1.6}.task-output{margin-top:9px;font-size:12px}.task-output summary{cursor:pointer;color:#555}.task-output pre{max-height:260px;overflow:auto;margin:8px 0 0;padding:10px;border-radius:5px;background:#f5f5f5;color:#333;white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace}.task-output pre.stderr{background:#fff2f2;color:#8d2222}.task-item-actions{display:flex;gap:8px;margin-top:11px}.task-empty{padding:18px 0;color:#888;font-size:12px}
@media(max-width:760px){.policy-heading{flex-direction:column}.policy-mode-switch-panel{align-items:flex-start;flex-direction:column}.policy-mode-grid,.policy-footer,.task-create-form{grid-template-columns:1fr}.task-wide,.task-form-actions{grid-column:1}.task-form-actions{align-items:flex-start;flex-direction:column}.task-form-actions>span{max-width:none}.policy-task-list>div{flex-direction:column;gap:5px}.policy-task-list span{max-width:none;text-align:left}.policy-actions{align-items:flex-start;flex-direction:column}.policy-rule-row{grid-template-columns:30px minmax(0,1fr)}.policy-lock{grid-column:2;justify-self:start}}
</style>
