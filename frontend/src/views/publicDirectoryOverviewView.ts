/**
 * Responsibility: render static public directory landing pages and their published-content cards.
 * Dependencies: page/tile configuration, read-only published content snapshots and a route navigation callback.
 * Collaboration: portalApplication.ts owns route/data loading and passes snapshots; this view owns only directory presentation.
 * Boundaries: all API/config text is written with textContent, external navigation is not synthesized, and no
 * business actions or authorization decisions are performed here. dispose releases delegated click listeners.
 */
import type { PublicDirectoryPage, PublicDirectoryTile } from '../config/publicDirectories'

type DirectoryContent = {
  id: string
  title: string
  slug: string
  contentType: 'article' | 'tool' | 'resource' | 'project'
  tags: string[]
  summary: string
  publishedAt?: string | null
}

export type PublicDirectoryOverviewState = {
  items: DirectoryContent[]
  loading: boolean
  error: string
}

export function mountPublicDirectoryOverview(
  host: HTMLElement,
  options: { page: PublicDirectoryPage; tiles: PublicDirectoryTile[]; contentTypeLabel: string; navigate: (target: string) => unknown },
) {
  const document = host.ownerDocument
  const main = document.createElement('main')
  main.className = 'directory-page'
  const hero = document.createElement('section')
  hero.className = 'directory-hero'
  const back = document.createElement('a')
  back.className = 'directory-back-link'
  back.href = '/'
  back.append(document.createElement('i'))
  back.firstElementChild!.className = 'bi bi-arrow-left'
  back.firstElementChild!.setAttribute('aria-hidden', 'true')
  back.append(document.createTextNode(' 返回首页'))
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  eyebrow.textContent = options.page.eyebrow
  const title = document.createElement('h1')
  title.append(document.createTextNode(options.page.title))
  const titleDot = document.createElement('span')
  titleDot.textContent = '.'
  title.append(titleDot)
  const subtitle = document.createElement('h2')
  subtitle.textContent = options.page.subtitle
  const description = document.createElement('p')
  description.className = 'directory-description'
  description.textContent = options.page.description
  hero.append(back, eyebrow, title, subtitle, description)

  const categories = document.createElement('section')
  categories.className = 'directory-section'
  const sectionHeading = document.createElement('div')
  sectionHeading.className = 'directory-section-heading'
  const headingGroup = document.createElement('div')
  const sectionEyebrow = document.createElement('p')
  sectionEyebrow.className = 'eyebrow'
  sectionEyebrow.textContent = 'BROWSE BY CATEGORY'
  const sectionTitle = document.createElement('h2')
  sectionTitle.textContent = '按主题探索'
  headingGroup.append(sectionEyebrow, sectionTitle)
  const count = document.createElement('span')
  count.textContent = `${String(options.tiles.length).padStart(2, '0')} 个分类`
  sectionHeading.append(headingGroup, count)
  const tileGrid = document.createElement('div')
  tileGrid.className = 'directory-tile-grid row g-3'
  for (const tile of options.tiles) {
    const card = document.createElement('article')
    card.className = 'directory-tile col-12 md:col-6 xl:col-3'
    const top = document.createElement('div')
    top.className = 'directory-tile-top'
    const number = document.createElement('span')
    number.textContent = tile.number
    const arrow = document.createElement('i')
    arrow.className = 'bi bi-arrow-up-right'
    arrow.setAttribute('aria-hidden', 'true')
    top.append(number, arrow)
    const heading = document.createElement('h3')
    heading.textContent = tile.title
    const copy = document.createElement('p')
    copy.textContent = tile.description
    const tags = document.createElement('div')
    tags.className = 'tag-list'
    for (const tag of tile.tags) { const chip = document.createElement('span'); chip.textContent = tag; tags.append(chip) }
    card.append(top, heading, copy, tags)
    tileGrid.append(card)
  }
  categories.append(sectionHeading, tileGrid)
  main.append(hero, categories)

  const published = document.createElement('section')
  published.className = 'directory-published-section'
  const publishedHeading = document.createElement('div')
  publishedHeading.className = 'directory-section-heading'
  const publishedHeadingGroup = document.createElement('div')
  const publishedEyebrow = document.createElement('p')
  publishedEyebrow.className = 'eyebrow'
  publishedEyebrow.textContent = 'RECENTLY ADDED'
  const publishedTitle = document.createElement('h2')
  publishedTitle.textContent = '最近添加'
  publishedHeadingGroup.append(publishedEyebrow, publishedTitle)
  const publishedCount = document.createElement('span')
  publishedHeading.append(publishedHeadingGroup, publishedCount)
  const publishedState = document.createElement('div')
  published.append(publishedHeading, publishedState)
  if (options.contentTypeLabel) main.append(published)

  const comingSoon = document.createElement('section')
  comingSoon.className = 'directory-published-section directory-coming-soon'
  const comingEyebrow = document.createElement('p')
  comingEyebrow.className = 'eyebrow'
  comingEyebrow.textContent = 'LISTINGS COMING SOON'
  const comingTitle = document.createElement('h2')
  comingTitle.textContent = '房源内容正在整理中'
  const comingCopy = document.createElement('p')
  comingCopy.textContent = '后续将在这里展示民宿图片、所在位置、价格、设施说明、入住规则及联系预订方式。'
  comingSoon.append(comingEyebrow, comingTitle, comingCopy)
  if (!options.contentTypeLabel) main.append(comingSoon)

  host.replaceChildren(main)
  const renderState = (state: PublicDirectoryOverviewState) => {
    if (!options.contentTypeLabel) return
    publishedCount.textContent = `${state.items.length} 条`
    publishedState.replaceChildren()
    if (state.loading && !state.items.length) {
      const message = document.createElement('p'); message.className = 'directory-empty'; message.textContent = '正在读取已发布内容…'; publishedState.append(message); return
    }
    if (state.error) {
      const message = document.createElement('p'); message.className = 'directory-empty'; message.textContent = state.error; publishedState.append(message); return
    }
    if (!state.items.length) {
      const message = document.createElement('p'); message.className = 'directory-empty'; message.textContent = '这里暂时还没有已发布的内容。内容发布后会自动显示在这里。'; publishedState.append(message); return
    }
    const grid = document.createElement('div')
    grid.className = 'published-content-grid row g-3'
    state.items.forEach((item, index) => {
      const link = document.createElement('a')
      link.className = 'published-content-card col-12 md:col-6 xl:col-4'
      link.href = `/content/${encodeURIComponent(item.slug)}`
      const meta = document.createElement('div')
      meta.className = 'published-content-meta'
      const type = document.createElement('span')
      type.className = 'published-content-type'
      type.textContent = item.tags.length ? item.tags.join(' · ') : options.contentTypeLabel
      const date = document.createElement('span')
      date.textContent = item.publishedAt ? new Date(item.publishedAt).toLocaleDateString('zh-CN') : '已发布'
      meta.append(type, date)
      const content = document.createElement('div')
      content.className = 'published-content-main'
      const number = document.createElement('span')
      number.className = 'published-content-index'
      number.textContent = String(index + 1).padStart(2, '0')
      const copy = document.createElement('div')
      copy.className = 'published-content-copy'
      const heading = document.createElement('h3')
      heading.textContent = item.title
      const summary = document.createElement('p')
      summary.textContent = item.summary || '打开查看完整内容。'
      const open = document.createElement('span')
      open.className = 'published-content-link'
      open.textContent = '查看详情 '
      const icon = document.createElement('i')
      icon.className = 'bi bi-arrow-up-right'
      icon.setAttribute('aria-hidden', 'true')
      open.append(icon)
      copy.append(heading, summary, open)
      content.append(number, copy)
      link.append(meta, content)
      grid.append(link)
    })
    publishedState.append(grid)
  }
  const onClick = (event: MouseEvent) => {
    const target = event.target instanceof document.defaultView!.Element ? event.target.closest<HTMLAnchorElement>('a[href^="/content/"]') : null
    if (!target || !host.contains(target)) return
    event.preventDefault()
    options.navigate(target.getAttribute('href') || '/')
  }
  host.addEventListener('click', onClick)
  return {
    update: renderState,
    dispose() { host.removeEventListener('click', onClick); host.replaceChildren() },
  }
}
