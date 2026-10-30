/**
 * Responsibility: render account navigation groups and active-section state in native DOM.
 * Dependencies: filtered account navigation configuration and route/expansion callbacks.
 * Collaboration: portalApplication.ts owns permission filtering and section routing; this view only projects navigation state.
 * Boundaries: no authorization is decided here; restricted groups are removed by the caller based on server session.
 */
import type { AccountNavigationGroup } from '../navigation/accountNavigation'

export type AccountSidebarState = { groups: AccountNavigationGroup[]; active: string; expanded: string[] }

export function mountAccountSidebarView(host: HTMLElement, options: {
  getState: () => AccountSidebarState
  toggleGroup: (id: string) => void
  selectSection: (id: string) => void
  navigate: (target: string) => unknown
}) {
  const document = host.ownerDocument
  const render = (state: AccountSidebarState) => {
    host.replaceChildren()
    for (const group of state.groups) {
      const wrapper = document.createElement('div')
      wrapper.className = 'account-nav-group'
      const heading = document.createElement('button')
      heading.type = 'button'
      heading.className = `account-sidebar-item account-group-toggle${state.active === group.id ? ' is-active' : ''}`
      heading.setAttribute('aria-expanded', String(state.expanded.includes(group.id)))
      const icon = document.createElement('span')
      icon.className = `sidebar-group-icon bi ${accountGroupIcon(group.id)}`
      icon.setAttribute('aria-hidden', 'true')
      heading.append(icon, document.createTextNode(group.label))
      heading.addEventListener('click', () => options.toggleGroup(group.id))
      const children = document.createElement('div')
      children.className = 'account-sidebar-collapse'
      children.style.display = state.expanded.includes(group.id) ? '' : 'none'
      for (const item of group.items) {
        const button = document.createElement('button')
        button.type = 'button'
        button.className = `account-sidebar-item account-sidebar-subitem${state.active === item.id ? ' is-active' : ''}`
        const dot = document.createElement('span')
        dot.className = 'sidebar-item-dot'
        dot.setAttribute('aria-hidden', 'true')
        button.append(dot, document.createTextNode(item.label))
        button.addEventListener('click', () => options.selectSection(item.id))
        children.append(button)
      }
      wrapper.append(heading, children)
      host.append(wrapper)
    }
    const home = document.createElement('a')
    home.className = 'account-sidebar-home'
    home.href = '/'
    home.textContent = '返回首页'
    home.addEventListener('click', event => { event.preventDefault(); options.navigate('/') })
    host.append(home)
  }
  render(options.getState())
  return { update: render, dispose() { host.replaceChildren() } }
}

function accountGroupIcon(groupId: string): string {
  const icons: Record<string, string> = {
    personal: 'bi-person', publishing: 'bi-journal-text', housing: 'bi-house-door',
    education: 'bi-mortarboard', software: 'bi-tools', workflows: 'bi-diagram-3', compliance: 'bi-shield-check', 'system-ops': 'bi-activity',
  }
  return icons[groupId] || 'bi-grid'
}
