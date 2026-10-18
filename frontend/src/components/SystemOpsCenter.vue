<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, watch } from 'vue'
const SystemExecutionPolicyCenter = defineAsyncComponent(() => import('./SystemExecutionPolicyCenter.vue'))

type LogEntry = {
  id: number
  time?: string
  source: 'server' | 'ssh'
  level: 'info' | 'warning' | 'error' | string
  action?: string
  message: string
  details?: Record<string, unknown>
}
type Props = { view: 'warnings' | 'logs' | 'policy' }
const props = defineProps<Props>()
const activeSource = ref<'server' | 'ssh'>('server')
const logs = ref<LogEntry[]>([])
const warnings = ref<LogEntry[]>([])
const searchQuery = ref('')
const levelFilter = ref('all')
const loading = ref(false)
const error = ref('')
const connectionState = ref<'connecting' | 'live' | 'paused' | 'offline'>('connecting')
const paused = ref(false)
const cursor = ref(0)
const failedFileCleanupEvents = ref(0)
const checkedAt = ref('')
let stream: EventSource | null = null
let warningTimer: ReturnType<typeof setInterval> | undefined

const filteredLogs = computed(() => logs.value.filter(item => {
  const matchesLevel = levelFilter.value === 'all' || item.level === levelFilter.value
  const haystack = `${item.time || ''} ${item.action || ''} ${item.message} ${JSON.stringify(item.details || {})}`.toLocaleLowerCase()
  return matchesLevel && haystack.includes(searchQuery.value.trim().toLocaleLowerCase())
}))
const sourceTitle = computed(() => activeSource.value === 'server' ? '服务器运行日志' : 'GPT · SSH 操作日志')
const stateLabel = computed(() => ({ connecting: '正在连接', live: '实时连接中', paused: '已暂停', offline: '连接中断' })[connectionState.value])

async function api<T>(path: string): Promise<T> {
  const response = await fetch(`/api${path}`, { credentials: 'same-origin', headers: { Accept: 'application/json' }, cache: 'no-store' })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body?.error || `请求失败（${response.status}）`)
  return body as T
}
function closeStream() {
  stream?.close()
  stream = null
}
function addLog(item: LogEntry) {
  if (!item || !item.message) return
  if (item.id && logs.value.some(existing => existing.id === item.id)) return
  logs.value.push(item)
  if (item.id) cursor.value = Math.max(cursor.value, item.id)
  if (logs.value.length > 500) logs.value.splice(0, logs.value.length - 500)
}
async function loadLogs(reset = false) {
  if (props.view !== 'logs') return
  closeStream()
  loading.value = true
  error.value = ''
  if (reset) { logs.value = []; cursor.value = 0 }
  try {
    const params = new URLSearchParams({ source: activeSource.value, limit: '100', after: reset ? '0' : String(cursor.value) })
    const result = await api<{ items: LogEntry[]; cursor: number }>(`/system-ops/logs?${params}`)
    if (reset) logs.value = result.items || []
    else for (const item of result.items || []) addLog(item)
    cursor.value = Math.max(cursor.value, Number(result.cursor || 0))
    if (!paused.value) connectStream()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '读取日志失败'
    connectionState.value = 'offline'
  } finally {
    loading.value = false
  }
}
function connectStream() {
  closeStream()
  if (paused.value || props.view !== 'logs') { connectionState.value = 'paused'; return }
  connectionState.value = 'connecting'
  const params = new URLSearchParams({ source: activeSource.value, after: String(cursor.value) })
  stream = new EventSource(`/api/system-ops/stream?${params}`, { withCredentials: true })
  stream.onopen = () => { connectionState.value = 'live'; error.value = '' }
  stream.onerror = () => { connectionState.value = 'offline' }
  stream.addEventListener('log', event => {
    try { addLog(JSON.parse((event as MessageEvent).data) as LogEntry) }
    catch { /* Ignore a malformed event without breaking the stream. */ }
  })
}
async function loadWarnings() {
  error.value = ''
  try {
    const result = await api<{ items: LogEntry[]; failedFileCleanupEvents: number; checkedAt: string }>('/system-ops/warnings')
    warnings.value = result.items || []
    failedFileCleanupEvents.value = Number(result.failedFileCleanupEvents || 0)
    checkedAt.value = result.checkedAt || ''
  } catch (e) { error.value = e instanceof Error ? e.message : '读取系统警告失败' }
}
function togglePause() {
  paused.value = !paused.value
  if (paused.value) { closeStream(); connectionState.value = 'paused' }
  else void loadLogs(false)
}
function formatTime(value?: string) {
  if (!value) return '时间未提供'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN', { hour12: false })
}
function levelLabel(level: string) { return ({ info: '信息', warning: '警告', error: '错误' } as Record<string, string>)[level] || level }
function actionLabel(action?: string) {
  if (!action) return ''
  const labels: Record<string, string> = {
    execute_shell: '执行 Shell 命令', execute_shell_result: 'Shell 命令执行结果', execute_shell_error: 'Shell 命令执行失败',
    read_file: '读取文件', write_file: '写入文件', replace_in_file: '修改文件', move_path: '移动路径', delete_path: '删除路径',
    get_service_status: '查询服务状态', get_service_logs: '读取服务日志', get_system_info: '读取系统信息', list_directory: '列出目录',
  }
  return labels[action] || action
}
watch(activeSource, () => { paused.value = false; void loadLogs(true) })
onMounted(() => {
  if (props.view === 'logs') void loadLogs(true)
  else if (props.view === 'warnings') {
    void loadWarnings()
    warningTimer = setInterval(() => { if (!document.hidden) void loadWarnings() }, 15000)
  }
})
onBeforeUnmount(() => { closeStream(); if (warningTimer) clearInterval(warningTimer) })
</script>

