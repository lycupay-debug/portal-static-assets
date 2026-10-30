/**
 * Responsibility: render account overview, profile and session-security summaries in native DOM.
 * Dependencies: the authenticated user snapshot and logout callback supplied by the application shell.
 * Collaboration: App.vue owns session retrieval, authentication state and logout requests.
 * Boundaries: user data is inserted as text only; this view does not infer permissions or persist account data.
 */
export type AccountIdentityUser = { email: string; displayName?: string | null; createdAt: string }
export type AccountIdentityState = { section: 'overview' | 'profile' | 'security'; loading: boolean; busy: boolean; user: AccountIdentityUser | null }

export function mountAccountIdentityView(host: HTMLElement, options: { getState: () => AccountIdentityState; logout: () => unknown }) {
  const document = host.ownerDocument
  const render = (state: AccountIdentityState) => {
    host.replaceChildren()
    const main = document.createElement('section')
    main.className = `account-identity-view account-identity-${state.section}`
    const title = document.createElement('h1')
    title.className = 'account-page-title'
    title.textContent = state.section === 'overview' ? '账户概览' : state.section === 'profile' ? '个人资料' : '登录与安全'
    const intro = document.createElement('p')
    intro.className = 'auth-intro'
    intro.textContent = state.section === 'overview' ? '查看你的账户状态与基础信息。' : state.section === 'profile' ? '当前账户的基本资料。' : '账户登录状态和会话管理。'
    main.append(title, intro)
    if (state.loading) {
      const loading = document.createElement('div'); loading.className = 'account-loading'; loading.textContent = '正在读取账户信息…'; main.append(loading)
    } else if (state.user) {
      const user = state.user
      const details = document.createElement('div')
      details.className = 'account-details'
      const summary = document.createElement('div')
      summary.className = 'account-summary-card'
      const avatar = document.createElement('div')
      avatar.className = 'account-avatar'
      avatar.textContent = (user.displayName || user.email).slice(0, 1).toUpperCase()
      const identity = document.createElement('div')
      const name = document.createElement('strong'); name.className = 'account-summary-name'; name.textContent = user.displayName || '未设置昵称'
      const email = document.createElement('span'); email.className = 'account-summary-email'; email.textContent = user.email
      identity.append(name, email)
      summary.append(avatar, identity)
      if (state.section === 'overview') {
        const status = document.createElement('span'); status.className = 'account-status'
        const dot = document.createElement('i'); dot.setAttribute('aria-hidden', 'true'); status.append(dot, document.createTextNode(' 已登录'))
        summary.append(status)
      }
      details.append(summary)
      const addField = (label: string, value: string) => {
        const field = document.createElement('div'); field.className = 'account-field'
        const fieldLabel = document.createElement('span'); fieldLabel.textContent = label
        const fieldValue = document.createElement('strong'); fieldValue.textContent = value
        field.append(fieldLabel, fieldValue); details.append(field)
      }
      if (state.section === 'overview') addField('账户昵称', user.displayName || '未设置')
      if (state.section !== 'overview') addField(state.section === 'profile' ? '昵称' : '登录邮箱', state.section === 'profile' ? user.displayName || '未设置' : user.email)
      if (state.section === 'profile') addField('登录邮箱', user.email)
      addField('注册时间', new Date(user.createdAt).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }))
      if (state.section === 'security') {
        const security = document.createElement('div'); security.className = 'security-info-row'
        const icon = document.createElement('span'); icon.className = 'security-info-icon bi bi-shield-check'; icon.setAttribute('aria-hidden', 'true')
        const copy = document.createElement('div')
        const heading = document.createElement('strong'); heading.textContent = '账户已验证登录'
        const description = document.createElement('p'); description.textContent = '当前浏览器持有安全会话。'
        copy.append(heading, description); security.append(icon, copy); details.append(security)
        addField('会话有效期', '系统自动管理')
      }
      if (state.section !== 'profile') {
        const button = document.createElement('button'); button.type = 'button'; button.className = 'btn button button-primary auth-submit'; button.disabled = state.busy
        button.textContent = state.busy ? '正在退出…' : state.section === 'security' ? '退出当前登录' : '退出登录'
        button.addEventListener('click', () => options.logout())
        details.append(button)
      }
      main.append(details)
    }
    host.replaceChildren(main)
  }
  render(options.getState())
  return { update: render, dispose() { host.replaceChildren() } }
}
