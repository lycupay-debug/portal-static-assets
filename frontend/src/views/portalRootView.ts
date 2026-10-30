/**
 * Responsibility: build the route-aware portal DOM shell and expose stable mount points for native page views.
 * Dependencies: a compact route snapshot, navigation/scroll callbacks, and a host-ref assignment adapter.
 * Collaboration: page controllers own their host contents; this module owns only route structure and layout classes.
 * Boundaries: user/API data is never interpolated into HTML; all nodes are created through DOM APIs. No Vue import.
 * Lifecycle: update replaces route structure only when route/section state changes; dispose clears all host refs.
 */
export type PortalRootState = {
  route: string
  contentEditorPage: boolean
  accountPage: boolean
  authPage: boolean
  publicContentPage: boolean
  legalPage: boolean
  accountSection: string
  accountTableMode: boolean
}

export type PortalRootHostName =
  | 'knowledgeTagDialogHost' | 'siteHeaderHost' | 'siteFooterHost' | 'contentWorkspaceTopbarHost'
  | 'contentEditorHost' | 'contentSettingsHost' | 'homePageHost' | 'knowledgeDirectoryHost'
  | 'directoryOverviewHost' | 'educationResourceDirectoryHost' | 'authPageHost' | 'publicArticleHost'
  | 'legalPageHost' | 'accountSidebarHost' | 'accountIdentityHost' | 'complianceCenterHost'
  | 'systemOpsHost' | 'accountTableViewHost' | 'teachingResourceEditorHost' | 'teachingResourceDrawerHost'

const HOST_NAMES: PortalRootHostName[] = [
  'knowledgeTagDialogHost', 'siteHeaderHost', 'siteFooterHost', 'contentWorkspaceTopbarHost',
  'contentEditorHost', 'contentSettingsHost', 'homePageHost', 'knowledgeDirectoryHost',
  'directoryOverviewHost', 'educationResourceDirectoryHost', 'authPageHost', 'publicArticleHost',
  'legalPageHost', 'accountSidebarHost', 'accountIdentityHost', 'complianceCenterHost',
  'systemOpsHost', 'accountTableViewHost', 'teachingResourceEditorHost', 'teachingResourceDrawerHost',
]

