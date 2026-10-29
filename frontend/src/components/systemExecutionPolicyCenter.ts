/**
 * Responsibility: render the system execution-policy and command-task console using native DOM.
 * Dependencies: same-origin apiRequest transport and a host-provided DOM; styles are loaded by SystemOpsCenter.
 * Collaboration: SystemOpsCenter owns tab visibility and mount/dispose; this module owns policy/task view state,
 * polling lifecycle, validation, and explicit user-triggered API actions. The Go backend remains authoritative.
 * Boundaries: task creation, approval, cancellation, and policy writes require explicit UI actions; approval and
 * cancellation retain a browser confirmation gate. No shell command is executed by this frontend module.
 * Side effects: reads policy/tasks on mount and polls tasks every five seconds while visible; dispose clears the
 * interval, aborts DOM listeners, and invalidates pending responses. In-flight server writes are not cancellable.
 */
import { apiRequest } from '../api/client'

type Mode = 'developer' | 'stable'
type PolicyResponse = {
  mode: Mode
  updatedAt: string
  changedBy: string
  enforcement: 'policy_source_only' | 'executor_connected' | 'executor_disconnected'
  executorStatus: string
  executorLastSeen?: string
}
type TaskClass = 'read_only' | 'file_inspection' | 'test' | 'build' | 'code_change' | 'system_admin' | 'deploy' | 'restart' | 'database_change' | 'destructive' | 'ssh_mcp'
type CommandTask = {
  taskId: string
  idempotencyKey: string
  title: string
  taskClass: TaskClass
  executable: string
  arguments: string[]
  workingDirectory: string
  timeoutSeconds: number
  requestedBy: string
  modeAtSubmission: Mode
  status: string
  approvalRequired: boolean
  approvedBy?: string
  createdAt: string
  updatedAt: string
  startedAt?: string
  finishedAt?: string
  exitCode?: number
  stdout: string
  stderr: string
  attempt: number
  failureReason?: string
}
type TaskDraft = {
  title: string
  taskClass: TaskClass
  executable: string
  argumentsText: string
  workingDirectory: string
  timeoutSeconds: number
}
type PolicyState = {
  policy: PolicyResponse | null
  loading: boolean
  saving: boolean
  error: string
  notice: string
  selectedMode: Mode
  tasks: CommandTask[]
  tasksLoading: boolean
  tasksError: string
  tasksNotice: string
  taskSubmitting: boolean
  taskDraft: TaskDraft
}

export type SystemExecutionPolicyController = { ready: Promise<void>; refreshTasks: () => Promise<void>; dispose: () => void }

function el<K extends keyof HTMLElementTagNameMap>(document: Document, tag: K, className = '', text = ''): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text) element.textContent = text
  return element
}

function button(document: Document, label: string, action: string, className = 'policy-button', disabled = false): HTMLButtonElement {
  const element = el(document, 'button', className, label)
  element.type = 'button'
  element.dataset.action = action
  element.dataset.focusId = action
  element.disabled = disabled
  return element
}

