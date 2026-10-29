/**
 * Responsibility: render and operate the administrator permission-management workspace with native DOM.
 * Dependencies: same-origin apiRequest transport; host-provided DOM; permission-management CSS.
 * Collaboration: SystemOpsCenter owns when this view is mounted; this module owns its local view state,
 * loading/saving lifecycle, selection synchronization, and event forwarding to the Go admin API.
 * Boundaries: client-side super-admin guards preserve the existing UI policy only; Go remains the
 * authorization authority. All server-provided strings are inserted as text, never parsed as HTML.
 * Side effects: performs three read requests on mount and PATCH requests only after explicit save clicks;
 * dispose aborts DOM listeners and invalidates pending responses but does not cancel server requests.
 */
import { apiRequest } from '../api/client'

type PortalUser = { id: string; email: string; displayName: string; isAdmin: boolean; isCopyrightReviewer: boolean; createdAt: string; roles: string[] }
type PortalRole = { roleKey: string; displayName: string; description: string; isSystem: boolean; permissions: string[] }
type PortalPermission = { permissionKey: string; description: string }
type PermissionState = {
  loading: boolean
  saving: boolean
  error: string
  notice: string
  users: PortalUser[]
  roles: PortalRole[]
  permissions: PortalPermission[]
  selectedRole: string
  selectedUser: string
  selectedUserRoles: string[]
  selectedRolePermissions: string[]
  query: string
}

export type PermissionManagementController = {
  ready: Promise<void>
  refresh: () => Promise<void>
  dispose: () => void
}

function createElement<K extends keyof HTMLElementTagNameMap>(
  document: Document,
  tag: K,
  className = '',
  text = '',
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text) element.textContent = text
  return element
}

function createButton(document: Document, label: string, action: string, className: string, disabled: boolean): HTMLButtonElement {
  const button = createElement(document, 'button', `btn ${className}`, label)
  button.type = 'button'
  button.dataset.action = action
  button.disabled = disabled
  return button
}