<template>
  <SystemExecutionPolicyCenter v-if="view === 'policy'" />
  <section v-else class="system-ops-center">
    <header class="ops-heading">
      <div><p class="ops-eyebrow">SYSTEM OPERATIONS</p><h1 class="account-page-title">{{ view === 'warnings' ? '系统警告' : '系统日志' }}</h1><p class="ops-intro">{{ view === 'warnings' ? '集中查看服务器异常、执行失败和需要人工处理的运维事件。' : '按需订阅新增日志；关闭本页后停止实时连接，不进行后台持续轮询。' }}</p></div>
      <button v-if="view === 'warnings'" class="ops-button" type="button" @click="loadWarnings">刷新警告</button>
      <button v-else class="ops-button" type="button" :disabled="loading" @click="loadLogs(false)">刷新历史</button>
    </header>

    <p v-if="error" class="ops-error" role="alert">{{ error }}</p>

    <template v-if="view === 'warnings'">
      <div class="ops-summary-row">
        <article><span>当前警告记录</span><strong>{{ warnings.length }}</strong><small>最近可识别的异常与告警</small></article>
        <article :class="{ 'is-risk': failedFileCleanupEvents > 0 }"><span>清理事件失败</span><strong>{{ failedFileCleanupEvents }}</strong><small>重试耗尽后需要人工处理</small></article>
        <article><span>最近检查</span><strong class="ops-time-value">{{ checkedAt ? formatTime(checkedAt) : '—' }}</strong><small>页面打开期间每 15 秒检查一次</small></article>
      </div>
      <div class="ops-table-head"><div><h2>告警与异常</h2><p>来自系统消息日志、SSH 操作审计及持久化文件清理事件。</p></div><span>{{ warnings.length }} 条记录</span></div>
      <div v-if="warnings.length" class="ops-log-list">
        <article v-for="(item, index) in warnings" :key="`${item.source}-${item.id || item.time}-${index}`" class="ops-log-row">
          <span class="ops-level" :class="`level-${item.level}`">{{ levelLabel(item.level) }}</span><div class="ops-log-copy"><strong>{{ item.message }}</strong><p>{{ item.source === 'ssh' ? 'GPT · SSH 操作审计' : '服务器运行日志' }}<span v-if="item.action"> · {{ actionLabel(item.action) }}</span></p></div><time>{{ formatTime(item.time) }}</time>
        </article>
      </div>
      <div v-else class="ops-empty"><strong>暂未发现可识别的系统警告</strong><p>系统日志会继续记录新事件；此处不代表服务器完全没有风险。</p></div>
    </template>

    <template v-else>
      <nav class="ops-source-tabs" aria-label="日志来源">
        <button type="button" :class="{ active: activeSource === 'server' }" @click="activeSource = 'server'">服务器运行日志</button>
        <button type="button" :class="{ active: activeSource === 'ssh' }" @click="activeSource = 'ssh'">GPT 连接 SSH 的操作日志</button>
      </nav>
      <div class="ops-log-toolbar"><div><strong>{{ sourceTitle }}</strong><span class="ops-connection" :class="`state-${connectionState}`"><i></i>{{ stateLabel }}</span></div><div class="ops-toolbar-actions"><input v-model="searchQuery" type="search" placeholder="搜索日志内容…" aria-label="搜索日志内容"><select v-model="levelFilter" aria-label="按日志级别筛选"><option value="all">全部级别</option><option value="info">信息</option><option value="warning">警告</option><option value="error">错误</option></select><button class="ops-button" type="button" @click="togglePause">{{ paused ? '恢复实时' : '暂停实时' }}</button></div></div>
      <p v-if="activeSource === 'ssh'" class="ops-note">为避免在管理界面暴露高权限命令、密码或命令输出，SSH 日志只展示操作类型、结果与必要元数据；完整原始审计文件仍由服务器 root 权限保护。</p>
      <div v-if="filteredLogs.length" class="ops-log-list ops-live-list" aria-live="polite">
        <article v-for="item in [...filteredLogs].reverse()" :key="item.id" class="ops-log-row">
          <span class="ops-level" :class="`level-${item.level}`">{{ levelLabel(item.level) }}</span><div class="ops-log-copy"><strong>{{ activeSource === 'ssh' ? actionLabel(item.action) || item.message : item.message }}</strong><p><span>{{ formatTime(item.time) }}</span><template v-if="item.details"> <span v-for="(value, key) in item.details" :key="key"> · {{ key }}: {{ String(value) }}</span></template></p></div><code v-if="item.id">#{{ item.id }}</code>
        </article>
      </div>
      <div v-else class="ops-empty"><strong>{{ loading ? '正在读取日志…' : '暂无匹配日志' }}</strong><p>{{ searchQuery ? '请调整搜索词或级别筛选。' : '有新日志时会自动追加；实时连接只在此页面打开时运行。' }}</p></div>
      <footer class="ops-log-footer"><span>当前展示 {{ filteredLogs.length }} 条 · 最多保留最近 500 条在浏览器内存中</span><span>服务端每 2 秒检查一次追加内容，不重复读取整份日志</span></footer>
    </template>
  </section>
