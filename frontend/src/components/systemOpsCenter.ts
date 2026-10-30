/**
 * Responsibility: render system warnings, live logs, and the account role/permission management view.
 * Dependencies: same-origin apiRequest transport and the native permission-management module.
 * Collaboration: portalApplication.ts selects the protected view; this module owns log/warning lifecycles
 * and delegates role/permission state to the permission-management controller.
 * Boundaries: logs are display-only; no privileged command execution or server execution policy is exposed.
 * Side effects: warnings poll every 15s while mounted; logs open one credentialed EventSource while mounted;
 * dispose closes streams, timers, child controllers and listeners.
 */
import { apiRequest } from '../api/client'
import { mountPermissionManagement } from './permissionManagement'

type View = 'warnings' | 'logs' | 'permissions'
type Source = 'server' | 'ssh'
type ConnectionState = 'connecting' | 'live' | 'paused' | 'offline'
type LogEntry = {
  id: number
  time?: string
  source: 'server' | 'ssh'
  level: 'info' | 'warning' | 'error' | string
  action?: string
  message: string
  details?: Record<string, unknown>
}
type State = {
  activeSource: Source
  logs: LogEntry[]
  warnings: LogEntry[]
  searchQuery: string
  levelFilter: string
  loading: boolean
  error: string
  connectionState: ConnectionState
  paused: boolean
  cursor: number
  failedFileCleanupEvents: number
  checkedAt: string
}

export type SystemOpsController = { readonly ready: Promise<void>; dispose: () => void }

function el<K extends keyof HTMLElementTagNameMap>(document: Document, tag: K, className = '', text = ''): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text) element.textContent = text
  return element
}

function button(document: Document, label: string, action: string, className = 'ops-button', disabled = false): HTMLButtonElement {
  const control = el(document, 'button', className, label)
  control.type = 'button'
  control.dataset.action = action
  control.dataset.focusId = action
  control.disabled = disabled
  return control
}

