/**
 * Responsibility: render the public portal landing page, category search and static about section with native DOM.
 * Dependencies: configured home-section data and route navigation callback.
 * Collaboration: portalApplication.ts owns global navigation/header/footer; this view owns only the landing-page content and local search.
 * Boundaries: configured copy is inserted as text, no user data is interpreted as HTML, and anchor navigation is delegated.
 */
import type { HomeSection } from '../config/homeSections'

export function mountHomePageView(host: HTMLElement, sections: HomeSection[], navigate: (target: string) => unknown) {
  const document = host.ownerDocument
  const main = document.createElement('main')
  main.id = 'top'
  const hero = document.createElement('section')
  hero.className = 'hero'
  const heroCopy = document.createElement('div')
  heroCopy.className = 'hero-copy'
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  const statusDot = document.createElement('span')
  statusDot.className = 'status-dot'
  eyebrow.append(statusDot, document.createTextNode(' A PERSONAL KNOWLEDGE SPACE'))
  const heading = document.createElement('h1')
  heading.append(document.createTextNode('把想法整理好，'))
  heading.append(document.createElement('br'))
  const headingAccent = document.createElement('span')
  headingAccent.textContent = '让行动有迹可循。'
  heading.append(headingAccent)
  const description = document.createElement('p')
  description.className = 'hero-description'
  description.textContent = '一个持续生长的个人门户，用来整理知识、构建工具、归档资源，并记录从想法到实践的过程。'
  const heroActions = document.createElement('div')
  heroActions.className = 'hero-actions'
  const exploreLink = document.createElement('a')
  exploreLink.className = 'btn btn-dark button button-primary'
  exploreLink.href = '/#explore'
  exploreLink.textContent = '开始探索'
  const aboutLink = document.createElement('a')
  aboutLink.className = 'text-link'
  aboutLink.href = '/#about'
  aboutLink.textContent = '了解这个空间'
  heroActions.append(exploreLink, aboutLink)
  const heroNote = document.createElement('div')
  heroNote.className = 'hero-note'
  const noteLine = document.createElement('span')
  noteLine.className = 'note-line'
  const noteText = document.createElement('span')
  noteText.textContent = '持续整理 · 持续构建 · 持续迭代'
  heroNote.append(noteLine, noteText)
  heroCopy.append(eyebrow, heading, description, heroActions, heroNote)

  const art = document.createElement('div')
  art.className = 'hero-art'
  art.setAttribute('aria-label', '由层叠卡片构成的抽象知识结构插图')
  art.setAttribute('role', 'img')
  for (const orbit of ['orbit-one', 'orbit-two']) { const element = document.createElement('div'); element.className = `art-orbit ${orbit}`; art.append(element) }
  const backCard = document.createElement('div')
  backCard.className = 'art-card card-back'
  const backLabel = document.createElement('span')
  backLabel.className = 'art-card-label'
  backLabel.textContent = 'FIELD NOTES'
  const lines = document.createElement('span')
  lines.className = 'art-lines'
  for (let index = 0; index < 3; index += 1) lines.append(document.createElement('i'))
  backCard.append(backLabel, lines)
  const frontCard = document.createElement('div')
  frontCard.className = 'art-card card-front'
  const frontLabel = document.createElement('span')
  frontLabel.className = 'art-card-label'
  frontLabel.textContent = 'THOUGHT  ACTION'
  const symbol = document.createElement('div')
  symbol.className = 'art-symbol'
  symbol.append(document.createTextNode('P'))
  const symbolDot = document.createElement('span')
  symbolDot.textContent = '.'
  symbol.append(symbolDot)
  const cardFooter = document.createElement('div')
  cardFooter.className = 'art-card-footer'
  for (const word of ['COLLECT', 'CONNECT', 'CREATE']) { const label = document.createElement('span'); label.textContent = word; cardFooter.append(label) }
  frontCard.append(frontLabel, symbol, cardFooter)
  const artIndex = document.createElement('span')
  artIndex.className = 'art-index'
  artIndex.textContent = 'FIG. 001 — AN OPEN SYSTEM'
  const sparkOne = document.createElement('span')
  sparkOne.className = 'art-spark spark-one bi bi-stars'
  sparkOne.setAttribute('aria-hidden', 'true')
  const sparkTwo = document.createElement('span')
  sparkTwo.className = 'art-spark spark-two bi bi-plus-lg'
  sparkTwo.setAttribute('aria-hidden', 'true')
  art.append(backCard, frontCard, artIndex, sparkOne, sparkTwo)
  const heroBottom = document.createElement('div')
  heroBottom.className = 'hero-bottom'
  const independent = document.createElement('span')
  independent.textContent = '独立构建的个人空间'
  const publicSections = document.createElement('span')
  publicSections.textContent = '01 / 04  PUBLIC SECTIONS'
  const indicator = document.createElement('i')
  publicSections.append(indicator)
  heroBottom.append(independent, publicSections)
  hero.append(heroCopy, art, heroBottom)

  const explore = document.createElement('section')
  explore.id = 'explore'
  explore.className = 'explore-section'
  const sectionHeading = document.createElement('div')
  sectionHeading.className = 'section-heading'
  const headingGroup = document.createElement('div')
  const sectionEyebrow = document.createElement('p')
  sectionEyebrow.className = 'eyebrow'
  sectionEyebrow.textContent = 'EXPLORE THE PORTAL'
  const headingRule = document.createElement('span')
  headingRule.className = 'heading-rule'
  sectionEyebrow.append(headingRule)
  const sectionTitle = document.createElement('h2')
  sectionTitle.textContent = '探索你需要的资源。'
  headingGroup.append(sectionEyebrow, sectionTitle)
  const sectionIntro = document.createElement('p')
  sectionIntro.className = 'section-intro'
  sectionIntro.textContent = '从知识信息到实用工具，逐步拓展更多行业与服务。'
  sectionHeading.append(headingGroup, sectionIntro)
  const searchLabel = document.createElement('label')
  searchLabel.className = 'search-box'
  const searchIcon = document.createElement('span')
  searchIcon.className = 'search-icon bi bi-search'
  searchIcon.setAttribute('aria-hidden', 'true')
  const search = document.createElement('input')
  search.className = 'form-control'
  search.name = 'portalSearch'
  search.type = 'search'
  search.placeholder = '搜索资源、信息、工具或行业服务…'
  search.setAttribute('aria-label', '搜索门户分类')
  const shortcut = document.createElement('kbd')
  shortcut.textContent = '⌘ K'
  searchLabel.append(searchIcon, search, shortcut)
  const grid = document.createElement('div')
  grid.className = 'section-grid row g-3'
  const empty = document.createElement('div')
  empty.className = 'empty-search'
  const emptyTitle = document.createElement('strong')
  emptyTitle.textContent = '没有匹配的分类'
  const emptyDescription = document.createElement('p')
  emptyDescription.textContent = '试试“知识”“工具”“资源”或“项目”。'
  const clear = document.createElement('button')
  clear.type = 'button'
  clear.textContent = '清除搜索'
  empty.append(emptyTitle, emptyDescription, clear)
  explore.append(sectionHeading, searchLabel, grid, empty)

  const about = document.createElement('section')
  about.id = 'about'
  about.className = 'about-section'
  const mark = document.createElement('div')
  mark.className = 'about-mark'
  mark.textContent = 'R.'
  const aboutCopy = document.createElement('div')
  aboutCopy.className = 'about-copy'
  const aboutEyebrow = document.createElement('p')
  aboutEyebrow.className = 'eyebrow'
  aboutEyebrow.textContent = 'ONE PLATFORM, MULTIPLE INDUSTRIES'
  const aboutTitle = document.createElement('h2')
  aboutTitle.append(document.createTextNode('从信息发现，'))
  aboutTitle.append(document.createElement('br'))
  aboutTitle.append(document.createTextNode('到业务真正落地。'))
  const aboutDescription = document.createElement('p')
  aboutDescription.textContent = '这里汇集跨领域的资源信息、实用工具与专业服务，并逐步构建面向商家和不同行业的专属管理工作空间。用户获取信息，商家经营业务，各类服务在统一的平台体系中连接起来。'
  aboutCopy.append(aboutEyebrow, aboutTitle, aboutDescription)
  const aboutMeta = document.createElement('div')
  aboutMeta.className = 'about-meta'
  const designed = document.createElement('span')
  designed.textContent = 'DESIGNED FOR MANY'
  const meet = document.createElement('span')
  meet.textContent = 'Resources meet business.'
  aboutMeta.append(designed, meet)
  about.append(mark, aboutCopy, aboutMeta)
  main.append(hero, explore, about)
  host.replaceChildren(main)

  const renderCards = () => {
    const keyword = search.value.trim().toLocaleLowerCase()
    const matches = keyword ? sections.filter(section => [section.title, section.subtitle, section.description, ...section.tags].join(' ').toLocaleLowerCase().includes(keyword)) : sections
    grid.replaceChildren()
    empty.hidden = matches.length > 0
    grid.hidden = matches.length === 0
    for (const section of matches) {
      const card = document.createElement('article')
      card.className = 'section-card col-12 md:col-6 xl:col-4'
      const top = document.createElement('div')
      top.className = 'card-topline'
      const number = document.createElement('span')
      number.textContent = section.number
      const arrow = document.createElement('span')
      arrow.className = 'card-arrow bi bi-arrow-up-right'
      arrow.setAttribute('aria-hidden', 'true')
      top.append(number, arrow)
      const kicker = document.createElement('p')
      kicker.className = 'card-kicker'
      kicker.textContent = section.subtitle
      const title = document.createElement('h3')
      title.textContent = section.title
      const copy = document.createElement('p')
      copy.className = 'card-description'
      copy.textContent = section.description
      const tags = document.createElement('div')
      tags.className = 'tag-list'
      for (const tag of section.tags) { const chip = document.createElement('span'); chip.textContent = tag; tags.append(chip) }
      const bottom = document.createElement('div')
      bottom.className = 'card-bottom'
      const status = document.createElement('span')
      status.textContent = '内容逐步整理中'
      const bottomIcon = document.createElement('i')
      bottomIcon.className = 'bi bi-arrow-up-right'
      bottomIcon.setAttribute('aria-hidden', 'true')
      bottom.append(status, bottomIcon)
      card.append(top, kicker, title, copy, tags, bottom)
      grid.append(card)
    }
  }
  const onSearch = () => renderCards()
  const onClear = () => { search.value = ''; renderCards(); search.focus() }
  const onNavigate = (event: MouseEvent) => {
    event.preventDefault()
    const target = (event.currentTarget as HTMLAnchorElement).getAttribute('href') || '/'
    navigate(target)
  }
  search.addEventListener('input', onSearch)
  clear.addEventListener('click', onClear)
  exploreLink.addEventListener('click', onNavigate)
  aboutLink.addEventListener('click', onNavigate)
  renderCards()
  return {
    dispose() {
      search.removeEventListener('input', onSearch)
      clear.removeEventListener('click', onClear)
      exploreLink.removeEventListener('click', onNavigate)
      aboutLink.removeEventListener('click', onNavigate)
      host.replaceChildren()
    },
  }
}