export function mountPortalRootView(
  host: HTMLElement,
  options: {
    getState: () => PortalRootState
    navigate: (target: string) => unknown
    scrollToTop: () => void
    setHost: (name: PortalRootHostName, element: HTMLElement | null) => void
  },
) {
  const document = host.ownerDocument
  let disposed = false
  let currentLayoutKey = ''

  function layoutKey(state: PortalRootState): string {
    if (state.contentEditorPage) return 'content-editor'
    if (state.accountPage) {
      const section = ['overview', 'profile', 'security'].includes(state.accountSection) ? 'identity'
        : state.accountSection === 'copyright-compliance' ? 'compliance'
          : ['system-warnings', 'system-logs', 'system-execution-policy'].includes(state.accountSection) ? 'system-ops' : 'table'
      return `account:${section}`
    }
    if (state.authPage) return 'auth'
    if (state.publicContentPage) return 'public-article'
    if (state.legalPage) return 'legal-document'
    if (state.route === '/') return 'home'
    if (state.route === '/knowledge') return 'knowledge-directory'
    if (state.route === '/homestays' || state.route === '/software-tools') return 'directory-overview'
    if (state.route === '/education-resources') return 'education-directory'
    return `not-found:${state.route}`
  }

  function createHost(name: PortalRootHostName, tag = 'div', className = '') {
    const element = document.createElement(tag)
    if (className) element.className = className
    options.setHost(name, element)
    return element
  }

  function link(target: string, label: string) {
    const element = document.createElement('a'); element.href = target; element.textContent = label
    element.addEventListener('click', event => { event.preventDefault(); options.navigate(target) })
    return element
  }

  function render(state: PortalRootState) {
    if (disposed) return
    currentLayoutKey = layoutKey(state)
    for (const name of HOST_NAMES) options.setHost(name, null)
    const shell = document.createElement('div'); shell.className = 'site-shell'
    shell.append(createHost('knowledgeTagDialogHost'))

    if (!state.accountPage && !state.contentEditorPage) shell.append(createHost('siteHeaderHost', 'header', 'site-header'))

    if (state.contentEditorPage) {
      const main = document.createElement('main'); main.className = 'content-workspace-page'
      main.append(createHost('contentWorkspaceTopbarHost', 'header', 'content-workspace-topbar'))
      const grid = document.createElement('div'); grid.className = 'content-workspace-grid'
      grid.append(createHost('contentEditorHost', 'div', 'content-editor-native-main'))
      const aside = document.createElement('aside'); aside.className = 'content-tools-sidebar'
      const settings = document.createElement('div'); settings.className = 'workspace-sidebar-settings'
      settings.append(createHost('contentSettingsHost'))
      const hint = document.createElement('p'); hint.className = 'context-menu-hint'; hint.textContent = '在正文中单击右键，打开统一编辑菜单。'
      settings.append(hint); aside.append(settings); grid.append(aside); main.append(grid); shell.append(main)
    } else if (state.route === '/') {
      shell.append(createHost('homePageHost'))
    } else if (state.route === '/knowledge') {
      shell.append(createHost('knowledgeDirectoryHost'))
    } else if (state.route === '/homestays' || state.route === '/software-tools') {
      shell.append(createHost('directoryOverviewHost'))
    } else if (state.route === '/education-resources') {
      shell.append(createHost('educationResourceDirectoryHost'))
    } else if (state.authPage) {
      const main = document.createElement('main'); main.className = 'route-main'; main.append(createHost('authPageHost')); shell.append(main)
    } else if (state.publicContentPage) {
      const main = document.createElement('main'); main.className = 'public-content-page'; main.append(createHost('publicArticleHost')); shell.append(main)
    } else if (state.legalPage) {
      const main = document.createElement('main'); main.className = 'legal-document-page'; main.append(createHost('legalPageHost')); shell.append(main)
    } else if (state.accountPage) {
      const main = document.createElement('main'); main.className = 'account-layout'
      main.append(createHost('accountSidebarHost', 'aside', 'account-sidebar'))
      const content = document.createElement('section'); content.className = `account-content${state.accountTableMode ? ' account-content-table-mode' : ''}`
      if (['overview', 'profile', 'security'].includes(state.accountSection)) content.append(createHost('accountIdentityHost'))
      else if (state.accountSection === 'copyright-compliance') content.append(createHost('complianceCenterHost'))
      else if (['system-warnings', 'system-logs', 'system-execution-policy'].includes(state.accountSection)) content.append(createHost('systemOpsHost'))
      else content.append(createHost('accountTableViewHost'))
      main.append(content); shell.append(main)
    } else {
      const main = document.createElement('main'); main.className = 'route-main'
      const panel = document.createElement('section'); panel.className = 'auth-panel glass-panel'
      const eyebrow = document.createElement('p'); eyebrow.className = 'eyebrow'; eyebrow.textContent = 'PAGE NOT FOUND'
      const title = document.createElement('h1'); title.textContent = '页面不存在'
      const back = link('/', '返回首页'); back.className = 'btn btn-dark button button-primary'
      panel.append(eyebrow, title, back); main.append(panel); shell.append(main)
    }

    shell.append(createHost('teachingResourceEditorHost'))
    shell.append(createHost('teachingResourceDrawerHost'))
    if (!state.accountPage && !state.contentEditorPage) shell.append(createHost('siteFooterHost', 'footer', 'site-footer'))
    const scroll = document.createElement('button'); scroll.type = 'button'; scroll.className = 'scroll-to-top'; scroll.setAttribute('aria-label', '返回页面顶部'); scroll.title = '返回顶部'
    const icon = document.createElement('i'); icon.className = 'bi bi-arrow-up'; icon.setAttribute('aria-hidden', 'true'); scroll.append(icon)
    scroll.addEventListener('click', options.scrollToTop)
    shell.append(scroll)
    host.replaceChildren(shell)
  }

  render(options.getState())
  return {
    update(state: PortalRootState) {
      if (disposed) return
      const nextLayoutKey = layoutKey(state)
      if (nextLayoutKey !== currentLayoutKey) { render(state); return }
      const accountContent = host.querySelector<HTMLElement>('.account-content')
      accountContent?.classList.toggle('account-content-table-mode', state.accountPage && state.accountTableMode)
    },
    dispose() { if (disposed) return; disposed = true; for (const name of HOST_NAMES) options.setHost(name, null); host.replaceChildren() },
  }
}