export function mountPermissionManagement(host: HTMLElement): PermissionManagementController {
  const document = host.ownerDocument
  const root = createElement(document, 'section', 'permission-center')
  const listeners = new document.defaultView!.AbortController()
  const state: PermissionState = {
    loading: true,
    saving: false,
    error: '',
    notice: '',
    users: [],
    roles: [],
    permissions: [],
    selectedRole: '',
    selectedUser: '',
    selectedUserRoles: [],
    selectedRolePermissions: [],
    query: '',
  }
  let disposed = false
  let requestSequence = 0

  host.replaceChildren(root)

  async function api<T>(path: string, init?: RequestInit): Promise<T> {
    return apiRequest<T>(path, { cache: 'no-store', ...init })
  }

  function syncSelections() {
    const user = state.users.find(item => item.id === state.selectedUser)
    state.selectedUserRoles = user ? [...user.roles] : []
    const currentRole = state.roles.find(item => item.roleKey === state.selectedRole)
    state.selectedRolePermissions = currentRole ? [...currentRole.permissions] : []
  }

  async function refresh(): Promise<void> {
    const sequence = ++requestSequence
    state.loading = true
    state.error = ''
    render()
    try {
      const [userResult, roleResult, permissionResult] = await Promise.all([
        api<{ items: PortalUser[] }>('/admin/users'),
        api<{ items: PortalRole[] }>('/admin/roles'),
        api<{ items: PortalPermission[] }>('/admin/permissions'),
      ])
      if (disposed || sequence !== requestSequence) return
      state.users = userResult.items || []
      state.roles = roleResult.items || []
      state.permissions = permissionResult.items || []
      if (!state.selectedRole || !state.roles.some(item => item.roleKey === state.selectedRole)) {
        state.selectedRole = state.roles[0]?.roleKey || ''
      }
      if (!state.selectedUser || !state.users.some(item => item.id === state.selectedUser)) {
        state.selectedUser = state.users.find(item => item.email === 'lycupay@gmail.com')?.id || state.users[0]?.id || ''
      }
      syncSelections()
    } catch (error) {
      if (disposed || sequence !== requestSequence) return
      state.error = error instanceof Error ? error.message : '读取权限数据失败'
    } finally {
      if (!disposed && sequence === requestSequence) {
        state.loading = false
        render()
      }
    }
  }

  async function saveUserRoles(): Promise<void> {
    const target = state.users.find(item => item.id === state.selectedUser)
    if (!target) return
    if (target.email === 'lycupay@gmail.com' && !state.selectedUserRoles.includes('super_admin')) {
      state.error = '指定超级管理员必须保留 super_admin 角色。'
      render()
      return
    }
    if (target.email !== 'lycupay@gmail.com' && state.selectedUserRoles.includes('super_admin')) {
      state.error = '系统策略只允许 lycupay@gmail.com 担任超级管理员。'
      render()
      return
    }
    state.saving = true
    state.error = ''
    state.notice = ''
    render()
    try {
      await api(`/admin/users/${encodeURIComponent(target.id)}/roles`, {
        method: 'PATCH',
        body: JSON.stringify({ roles: state.selectedUserRoles }),
      })
      state.notice = `${target.email} 的角色已更新。`
      await refresh()
    } catch (error) {
      state.error = error instanceof Error ? error.message : '保存角色失败'
    } finally {
      if (!disposed) {
        state.saving = false
        render()
      }
    }
  }

  async function saveRolePermissions(): Promise<void> {
    if (!state.selectedRole || state.selectedRole === 'super_admin') return
    state.saving = true
    state.error = ''
    state.notice = ''
    render()
    try {
      await api(`/admin/roles/${encodeURIComponent(state.selectedRole)}/permissions`, {
        method: 'PATCH',
        body: JSON.stringify({ permissions: state.selectedRolePermissions }),
      })
      const role = state.roles.find(item => item.roleKey === state.selectedRole)
      state.notice = `角色“${role?.displayName || state.selectedRole}”的权限已更新。`
      await refresh()
    } catch (error) {
      state.error = error instanceof Error ? error.message : '保存权限失败'
    } finally {
      if (!disposed) {
        state.saving = false
        render()
      }
    }
  }

  function appendCheckbox(
    parent: HTMLElement,
    label: string,
    description: string,
    value: string,
    checked: boolean,
    disabled: boolean,
    selection: 'user-role' | 'role-permission',
  ) {
    const wrapper = createElement(document, 'label', 'permission-check')
    const input = createElement(document, 'input')
    input.type = 'checkbox'
    input.value = value
    input.checked = checked
    input.disabled = disabled
    input.dataset.selection = selection
    const copy = createElement(document, 'span')
    copy.append(createElement(document, 'strong', '', label), createElement(document, 'small', '', description))
    wrapper.append(input, copy)
    parent.append(wrapper)
  }

  function renderUserPanel(): HTMLElement {
    const panel = createElement(document, 'section', 'permission-panel')
    const heading = createElement(document, 'header')
    heading.append(createElement(document, 'h2', '', '账户角色'), createElement(document, 'span', '', `${state.users.length} 个账户`))
    panel.append(heading)

    const search = createElement(document, 'input', 'form-control permission-search')
    search.type = 'search'
    search.placeholder = '搜索邮箱或昵称'
    search.setAttribute('aria-label', '搜索账户')
    search.dataset.focusKey = 'user-search'
    search.value = state.query
    panel.append(search)

    const userList = createElement(document, 'div', 'permission-user-list')
    const term = state.query.trim().toLocaleLowerCase()
    const filteredUsers = state.users.filter(user => !term
      || user.email.toLocaleLowerCase().includes(term)
      || user.displayName.toLocaleLowerCase().includes(term))
    for (const user of filteredUsers) {
      const button = createElement(document, 'button', `permission-user${user.id === state.selectedUser ? ' active' : ''}`)
      button.type = 'button'
      button.disabled = state.saving
      button.dataset.action = 'choose-user'
      button.dataset.userId = user.id
      button.append(
        createElement(document, 'span', 'permission-user-name', user.displayName || '未设置昵称'),
        createElement(document, 'span', 'permission-user-email', user.email),
      )
      const tags = createElement(document, 'span', 'permission-tags')
      for (const key of user.roles) {
        const role = state.roles.find(item => item.roleKey === key)
        tags.append(createElement(document, 'i', '', role?.displayName || key))
      }
      button.append(tags)
      userList.append(button)
    }
    if (!filteredUsers.length) userList.append(createElement(document, 'p', 'permission-empty', '没有匹配账户'))
    panel.append(userList)

    const selectedUser = state.users.find(item => item.id === state.selectedUser)
    if (selectedUser) {
      const editor = createElement(document, 'div', 'permission-editor')
      editor.append(createElement(document, 'h3', '', '分配角色'))
      for (const role of state.roles) {
        appendCheckbox(editor, role.displayName, role.description, role.roleKey,
          state.selectedUserRoles.includes(role.roleKey), role.roleKey === 'super_admin' || state.saving, 'user-role')
      }
      editor.append(createElement(document, 'p', 'permission-footnote', '超级管理员只能由指定账户担任，不能通过此界面转授。'))
      editor.append(createButton(document, state.saving ? '正在保存…' : '保存账户角色', 'save-user-roles', 'permission-primary', state.saving))
      panel.append(editor)
    }
    return panel
  }

  function renderRolePanel(): HTMLElement {
    const panel = createElement(document, 'section', 'permission-panel')
    const heading = createElement(document, 'header')
    heading.append(createElement(document, 'h2', '', '角色权限'), createElement(document, 'span', '', `${state.permissions.length} 项权限`))
    panel.append(heading)

    const label = createElement(document, 'label', 'permission-field-label', '选择角色')
    label.htmlFor = 'permission-role'
    const select = createElement(document, 'select', 'form-select')
    select.id = 'permission-role'
    select.disabled = state.saving
    select.dataset.roleSelect = 'true'
    for (const role of state.roles) {
      const option = createElement(document, 'option', '', role.displayName)
      option.value = role.roleKey
      option.selected = role.roleKey === state.selectedRole
      select.append(option)
    }
    panel.append(label, select)

    const selectedRole = state.roles.find(item => item.roleKey === state.selectedRole)
    if (selectedRole) {
      const editor = createElement(document, 'div', 'permission-editor')
      editor.append(createElement(document, 'p', 'permission-role-description', selectedRole.description))
      for (const permission of state.permissions) {
        appendCheckbox(editor, permission.permissionKey, permission.description, permission.permissionKey,
          state.selectedRolePermissions.includes(permission.permissionKey), state.selectedRole === 'super_admin' || state.saving, 'role-permission')
      }
      if (state.selectedRole === 'super_admin') {
        editor.append(createElement(document, 'p', 'permission-footnote', '超级管理员保留系统完整权限，基础权限不可通过界面移除。'))
      } else {
        editor.append(createButton(document, state.saving ? '正在保存…' : '保存角色权限', 'save-role-permissions', 'permission-primary', state.saving))
      }
      panel.append(editor)
    }
    return panel
  }

  function render(): void {
    if (disposed) return
    const active = document.activeElement as HTMLElement | null
    const focusKey = active?.dataset.focusKey
    const activeInput = active?.tagName === 'INPUT' ? active as HTMLInputElement : null
    const selectionStart = activeInput?.selectionStart ?? null
    const selectionEnd = activeInput?.selectionEnd ?? null
    root.replaceChildren()

    const heading = createElement(document, 'div', 'permission-heading')
    const headingCopy = createElement(document, 'div')
    headingCopy.append(
      createElement(document, 'p', 'permission-eyebrow', 'ACCESS CONTROL'),
      createElement(document, 'h1', '', '权限管理'),
      createElement(document, 'p', '', '管理账户角色与角色权限。所有变更均由服务器端校验并写入审计日志。'),
    )
    heading.append(headingCopy, createButton(document, '刷新', 'refresh', 'permission-secondary', state.loading || state.saving))
    root.append(heading)

    if (state.error) {
      const alert = createElement(document, 'p', 'permission-alert', state.error)
      alert.setAttribute('role', 'alert')
      root.append(alert)
    }
    if (state.notice) {
      const notice = createElement(document, 'p', 'permission-notice', state.notice)
      notice.setAttribute('role', 'status')
      root.append(notice)
    }
    if (state.loading) {
      root.append(createElement(document, 'div', 'permission-empty', '正在读取权限配置…'))
    } else {
      const grid = createElement(document, 'div', 'permission-grid')
      grid.append(renderUserPanel(), renderRolePanel())
      root.append(grid)
    }

    if (focusKey) {
      const replacement = root.querySelector<HTMLElement>(`[data-focus-key="${focusKey}"]`)
      replacement?.focus()
      if (replacement?.tagName === 'INPUT' && selectionStart !== null && selectionEnd !== null) {
        (replacement as HTMLInputElement).setSelectionRange(selectionStart, selectionEnd)
      }
    }
  }

  function onClick(event: Event): void {
    const target = event.target as HTMLElement | null
    const button = target?.closest<HTMLButtonElement>('button[data-action]')
    if (!button || button.disabled) return
    switch (button.dataset.action) {
      case 'refresh': void refresh(); break
      case 'choose-user':
        state.selectedUser = button.dataset.userId || ''
        syncSelections()
        state.notice = ''
        render()
        break
      case 'save-user-roles': void saveUserRoles(); break
      case 'save-role-permissions': void saveRolePermissions(); break
    }
  }

  function onInput(event: Event): void {
    const target = event.target as HTMLInputElement | null
    if (target?.dataset.focusKey === 'user-search') {
      state.query = target.value
      render()
    }
  }

  function onChange(event: Event): void {
    const target = event.target as HTMLInputElement | HTMLSelectElement | null
    if (!target) return
    if (target.tagName === 'SELECT' && (target as HTMLSelectElement).dataset.roleSelect) {
      state.selectedRole = (target as HTMLSelectElement).value
      syncSelections()
      state.notice = ''
      render()
      return
    }
    if (target.tagName === 'INPUT' && (target as HTMLInputElement).type === 'checkbox') {
      const input = target as HTMLInputElement
      const selection = input.dataset.selection
      const values = selection === 'user-role' ? state.selectedUserRoles
        : selection === 'role-permission' ? state.selectedRolePermissions : null
      if (!values) return
      const next = input.checked
        ? [...new Set([...values, input.value])]
        : values.filter(value => value !== input.value)
      if (selection === 'user-role') state.selectedUserRoles = next
      else state.selectedRolePermissions = next
    }
  }

  root.addEventListener('click', onClick, { signal: listeners.signal })
  root.addEventListener('input', onInput, { signal: listeners.signal })
  root.addEventListener('change', onChange, { signal: listeners.signal })
  const ready = refresh()

  return {
    ready,
    refresh,
    dispose() {
      if (disposed) return
      disposed = true
      requestSequence += 1
      listeners.abort()
      host.replaceChildren()
    },
  }
}