export function mountSystemExecutionPolicyCenter(host: HTMLElement): SystemExecutionPolicyController {
  const document = host.ownerDocument
  const view = el(document, 'section', 'execution-policy')
  const listeners = new document.defaultView!.AbortController()
  const state: PolicyState = {
    policy: null,
    loading: true,
    saving: false,
    error: '',
    notice: '',
    selectedMode: 'stable',
    tasks: [],
    tasksLoading: false,
    tasksError: '',
    tasksNotice: '',
    taskSubmitting: false,
    taskDraft: {
      title: '只读诊断：系统版本',
      taskClass: 'read_only',
      executable: '/usr/bin/uname',
      argumentsText: '["-a"]',
      workingDirectory: '/opt/personal-portal',
      timeoutSeconds: 120,
    },
  }
  let disposed = false
  let policySequence = 0
  let taskSequence = 0
  let taskRefreshTimer: number | undefined
  host.replaceChildren(view)

  async function request<T>(path: string, init?: RequestInit): Promise<T> {
    return apiRequest<T>(path, { cache: 'no-store', ...init })
  }

  function isDirty(): boolean {
    return state.policy !== null && state.selectedMode !== state.policy.mode
  }

  async function loadPolicy(): Promise<void> {
    const sequence = ++policySequence
    state.loading = true
    state.error = ''
    render()
    try {
      const result = await request<PolicyResponse>('/system-ops/policy')
      if (disposed || sequence !== policySequence) return
      state.policy = result
      state.selectedMode = result.mode
    } catch (error) {
      if (disposed || sequence !== policySequence) return
      state.error = error instanceof Error ? error.message : '读取运行策略失败'
    } finally {
      if (!disposed && sequence === policySequence) {
        state.loading = false
        render()
      }
    }
  }

  async function saveMode(): Promise<void> {
    if (!isDirty() || state.saving) return
    state.saving = true
    state.error = ''
    state.notice = ''
    render()
    try {
      const result = await request<PolicyResponse>('/system-ops/policy', {
        method: 'PUT',
        body: JSON.stringify({ mode: state.selectedMode }),
      })
      if (disposed) return
      state.policy = result
      state.selectedMode = result.mode
      state.notice = '运行模式已保存，后端已记录本次策略变更。'
    } catch (error) {
      if (disposed) return
      const saveError = error instanceof Error ? error.message : '保存运行策略失败'
      await loadPolicy()
      if (disposed) return
      const reloadError = state.error
      state.error = reloadError ? `${saveError}（重新读取当前策略也失败：${reloadError}）` : saveError
    } finally {
      if (!disposed) {
        state.saving = false
        render()
      }
    }
  }

  async function loadTasks(silent = false): Promise<void> {
    const sequence = ++taskSequence
    if (!silent) state.tasksLoading = true
    state.tasksError = ''
    render()
    try {
      const result = await request<{ items: CommandTask[] }>('/system-ops/tasks')
      if (disposed || sequence !== taskSequence) return
      state.tasks = result.items || []
    } catch (error) {
      if (disposed || sequence !== taskSequence) return
      state.tasksError = error instanceof Error ? error.message : '读取任务队列失败'
    } finally {
      if (!disposed && sequence === taskSequence) {
        state.tasksLoading = false
        render()
      }
    }
  }

  async function createTask(): Promise<void> {
    if (state.taskSubmitting) return
    state.taskSubmitting = true
    state.tasksError = ''
    state.tasksNotice = ''
    render()
    try {
      let args: unknown
      try {
        args = JSON.parse(state.taskDraft.argumentsText)
      } catch {
        throw new Error('参数必须是合法 JSON 数组，例如 ["-a"]')
      }
      if (!Array.isArray(args) || args.some(value => typeof value !== 'string')) {
        throw new Error('参数必须是字符串数组')
      }
      const result = await request<{ taskId: string; status: string; approvalRequired: boolean; policyDecision: string; mode: Mode }>('/system-ops/tasks', {
        method: 'POST',
        body: JSON.stringify({
          idempotencyKey: `task-${Date.now()}-${Math.random().toString(36).slice(2, 12)}`,
          title: state.taskDraft.title,
          taskClass: state.taskDraft.taskClass,
          executable: state.taskDraft.executable,
          arguments: args,
          workingDirectory: state.taskDraft.workingDirectory,
          timeoutSeconds: Number(state.taskDraft.timeoutSeconds),
          shellMode: false,
        }),
      })
      if (disposed) return
      state.tasksNotice = result.status === 'manual_only'
        ? '任务已登记为人工处理，执行器不会代为处理 SSH_MCP。'
        : result.approvalRequired ? '任务已进入待审批队列；审批后才会执行。' : '任务已进入执行队列。'
      await loadTasks(true)
    } catch (error) {
      if (!disposed) state.tasksError = error instanceof Error ? error.message : '提交任务失败'
    } finally {
      if (!disposed) {
        state.taskSubmitting = false
        render()
      }
    }
  }

  async function approveTask(task: CommandTask): Promise<void> {
    const message = `确认审批并执行任务“${task.title}”？\n分类：${task.taskClass}\n程序：${task.executable}\n参数：${JSON.stringify(task.arguments)}\n工作目录：${task.workingDirectory}`
    if (!document.defaultView!.confirm(message)) return
    state.tasksError = ''
    state.tasksNotice = ''
    render()
    try {
      await request(`/system-ops/tasks/${encodeURIComponent(task.taskId)}/approve`, { method: 'POST', body: '{}' })
      if (disposed) return
      state.tasksNotice = '审批已记录，任务已进入执行队列。'
      await loadTasks(true)
    } catch (error) {
      if (!disposed) state.tasksError = error instanceof Error ? error.message : '审批任务失败'
    } finally {
      if (!disposed) render()
    }
  }

  async function cancelTask(task: CommandTask): Promise<void> {
    if (!document.defaultView!.confirm(`确认取消任务“${task.title}”？运行中的任务将尝试终止整个进程组。`)) return
    state.tasksError = ''
    render()
    try {
      await request(`/system-ops/tasks/${encodeURIComponent(task.taskId)}/cancel`, { method: 'POST', body: '{}' })
      if (disposed) return
      state.tasksNotice = '取消请求已提交。'
      await loadTasks(true)
    } catch (error) {
      if (!disposed) state.tasksError = error instanceof Error ? error.message : '取消任务失败'
    } finally {
      if (!disposed) render()
    }
  }

  function taskClassLabel(value: TaskClass): string {
    return ({ read_only: '只读诊断', file_inspection: '文件检查', test: '测试', build: '构建', code_change: '代码修改', system_admin: '系统管理', deploy: '部署', restart: '重启', database_change: '数据库变更', destructive: '破坏性操作', ssh_mcp: 'SSH_MCP 人工处理' } as Record<TaskClass, string>)[value] || value
  }
  function taskStatusLabel(value: string): string {
    return ({ pending_approval: '待审批', queued: '排队中', running: '执行中', completed: '已完成', failed: '失败', cancelled: '已取消', manual_only: '需人工处理', denied: '已拒绝' } as Record<string, string>)[value] || value
  }
  function taskStatusClass(value: string): string {
    return ['completed'].includes(value) ? 'done' : ['failed', 'denied'].includes(value) ? 'failed' : ['running', 'queued', 'pending_approval'].includes(value) ? 'active' : 'neutral'
  }
  function formatTime(value?: string): string {
    if (!value) return '尚无变更记录'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
  }

  function appendLabeledInput(parent: HTMLElement, labelText: string, field: keyof TaskDraft, value: string, options: { type?: string; wide?: boolean; required?: boolean; min?: string; max?: string; rows?: number; placeholder?: string; spellcheck?: boolean } = {}) {
    const label = el(document, 'label', options.wide ? 'task-wide' : '')
    label.append(el(document, 'span', '', labelText))
    const control = options.rows
      ? el(document, 'textarea')
      : el(document, 'input')
    if (control instanceof document.defaultView!.HTMLTextAreaElement) control.rows = options.rows!
    else (control as HTMLInputElement).type = options.type || 'text'
    if (options.required !== false) control.required = true
    if (options.min) control.setAttribute('min', options.min)
    if (options.max) control.setAttribute('max', options.max)
    if (options.placeholder) control.placeholder = options.placeholder
    if (options.spellcheck !== undefined) control.spellcheck = options.spellcheck
    control.value = value
    control.dataset.field = field
    control.dataset.focusId = `field-${field}`
    label.append(control)
    parent.append(label)
  }

  function renderTaskForm(): HTMLElement {
    const form = el(document, 'form', 'task-create-form')
    appendLabeledInput(form, '任务标题', 'title', state.taskDraft.title, { required: true, min: '3', max: '160', placeholder: '描述任务目的' })

    const classLabel = el(document, 'label')
    classLabel.append(el(document, 'span', '', '任务分类'))
    const classSelect = el(document, 'select')
    classSelect.dataset.field = 'taskClass'
    classSelect.dataset.focusId = 'field-taskClass'
    const taskClasses: Array<[TaskClass, string]> = [
      ['read_only', '只读诊断'], ['file_inspection', '文件检查'], ['test', '测试'], ['build', '构建'],
      ['code_change', '代码修改'], ['system_admin', '系统管理'], ['deploy', '部署'], ['restart', '重启'],
      ['database_change', '数据库变更'], ['destructive', '破坏性操作'], ['ssh_mcp', 'SSH_MCP 人工处理'],
    ]
    for (const [value, label] of taskClasses) {
      const option = el(document, 'option', '', label)
      option.value = value
      option.selected = state.taskDraft.taskClass === value
      classSelect.append(option)
    }
    classLabel.append(classSelect)
    form.append(classLabel)

    appendLabeledInput(form, '可执行文件（绝对路径）', 'executable', state.taskDraft.executable, { wide: true, required: true, max: '512', placeholder: '/usr/bin/uname' })
    appendLabeledInput(form, '参数（JSON 字符串数组）', 'argumentsText', state.taskDraft.argumentsText, { wide: true, required: true, rows: 2, placeholder: '["-a"]', spellcheck: false })
    appendLabeledInput(form, '工作目录（限定工作区）', 'workingDirectory', state.taskDraft.workingDirectory, { wide: true, required: true, placeholder: '/opt/personal-portal' })
    appendLabeledInput(form, '超时（秒）', 'timeoutSeconds', String(state.taskDraft.timeoutSeconds), { type: 'number', required: true, min: '1', max: '1800' })

    const actions = el(document, 'div', 'task-form-actions')
    actions.append(el(document, 'span', '', 'Shell 拼接执行已禁用。开发者模式以 root 执行命令，请确认命令来源与分类。'))
    const submit = button(document, state.taskSubmitting ? '提交中…' : '提交任务', 'submit-task', 'policy-button is-primary', state.taskSubmitting)
    submit.type = 'submit'
    actions.append(submit)
    form.append(actions)
    return form
  }

  function renderTaskItem(task: CommandTask, openDetails: Set<string>, outputScroll: Map<string, number>): HTMLElement {
    const item = el(document, 'article', 'task-item')
    const head = el(document, 'div', 'task-item-head')
    const copy = el(document, 'div')
    copy.append(el(document, 'strong', '', task.title), el(document, 'p', '', `${taskClassLabel(task.taskClass)} · ${task.executable} ${task.arguments.join(' ')}`))
    head.append(copy, el(document, 'span', `task-status ${taskStatusClass(task.status)}`, taskStatusLabel(task.status)))
    item.append(head)

    const meta = el(document, 'div', 'task-meta')
    meta.append(el(document, 'span', '', `提交者：${task.requestedBy}`))
    meta.append(el(document, 'span', '', `模式：${task.modeAtSubmission === 'developer' ? '开发者' : '稳定运行'}`))
    meta.append(el(document, 'span', '', `创建：${formatTime(task.createdAt)}`))
    if (task.exitCode !== undefined) meta.append(el(document, 'span', '', `退出码：${task.exitCode}`))
    item.append(meta)
    if (task.failureReason) item.append(el(document, 'p', 'task-failure', task.failureReason))
    if (task.stdout || task.stderr) {
      const details = el(document, 'details', 'task-output')
      details.dataset.taskId = task.taskId
      details.open = openDetails.has(task.taskId)
      details.append(el(document, 'summary', '', '执行输出'))
      if (task.stdout) {
        const stdout = el(document, 'pre', '', task.stdout)
        stdout.dataset.outputKey = `${task.taskId}:stdout`
        stdout.scrollTop = outputScroll.get(stdout.dataset.outputKey) || 0
        details.append(stdout)
      }
      if (task.stderr) {
        const stderr = el(document, 'pre', 'stderr', task.stderr)
        stderr.dataset.outputKey = `${task.taskId}:stderr`
        stderr.scrollTop = outputScroll.get(stderr.dataset.outputKey) || 0
        details.append(stderr)
      }
      item.append(details)
    }
    if (['pending_approval', 'queued', 'running'].includes(task.status)) {
      const actions = el(document, 'div', 'task-item-actions')
      if (task.status === 'pending_approval') {
        const approve = button(document, '审批并执行', 'approve-task', 'policy-button is-primary')
        approve.dataset.taskId = task.taskId
        approve.dataset.focusId = `approve-${task.taskId}`
        actions.append(approve)
      }
      const cancel = button(document, task.status === 'running' ? '请求终止' : '取消任务', 'cancel-task', 'policy-button')
      cancel.dataset.taskId = task.taskId
      cancel.dataset.focusId = `cancel-${task.taskId}`
      actions.append(cancel)
      item.append(actions)
    }
    return item
  }

  function render(): void {
    if (disposed) return
    const active = document.activeElement as HTMLElement | null
    const focusId = active?.dataset.focusId
    const activeControl = active && ['INPUT', 'TEXTAREA'].includes(active.tagName) ? active as HTMLInputElement | HTMLTextAreaElement : null
    const selectionStart = activeControl?.selectionStart ?? null
    const selectionEnd = activeControl?.selectionEnd ?? null
    const openDetails = new Set(Array.from(view.querySelectorAll<HTMLDetailsElement>('.task-output[open]')).map(item => item.dataset.taskId || ''))
    const outputScroll = new Map(Array.from(view.querySelectorAll<HTMLElement>('[data-output-key]')).map(item => [item.dataset.outputKey || '', item.scrollTop]))
    view.replaceChildren()
    const heading = el(document, 'header', 'policy-heading')
    const headingCopy = el(document, 'div')
    headingCopy.append(
      el(document, 'p', 'policy-eyebrow', 'SYSTEM OPERATIONS / POLICY'),
      el(document, 'h1', 'account-page-title', '运行模式与任务策略'),
      el(document, 'p', 'policy-intro', '统一管理服务器任务的默认执行策略。此处是独立的系统运维页面，不影响系统警告和系统日志页面。'),
    )
    heading.append(headingCopy, button(document, '刷新状态', 'refresh-policy', 'policy-button', state.loading))
    view.append(heading)

    if (state.error) {
      const message = el(document, 'p', 'policy-message is-error', state.error)
      message.setAttribute('role', 'alert')
      view.append(message)
    }
    if (state.notice) {
      const message = el(document, 'p', 'policy-message is-success', state.notice)
      message.setAttribute('role', 'status')
      view.append(message)
    }
    if (state.loading) {
      view.append(el(document, 'div', 'policy-loading', '正在读取服务器运行策略…'))
      return
    }
    const policy = state.policy
    if (!policy) return

    const current = el(document, 'section', 'policy-current')
    const currentCopy = el(document, 'div')
    currentCopy.append(
      el(document, 'span', 'policy-kicker', '当前持久化模式'),
      el(document, 'strong', '', policy.mode === 'developer' ? '开发者模式' : '稳定运行模式'),
      el(document, 'p', '', policy.mode === 'developer' ? '以开发效率为优先，在既定的用户授权范围内减少逐项审批。' : '以线上稳定性为优先，按任务风险、路径范围和审批要求执行。'),
    )
    current.append(currentCopy, el(document, 'span', `policy-state-dot${policy.mode === 'developer' ? ' is-developer' : ''}`))
    current.lastElementChild?.setAttribute('aria-hidden', 'true')
    view.append(current)

    const modePanel = el(document, 'section', 'policy-mode-switch-panel')
    const modeCopy = el(document, 'div', 'policy-mode-switch-copy')
    modeCopy.append(
      el(document, 'span', 'policy-kicker', '切换运行模式'),
      el(document, 'strong', '', state.selectedMode === 'developer' ? '开发者模式' : '稳定运行模式'),
      el(document, 'p', '', state.selectedMode === 'developer' ? '优先开发效率：在明确授权范围内自主连续执行开发、构建、测试和修复。' : '优先运行稳定：按任务风险限制权限，敏感操作需要逐次审批。'),
    )
    const switchControl = el(document, 'div', 'policy-switch-control')
    switchControl.append(el(document, 'span', state.selectedMode === 'stable' ? 'active' : '', '稳定运行'))
    const toggle = button(document, '', 'toggle-mode', 'policy-switch')
    toggle.setAttribute('role', 'switch')
    toggle.setAttribute('aria-checked', String(state.selectedMode === 'developer'))
    toggle.setAttribute('aria-label', state.selectedMode === 'developer' ? '切换到稳定运行模式' : '切换到开发者模式')
    toggle.append(el(document, 'span'))
    switchControl.append(toggle, el(document, 'span', state.selectedMode === 'developer' ? 'active' : '', '开发者'))
    modePanel.append(modeCopy, switchControl)
    view.append(modePanel)

    const modeGrid = el(document, 'div', 'policy-mode-grid')
    const modes: Array<{ mode: Mode; title: string; tag: string; description: string }> = [
      { mode: 'developer', title: '开发者模式', tag: '开发阶段', description: '在用户授权的开发任务范围内，允许自主规划、连续修改、构建、测试、修复和验证，减少重复确认。' },
      { mode: 'stable', title: '稳定运行模式', tag: '生产阶段', description: '只读任务可自动执行；文件、测试、构建和代码修改受范围约束；部署、重启、数据库变更及破坏性操作按规则审批。' },
    ]
    for (const mode of modes) {
      const card = el(document, 'article', `policy-mode-card${state.selectedMode === mode.mode ? ' selected' : ''}`)
      card.append(el(document, 'strong', '', mode.title), el(document, 'span', 'policy-mode-tag', mode.tag), el(document, 'p', '', mode.description))
      modeGrid.append(card)
    }
    view.append(modeGrid)

    const rules = el(document, 'section', 'policy-rules')
    const rulesHeading = el(document, 'div', 'policy-section-heading')
    const rulesCopy = el(document, 'div')
    rulesCopy.append(el(document, 'h2', '', '强制确认规则'), el(document, 'p', '', '无论选择哪种模式，以下规则都不能被模式开关解除。'))
    rulesHeading.append(rulesCopy, el(document, 'span', '', '始终生效'))
    rules.append(rulesHeading)
    for (const [number, title, description] of [
      ['01', '重启任何服务或进程', '必须事先取得用户同意。SSH_MCP 服务由用户本人执行重启。'],
      ['02', '处理 SSH_MCP 本身', '涉及 SSH_MCP 的配置、授权、连接机制、服务或进程的变更，必须先征得用户同意。'],
    ]) {
      const row = el(document, 'div', 'policy-rule-row')
      row.append(el(document, 'span', 'policy-rule-icon', number))
      const copy = el(document, 'div')
      copy.append(el(document, 'strong', '', title), el(document, 'p', '', description))
      row.append(copy, el(document, 'span', 'policy-lock', '需确认'))
      rules.append(row)
    }
    view.append(rules)

    const taskRules = el(document, 'section', 'policy-rules')
    const taskRulesHeading = el(document, 'div', 'policy-section-heading')
    const taskRulesCopy = el(document, 'div')
    taskRulesCopy.append(el(document, 'h2', '', '任务分类规则'), el(document, 'p', '', '策略定义用于统一后端判定；稳定运行模式不会把高风险操作自动降级为普通任务。'))
    taskRulesHeading.append(taskRulesCopy)
    taskRules.append(taskRulesHeading)
    const taskRuleList = el(document, 'div', 'policy-task-list')
    for (const [title, description] of [
      ['只读诊断与文件检查', '开发者模式：授权范围内自动 · 稳定模式：只读权限与路径范围'],
      ['测试与构建', '开发者模式：自主执行并修复 · 稳定模式：登记测试任务、限定构建工作区'],
      ['代码修改与系统管理', '开发者模式：按本次开发授权执行 · 稳定模式：限定文件、资源和操作范围'],
      ['部署、数据库变更、破坏性操作', '开发者模式：除强制确认项外按明确授权执行 · 稳定模式：逐次审批'],
    ]) {
      const row = el(document, 'div')
      row.append(el(document, 'strong', '', title), el(document, 'span', '', description))
      taskRuleList.append(row)
    }
    taskRules.append(taskRuleList)
    view.append(taskRules)

    const boundary = el(document, 'aside', 'policy-boundary')
    boundary.append(
      el(document, 'strong', '', '执行边界与权限说明'),
      el(document, 'p', '', '命令任务由独立的 root 执行器领取，但不通过 Shell 拼接执行；请求使用明确的可执行文件与参数数组。开发者模式会以 root 身份执行授权范围内的命令，稳定模式默认降权到 personalportal 账户，仅通过逐次审批及分类白名单的高风险任务会提升权限。可识别的直接命令重启特征会在后端重新分类；SSH_MCP 任务只登记、不代执行。任务队列禁用 Shell 字符串拼接，但脚本、测试和构建本身仍可能产生副作用；不要提交不可信脚本或来源不明的代码。'),
    )
    view.append(boundary)

    const taskConsole = el(document, 'section', 'policy-rules task-console')
    const consoleHeading = el(document, 'div', 'policy-section-heading')
    const consoleCopy = el(document, 'div')
    consoleCopy.append(el(document, 'h2', '', '服务器任务队列'), el(document, 'p', '', '任务通过 PostgreSQL 持久化，沿用现有事件核心进行唤醒，并由独立执行器处理。待审批任务不会自动执行。'))
    consoleHeading.append(consoleCopy, button(document, state.tasksLoading ? '刷新中…' : '刷新任务', 'refresh-tasks', 'policy-button', state.tasksLoading))
    taskConsole.append(consoleHeading)
    if (state.tasksError) {
      const message = el(document, 'p', 'policy-message is-error', state.tasksError)
      message.setAttribute('role', 'alert')
      taskConsole.append(message)
    }
    if (state.tasksNotice) {
      const message = el(document, 'p', 'policy-message is-success', state.tasksNotice)
      message.setAttribute('role', 'status')
      taskConsole.append(message)
    }
    taskConsole.append(renderTaskForm())
    if (state.tasks.length) {
      const list = el(document, 'div', 'task-list')
      for (const task of state.tasks) list.append(renderTaskItem(task, openDetails, outputScroll))
      taskConsole.append(list)
    } else if (!state.tasksLoading) {
      taskConsole.append(el(document, 'p', 'task-empty', '任务队列为空。提交只读诊断或受限测试任务进行验证。'))
    }
    view.append(taskConsole)

    const footer = el(document, 'footer', 'policy-footer')
    const updated = el(document, 'div')
    updated.append(el(document, 'span', '', '最近变更'), el(document, 'strong', '', formatTime(policy.updatedAt)), el(document, 'small', '', `操作者：${policy.changedBy || '系统初始化'}`))
    const enforcement = policy.enforcement === 'executor_connected' ? '执行器在线' : policy.enforcement === 'executor_disconnected' ? '执行器离线' : '执行器尚未接入'
    const status = el(document, 'div')
    status.append(el(document, 'span', '', '策略保存状态'), el(document, 'strong', '', enforcement), el(document, 'small', '', `执行器状态：${policy.executorStatus || '未知'} · 当前模式：${policy.mode === 'developer' ? '开发者模式' : '稳定运行模式'}`))
    footer.append(updated, status)
    view.append(footer)

    const actions = el(document, 'div', 'policy-actions')
    actions.append(el(document, 'span', '', isDirty() ? '有尚未保存的模式变更。' : '当前选择与服务器保存的模式一致。'))
    const actionButtons = el(document, 'div')
    const cancel = button(document, '取消更改', 'reset-mode', 'policy-button', state.saving || !isDirty())
    const save = button(document, state.saving ? '正在保存…' : '保存运行模式', 'save-mode', 'policy-button is-primary', state.saving || !isDirty())
    actionButtons.append(cancel, save)
    actions.append(actionButtons)
    view.append(actions)

    if (focusId) {
      const replacement = Array.from(view.querySelectorAll<HTMLElement>('[data-focus-id]')).find(item => item.dataset.focusId === focusId)
      replacement?.focus()
      if (replacement && ['INPUT', 'TEXTAREA'].includes(replacement.tagName) && selectionStart !== null && selectionEnd !== null) {
        (replacement as HTMLInputElement | HTMLTextAreaElement).setSelectionRange(selectionStart, selectionEnd)
      }
    }
  }

  function onInput(event: Event): void {
    const target = event.target as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null
    const field = target?.dataset.field as keyof TaskDraft | undefined
    if (!target || !field) return
    if (field === 'timeoutSeconds') state.taskDraft.timeoutSeconds = Number(target.value)
    else if (field === 'taskClass') state.taskDraft.taskClass = target.value as TaskClass
    else if (field === 'title' || field === 'executable' || field === 'workingDirectory') state.taskDraft[field] = target.value.trim()
    else if (field === 'argumentsText') state.taskDraft.argumentsText = target.value
  }

  function onSubmit(event: Event): void {
    const target = event.target as HTMLFormElement | null
    if (target?.classList.contains('task-create-form')) {
      event.preventDefault()
      void createTask()
    }
  }

  function onClick(event: Event): void {
    const target = event.target as HTMLElement | null
    const control = target?.closest<HTMLButtonElement>('button[data-action]')
    if (!control || control.disabled) return
    switch (control.dataset.action) {
      case 'refresh-policy': void loadPolicy(); break
      case 'refresh-tasks': void loadTasks(); break
      case 'toggle-mode': state.selectedMode = state.selectedMode === 'developer' ? 'stable' : 'developer'; render(); break
      case 'reset-mode': if (state.policy) { state.selectedMode = state.policy.mode; render() } break
      case 'save-mode': void saveMode(); break
      case 'approve-task': {
        const task = state.tasks.find(item => item.taskId === control.dataset.taskId)
        if (task) void approveTask(task)
        break
      }
      case 'cancel-task': {
        const task = state.tasks.find(item => item.taskId === control.dataset.taskId)
        if (task) void cancelTask(task)
        break
      }
    }
  }

  view.addEventListener('input', onInput, { signal: listeners.signal })
  view.addEventListener('change', onInput, { signal: listeners.signal })
  view.addEventListener('submit', onSubmit, { signal: listeners.signal })
  view.addEventListener('click', onClick, { signal: listeners.signal })

  const ready = Promise.all([loadPolicy(), loadTasks()]).then(() => undefined)
  taskRefreshTimer = document.defaultView!.setInterval(() => {
    if (!disposed && !document.hidden) void loadTasks(true)
  }, 5000)

  return {
    ready,
    refreshTasks: () => loadTasks(true),
    dispose() {
      if (disposed) return
      disposed = true
      policySequence += 1
      taskSequence += 1
      if (taskRefreshTimer !== undefined) document.defaultView!.clearInterval(taskRefreshTimer)
      listeners.abort()
      host.replaceChildren()
    },
  }
}
