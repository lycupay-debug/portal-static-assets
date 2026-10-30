/** Responsibility: render the public site header and responsive navigation in native DOM. */
export type SiteHeaderState = { menuOpen: boolean; loggedIn: boolean }
export function mountSiteHeaderView(host: HTMLElement, options: { getState: () => SiteHeaderState; navigate: (target: string) => unknown; setMenuOpen: (open: boolean) => void }) {
  const document = host.ownerDocument
  const render = (state: SiteHeaderState) => {
    host.replaceChildren()
    const brand = document.createElement('a'); brand.className = 'brand'; brand.href = '/'; brand.setAttribute('aria-label', '资源与服务平台首页')
    brand.addEventListener('click', event => { event.preventDefault(); options.navigate('/') })
    const mark = document.createElement('span'); mark.className = 'brand-mark'; mark.textContent = 'R.'
    const copy = document.createElement('span'); copy.className = 'brand-copy'
    const name = document.createElement('strong'); name.textContent = '资源平台'
    const subtitle = document.createElement('small'); subtitle.textContent = '资源 · 信息 · 行业服务'
    copy.append(name, subtitle); brand.append(mark, copy)
    const toggle = document.createElement('button'); toggle.type = 'button'; toggle.className = 'menu-toggle'; toggle.setAttribute('aria-expanded', String(state.menuOpen)); toggle.setAttribute('aria-label', '切换导航菜单')
    const toggleIcon = document.createElement('span'); toggle.append(toggleIcon)
    toggle.addEventListener('click', () => options.setMenuOpen(!options.getState().menuOpen))
    const nav = document.createElement('nav'); nav.className = `main-nav${state.menuOpen ? ' is-open' : ''}`; nav.setAttribute('aria-label', '主导航')
    const links: Array<[string, string]> = [
      ['/knowledge', '知识广场'], ['/education-resources', '教育资源'], ['/homestays', '民宿房源'],
      ['/software-tools', '软件工具'], ['/#explore', '资源目录'], ['/#about', '关于'],
    ]
    for (const [href, label] of links) {
      const link = document.createElement('a'); link.href = href; link.textContent = label
      link.addEventListener('click', event => { event.preventDefault(); options.navigate(href) })
      nav.append(link)
    }
    const auth = document.createElement('a'); auth.className = 'btn btn-outline-secondary nav-auth'; auth.href = state.loggedIn ? '/account' : '/login'; auth.textContent = state.loggedIn ? '个人中心' : '登录'
    auth.addEventListener('click', event => { event.preventDefault(); options.navigate(auth.getAttribute('href') || '/') })
    nav.append(auth)
    host.append(brand, toggle, nav)
  }
  render(options.getState())
  return { update: render, dispose() { host.replaceChildren() } }
}