export function mountSystemOpsCenter(host: HTMLElement, options: { view: View }): SystemOpsController {
  const document = host.ownerDocument
  const window = document.defaultView!
  const root = el(document, 'section', 'system-ops-center')
  const listeners = new window.AbortController()
  const state: State = {
    activeSource: 'server', logs: [], warnings: [], searchQuery: '', levelFilter: 'all', loading: false,
    error: '', connectionState: 'connecting', paused: false, cursor: 0, failedFileCleanupEvents: 0,
    checkedAt: '',
  }
  let disposed = false
  let stream: EventSource | null = null
  let logLoadSequence = 0
  let warningTimer: number | undefined
  let childDispose: (() => void) | null = null
  let ready: Promise<void> = Promise.resolve()
  host.replaceChildren(root)

  async function api<T>(path: string): Promise<T> {
    return apiRequest<T>(path, { cache: 'no-store' })
  }

  function closeStream(): void {
    stream?.close()
    stream = null
  }

  function addLog(item: LogEntry): void {
    if (!item || !item.message) return
    if (item.id && state.logs.some(existing => existing.id === item.id)) return
    state.logs.push(item)
    if (item.id) state.cursor = Math.max(state.cursor, item.id)
    if (state.logs.length > 500) state.logs.splice(0, state.logs.length - 500)
  }

  function connectStream(): void {
    closeStream()
    if (state.paused || options.view !== 'logs') {
      state.connectionState = 'paused'
      render()
      return
    }
    state.connectionState = 'connecting'
    const params = new URLSearchParams({ source: state.activeSource, after: String(state.cursor) })
    const EventSourceConstructor = window.EventSource
    if (!EventSourceConstructor) {
      state.connectionState = 'offline'
      state.error = '当前浏览器不支持实时日志连接。'
      render()
      return
    }
    stream = new EventSourceConstructor(`/api/system-ops/stream?${params}`, { withCredentials: true })
    stream.onopen = () => {
      if (disposed) return
      state.connectionState = 'live'
      state.error = ''
      render()
    }
    stream.onerror = () => {
      if (disposed) return
      state.connectionState = 'offline'
      render()
    }
    stream.addEventListener('log', event => {
      if (disposed) return
      try {
        addLog(JSON.parse((event as MessageEvent).data) as LogEntry)
        render()
      } catch {
        // Ignore one malformed event; keep the stream available for subsequent valid events.
      }
    })
    render()
  }

  async function loadLogs(reset = false): Promise<void> {
    if (options.view !== 'logs' || disposed) return
    const sequence = ++logLoadSequence
    const source = state.activeSource
    closeStream()
    state.loading = true
    state.error = ''
    if (reset) { state.logs = []; state.cursor = 0 }
    render()
    try {
      const params = new URLSearchParams({ source, limit: '100', after: reset ? '0' : String(state.cursor) })
      const result = await api<{ items: LogEntry[]; cursor: number }>(`/system-ops/logs?${params}`)
      if (disposed || sequence !== logLoadSequence || source !== state.activeSource) return
      if (reset) state.logs = result.items || []
      else for (const item of result.items || []) addLog(item)
      state.cursor = Math.max(state.cursor, Number(result.cursor || 0))
      if (!state.paused) connectStream()
    } catch (error) {
      if (disposed || sequence !== logLoadSequence || source !== state.activeSource) return
      state.error = error instanceof Error ? error.message : '读取日志失败'
      state.connectionState = 'offline'
    } finally {
      if (!disposed && sequence === logLoadSequence) {
        state.loading = false
        render()
      }
    }
  }

  async function loadWarnings(): Promise<void> {
    if (options.view !== 'warnings' || disposed) return
    state.error = ''
    render()
    try {
      const result = await api<{ items: LogEntry[]; failedFileCleanupEvents: number; checkedAt: string }>('/system-ops/warnings')
      if (disposed) return
      state.warnings = result.items || []
      state.failedFileCleanupEvents = Number(result.failedFileCleanupEvents || 0)
      state.checkedAt = result.checkedAt || ''
    } catch (error) {
      if (disposed) return
      state.error = error instanceof Error ? error.message : '读取系统警告失败'
    } finally {
      if (!disposed) render()
    }
  }

  function togglePause(): void {
    state.paused = !state.paused
    if (state.paused) {
      closeStream()
      state.connectionState = 'paused'
      render()
    } else {
      void loadLogs(false)
    }
  }

  function formatTime(value?: string): string {
    if (!value) return '时间未提供'
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString('zh-CN', { hour12: false, timeZone: 'Asia/Shanghai' })
  }
  function levelLabel(level: string): string {
    return ({ info: '信息', warning: '警告', error: '错误' } as Record<string, string>)[level] || level
  }
  function actionLabel(action?: string): string {
    if (!action) return ''
    const labels: Record<string, string> = {
      execute_shell: '执行 Shell 命令', execute_shell_result: 'Shell 命令执行结果', execute_shell_error: 'Shell 命令执行失败',
      read_file: '读取文件', write_file: '写入文件', replace_in_file: '修改文件', move_path: '移动路径', delete_path: '删除路径',
      get_service_status: '查询服务状态', get_service_logs: '读取服务日志', get_system_info: '读取系统信息', list_directory: '列出目录',
    }
    return labels[action] || action
  }
  function stateLabel(): string {
    return ({ connecting: '正在连接', live: '实时连接中', paused: '已暂停', offline: '连接中断' } as Record<ConnectionState, string>)[state.connectionState]
  }
  function filteredLogs(): LogEntry[] {
    const query = state.searchQuery.trim().toLocaleLowerCase()
    return state.logs.filter(item => {
      const matchesLevel = state.levelFilter === 'all' || item.level === state.levelFilter
      const haystack = `${item.time || ''} ${item.action || ''} ${item.message} ${JSON.stringify(item.details || {})}`.toLocaleLowerCase()
      return matchesLevel && haystack.includes(query)
    })
  }

  function renderWarnings(): void {
    const heading = el(document, 'header', 'ops-heading')
    const copy = el(document, 'div')
    copy.append(el(document, 'p', 'ops-eyebrow', 'SYSTEM OPERATIONS'), el(document, 'h1', 'account-page-title', '系统警告'), el(document, 'p', 'ops-intro', '集中查看服务器异常、文件清理失败和需要人工处理的运维事件。'))
    heading.append(copy, button(document, '刷新警告', 'refresh-warnings'))
    root.append(heading)

    if (state.error) {
      const error = el(document, 'p', 'ops-error', state.error)
      error.setAttribute('role', 'alert')
      root.append(error)
    }

    const summary = el(document, 'div', 'ops-summary-row')
    const recordCard = el(document, 'article')
    recordCard.append(el(document, 'span', '', '当前警告记录'), el(document, 'strong', '', String(state.warnings.length)), el(document, 'small', '', '最近可识别的异常与告警'))
    const cleanupCard = el(document, 'article', state.failedFileCleanupEvents > 0 ? 'is-risk' : '')
    cleanupCard.append(el(document, 'span', '', '清理事件失败'), el(document, 'strong', '', String(state.failedFileCleanupEvents)), el(document, 'small', '', '重试耗尽后需要人工处理'))
    const checkedCard = el(document, 'article')
    checkedCard.append(el(document, 'span', '', '最近检查'), el(document, 'strong', 'ops-time-value', state.checkedAt ? formatTime(state.checkedAt) : '—'), el(document, 'small', '', '页面打开期间每 15 秒检查一次'))
    summary.append(recordCard, cleanupCard, checkedCard)
    root.append(summary)

    const tableHead = el(document, 'div', 'ops-table-head')
    const tableCopy = el(document, 'div')
    tableCopy.append(el(document, 'h2', '', '告警与异常'), el(document, 'p', '', '来自系统消息日志、SSH 操作审计及持久化文件清理事件。'))
    tableHead.append(tableCopy, el(document, 'span', '', `${state.warnings.length} 条记录`))
    root.append(tableHead)

    if (state.warnings.length) {
      const list = el(document, 'div', 'ops-log-list')
      state.warnings.forEach((item, index) => {
        const row = el(document, 'article', 'ops-log-row')
        row.append(el(document, 'span', `ops-level level-${item.level}`, levelLabel(item.level)))
        const copy = el(document, 'div', 'ops-log-copy')
        const description = `${item.source === 'ssh' ? 'GPT · SSH 操作审计' : '服务器运行日志'}${item.action ? ` · ${actionLabel(item.action)}` : ''}`
        copy.append(el(document, 'strong', '', item.message), el(document, 'p', '', description))
        row.append(copy, el(document, 'time', '', formatTime(item.time)))
        row.dataset.rowKey = `${item.source}-${item.id || item.time}-${index}`
        list.append(row)
      })
      root.append(list)
    } else {
      const empty = el(document, 'div', 'ops-empty')
      empty.append(el(document, 'strong', '', '暂未发现可识别的系统警告'), el(document, 'p', '', '系统日志会继续记录新事件；此处不代表服务器完全没有风险。'))
      root.append(empty)
    }
  }

  function renderLogs(): void {
    const heading = el(document, 'header', 'ops-heading')
    const copy = el(document, 'div')
    copy.append(el(document, 'p', 'ops-eyebrow', 'SYSTEM OPERATIONS'), el(document, 'h1', 'account-page-title', '系统日志'), el(document, 'p', 'ops-intro', '按需订阅新增日志；关闭本页后停止实时连接，不进行后台持续轮询。'))
    heading.append(copy, button(document, '刷新历史', 'refresh-logs', 'ops-button', state.loading))
    root.append(heading)

    if (state.error) {
      const error = el(document, 'p', 'ops-error', state.error)
      error.setAttribute('role', 'alert')
      root.append(error)
    }

    const tabs = el(document, 'nav', 'ops-source-tabs')
    tabs.setAttribute('aria-label', '日志来源')
    const serverTab = button(document, '服务器运行日志', 'source-server', 'ops-button')
    const sshTab = button(document, 'GPT 连接 SSH 的操作日志', 'source-ssh', 'ops-button')
    serverTab.classList.toggle('active', state.activeSource === 'server')
    sshTab.classList.toggle('active', state.activeSource === 'ssh')
    tabs.append(serverTab, sshTab)
    root.append(tabs)

    const toolbar = el(document, 'div', 'ops-log-toolbar')
    const toolbarTitle = el(document, 'div')
    toolbarTitle.append(el(document, 'strong', '', state.activeSource === 'server' ? '服务器运行日志' : 'GPT · SSH 操作日志'))
    const connection = el(document, 'span', `ops-connection state-${state.connectionState}`)
    connection.append(el(document, 'i'), document.createTextNode(stateLabel()))
    toolbarTitle.append(connection)
    const actions = el(document, 'div', 'ops-toolbar-actions')
    const search = el(document, 'input')
    search.type = 'search'
    search.placeholder = '搜索日志内容…'
    search.setAttribute('aria-label', '搜索日志内容')
    search.dataset.focusId = 'log-search'
    search.value = state.searchQuery
    const filter = el(document, 'select')
    filter.setAttribute('aria-label', '按日志级别筛选')
    filter.dataset.focusId = 'log-level-filter'
    const filters: Array<[string, string]> = [['all', '全部级别'], ['info', '信息'], ['warning', '警告'], ['error', '错误']]
    for (const [value, label] of filters) {
      const option = el(document, 'option', '', label)
      option.value = value
      option.selected = state.levelFilter === value
      filter.append(option)
    }
    actions.append(search, filter, button(document, state.paused ? '恢复实时' : '暂停实时', 'toggle-pause'))
    toolbar.append(toolbarTitle, actions)
    root.append(toolbar)

    if (state.activeSource === 'ssh') {
      root.append(el(document, 'p', 'ops-note', '为避免在管理界面暴露高权限命令、密码或命令输出，SSH 日志只展示操作类型、结果与必要元数据；完整原始审计文件仍由服务器 root 权限保护。'))
    }

    const items = filteredLogs().reverse()
    if (items.length) {
      const list = el(document, 'div', 'ops-log-list ops-live-list')
      list.setAttribute('aria-live', 'polite')
      for (const item of items) {
        const row = el(document, 'article', 'ops-log-row')
        row.append(el(document, 'span', `ops-level level-${item.level}`, levelLabel(item.level)))
        const copy = el(document, 'div', 'ops-log-copy')
        const title = state.activeSource === 'ssh' ? actionLabel(item.action) || item.message : item.message
        const description = el(document, 'p')
        description.append(el(document, 'span', '', formatTime(item.time)))
        if (item.details) {
          for (const [key, value] of Object.entries(item.details)) description.append(document.createTextNode(` · ${key}: ${String(value)}`))
        }
        copy.append(el(document, 'strong', '', title), description)
        row.append(copy)
        if (item.id) row.append(el(document, 'code', '', `#${item.id}`))
        list.append(row)
      }
      root.append(list)
    } else {
      const empty = el(document, 'div', 'ops-empty')
      empty.append(el(document, 'strong', '', state.loading ? '正在读取日志…' : '暂无匹配日志'))
      empty.append(el(document, 'p', '', state.searchQuery ? '请调整搜索词或级别筛选。' : '有新日志时会自动追加；实时连接只在此页面打开时运行。'))
      root.append(empty)
    }
    const footer = el(document, 'footer', 'ops-log-footer')
    footer.append(el(document, 'span', '', `当前展示 ${items.length} 条 · 最多保留最近 500 条在浏览器内存中`), el(document, 'span', '', '服务端每 2 秒检查一次追加内容，不重复读取整份日志'))
    root.append(footer)
  }

  function render(): void {
    if (disposed) return
    if (options.view === 'permissions') return
    const active = document.activeElement as HTMLElement | null
    const focusId = active?.dataset.focusId
    const activeInput = active?.tagName === 'INPUT' ? active as HTMLInputElement : null
    const selectionStart = activeInput?.selectionStart ?? null
    const selectionEnd = activeInput?.selectionEnd ?? null
    root.replaceChildren()
    if (options.view === 'warnings') renderWarnings()
    else renderLogs()
    if (focusId) {
      const replacement = Array.from(root.querySelectorAll<HTMLElement>('[data-focus-id]')).find(item => item.dataset.focusId === focusId)
      replacement?.focus()
      if (replacement?.tagName === 'INPUT' && selectionStart !== null && selectionEnd !== null) {
        (replacement as HTMLInputElement).setSelectionRange(selectionStart, selectionEnd)
      }
    }
  }

  function onInput(event: Event): void {
    const target = event.target as HTMLInputElement | HTMLSelectElement | null
    if (target?.dataset.focusId === 'log-search') {
      state.searchQuery = target.value
      render()
    }
  }

  function onChange(event: Event): void {
    const target = event.target as HTMLSelectElement | null
    if (target?.dataset.focusId === 'log-level-filter') {
      state.levelFilter = target.value
      render()
    }
  }

  function onClick(event: Event): void {
    const target = event.target as HTMLElement | null
    const control = target?.closest<HTMLButtonElement>('button[data-action]')
    if (!control || control.disabled) return
    switch (control.dataset.action) {
      case 'refresh-warnings': void loadWarnings(); break
      case 'refresh-logs': void loadLogs(false); break
      case 'source-server':
        if (state.activeSource !== 'server') { state.activeSource = 'server'; state.paused = false; void loadLogs(true) }
        break
      case 'source-ssh':
        if (state.activeSource !== 'ssh') { state.activeSource = 'ssh'; state.paused = false; void loadLogs(true) }
        break
      case 'toggle-pause': togglePause(); break
    }
  }

  root.addEventListener('click', onClick, { signal: listeners.signal })
  root.addEventListener('input', onInput, { signal: listeners.signal })
  root.addEventListener('change', onChange, { signal: listeners.signal })

  if (options.view === 'warnings') {
    ready = loadWarnings()
    warningTimer = window.setInterval(() => { if (!document.hidden) void loadWarnings() }, 15000)
  } else if (options.view === 'logs') {
    ready = loadLogs(true)
  } else {
    const controller = mountPermissionManagement(root)
    childDispose = controller.dispose
    ready = controller.ready
  }

  return {
    get ready() { return ready },
    dispose() {
      if (disposed) return
      disposed = true
      logLoadSequence += 1
      closeStream()
      if (warningTimer !== undefined) window.clearInterval(warningTimer)
      childDispose?.()
      childDispose = null
      listeners.abort()
      host.replaceChildren()
    },
  }
}