</template>

<style scoped>
.system-ops-center{min-width:0;color:#202020}.ops-heading{display:flex;justify-content:space-between;align-items:flex-start;gap:24px;margin:0 0 26px}.ops-heading h1{margin:0 0 7px;font-size:26px;font-weight:650;letter-spacing:-.035em}.ops-eyebrow{margin:0 0 7px;color:#777;font-size:11px;font-weight:650;letter-spacing:.12em}.ops-intro{max-width:760px;margin:0;color:#686868;font-size:14px;line-height:1.65}.ops-button{border:1px solid #d7d7d7;border-radius:7px;background:#fff;padding:9px 13px;color:#222;font-size:13px;white-space:nowrap;cursor:pointer}.ops-button:disabled{opacity:.5;cursor:not-allowed}.ops-error{margin:0 0 16px;padding:12px 14px;border-radius:7px;background:#fff0f0;color:#a32121;font-size:13px}.ops-summary-row{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:0 0 30px}.ops-summary-row article{display:flex;flex-direction:column;gap:8px;padding:16px 0;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7}.ops-summary-row article span,.ops-summary-row small{color:#777;font-size:12px}.ops-summary-row article strong{font-size:28px;font-weight:650;font-variant-numeric:tabular-nums}.ops-summary-row article.is-risk strong{color:#a52e21}.ops-summary-row .ops-time-value{font-size:14px;line-height:1.5}.ops-table-head{display:flex;justify-content:space-between;gap:16px;align-items:flex-start;margin:0 0 12px}.ops-table-head h2{margin:0 0 5px;font-size:17px}.ops-table-head p,.ops-table-head>span{margin:0;color:#777;font-size:12px;line-height:1.6}.ops-source-tabs{display:flex;gap:6px;flex-wrap:wrap;border-bottom:1px solid #e5e5e5;margin:0 0 18px}.ops-source-tabs button{padding:12px 14px;border:0;border-bottom:2px solid transparent;background:transparent;color:#6d6d6d;font-size:13px;cursor:pointer}.ops-source-tabs button.active{border-bottom-color:#222;color:#222;font-weight:650}.ops-log-toolbar{display:flex;justify-content:space-between;align-items:center;gap:14px;margin:0 0 12px}.ops-log-toolbar>div:first-child{display:flex;align-items:center;gap:12px;flex-wrap:wrap}.ops-log-toolbar strong{font-size:14px}.ops-connection{display:inline-flex;align-items:center;gap:6px;color:#777;font-size:11px}.ops-connection i{width:6px;height:6px;border-radius:50%;background:#aaa}.ops-connection.state-live i{background:#388451}.ops-connection.state-offline i{background:#b63b30}.ops-toolbar-actions{display:flex;gap:7px;align-items:center}.ops-toolbar-actions input,.ops-toolbar-actions select{max-width:220px;min-width:120px;border:1px solid #ddd;border-radius:6px;background:#fff;padding:9px 10px;color:#222;font:inherit;font-size:12px}.ops-toolbar-actions input{width:min(26vw,230px)}.ops-note{padding:10px 12px;margin:0 0 12px;background:#f6f6f6;color:#666;border-radius:6px;font-size:12px;line-height:1.6}.ops-log-list{border-top:1px solid #e7e7e7}.ops-log-row{display:grid;grid-template-columns:56px minmax(0,1fr) auto;align-items:start;gap:12px;padding:13px 4px;border-bottom:1px solid #ededed;font-size:12px}.ops-level{display:inline-flex;justify-content:center;align-items:center;padding:4px 6px;border-radius:4px;background:#f0f0f0;color:#555;font-size:11px;white-space:nowrap}.ops-level.level-warning{background:#fbf1dc;color:#805c0c}.ops-level.level-error{background:#f9e6e2;color:#9d3425}.ops-log-copy{min-width:0}.ops-log-copy strong{display:block;font-weight:550;color:#252525;overflow-wrap:anywhere;line-height:1.55}.ops-log-copy p{margin:4px 0 0;color:#888;line-height:1.6;overflow-wrap:anywhere}.ops-log-row time,.ops-log-row code{color:#888;font-size:11px;white-space:nowrap}.ops-log-row code{font-family:ui-monospace,monospace}.ops-empty{padding:52px 20px;text-align:center;border-top:1px solid #e7e7e7;border-bottom:1px solid #e7e7e7}.ops-empty strong{font-size:14px}.ops-empty p{margin:8px 0 0;color:#777;font-size:12px;line-height:1.6}.ops-log-footer{display:flex;justify-content:space-between;gap:12px;padding:13px 0;color:#888;font-size:11px;line-height:1.6}.ops-log-footer span:last-child{text-align:right}@media(max-width:850px){.ops-heading{flex-direction:column}.ops-summary-row{grid-template-columns:1fr}.ops-log-toolbar{align-items:flex-start;flex-direction:column}.ops-toolbar-actions{width:100%;flex-wrap:wrap}.ops-toolbar-actions input{width:100%;max-width:none;flex:1}.ops-log-footer{flex-direction:column}.ops-log-footer span:last-child{text-align:left}.ops-log-row{grid-template-columns:56px minmax(0,1fr)}.ops-log-row time,.ops-log-row code{grid-column:2;white-space:normal}.ops-table-head{flex-direction:column}}
</style>
