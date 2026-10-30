/** Responsibility: render the public footer and published legal-document links in native DOM. */
export type SiteFooterDocument = { slug: string; title: string }
export function mountSiteFooterView(host: HTMLElement, options: { navigate: (target: string) => unknown }) {
  const document = host.ownerDocument
  const brand = document.createElement('a'); brand.className = 'footer-brand'; brand.href = '/'
  brand.append(document.createTextNode('资源平台 '))
  const mark = document.createElement('span'); mark.textContent = '©'; brand.append(mark)
  brand.addEventListener('click', event => { event.preventDefault(); options.navigate('/') })
  const tagline = document.createElement('span'); tagline.textContent = '连接资源、用户与行业服务'
  const nav = document.createElement('nav'); nav.className = 'site-footer-policies'; nav.setAttribute('aria-label', '网站政策')
  host.replaceChildren(brand, tagline, nav)
  const update = (documents: SiteFooterDocument[]) => {
    nav.replaceChildren()
    nav.hidden = documents.length === 0
    for (const item of documents) {
      const link = document.createElement('a'); link.href = `/legal/${encodeURIComponent(item.slug)}`; link.textContent = item.title
      link.addEventListener('click', event => { event.preventDefault(); options.navigate(link.getAttribute('href') || '/') })
      nav.append(link)
    }
  }
  return { update, dispose() { host.replaceChildren() } }
}
