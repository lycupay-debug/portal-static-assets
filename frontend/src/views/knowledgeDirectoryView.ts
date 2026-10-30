/**
 * Responsibility: render the public knowledge directory, category filters, autocomplete and paged article list.
 * Dependencies: immutable controller snapshots, knowledge category metadata and navigation/action callbacks.
 * Collaboration: App.vue retains API loading and controller ownership; this view is a DOM-only projection of state.
 * Boundaries: API metadata is rendered as text, article links are encoded local routes, and filtering/paging rules
 * remain in createKnowledgeDirectoryController. dispose removes listeners and releases the mounted DOM subtree.
 */
import type { KnowledgeCategoryMenu } from '../config/knowledgeCategories'
import type { KnowledgeSearchSuggestion } from '../utils/knowledgeSearch'

type Article = { id: string; slug: string; title: string; summary?: string | null; publishedAt?: string | null }
export type KnowledgeDirectoryState = {
  category: string
  subcategory: string
  query: string
  highlightedSuggestion: number
  suggestions: KnowledgeSearchSuggestion[]
  filteredItems: Article[]
  visibleItems: Article[]
  loading: boolean
  error: string
}

export function mountKnowledgeDirectoryView(host: HTMLElement, options: {
  categories: KnowledgeCategoryMenu[]
  getState: () => KnowledgeDirectoryState
  chooseCategory: (category: string, subcategory?: string) => void
  setQuery: (query: string) => void
  setHighlightedSuggestion: (index: number) => void
  chooseSuggestion: (item: { value: string; kind: 'theme' | 'article' }) => void
  onSearchKeydown: (event: KeyboardEvent) => void
  setVisibleCount: () => void
  ensureListFilled: () => void
  navigate: (target: string) => unknown
}) {
  const document = host.ownerDocument
  const main = document.createElement('main')
  main.className = 'directory-page knowledge-directory-page'
  const toolbarSection = document.createElement('section')
  toolbarSection.className = 'directory-published-section knowledge-filter-section'
  const toolbar = document.createElement('div')
  toolbar.className = 'knowledge-toolbar'
  toolbar.setAttribute('aria-label', '文章筛选与搜索')
  const controls = document.createElement('div')
  controls.className = 'knowledge-toolbar-controls'
  const categoryNav = document.createElement('nav')
  categoryNav.className = 'knowledge-category-nav'
  categoryNav.setAttribute('aria-label', '知识分类')
  const subcategoryNav = document.createElement('div')
  subcategoryNav.className = 'knowledge-subcategory-nav'
  subcategoryNav.setAttribute('aria-label', '子分类')
  const searchWrap = document.createElement('div')
  searchWrap.className = 'knowledge-search-wrap'
  const searchLabel = document.createElement('label')
  searchLabel.className = 'knowledge-search-form knowledge-autocomplete'
  const searchIcon = document.createElement('span')
  searchIcon.className = 'knowledge-search-icon bi bi-search'
  searchIcon.setAttribute('aria-hidden', 'true')
  const search = document.createElement('input')
  search.className = 'form-control'
  search.type = 'search'
  search.placeholder = '搜索文章…'
  search.setAttribute('aria-label', '模糊搜索知识文章')
  search.autocomplete = 'off'
  const clear = document.createElement('button')
  clear.type = 'button'
  clear.className = 'knowledge-search-clear'
  clear.setAttribute('aria-label', '清除搜索')
  clear.innerHTML = '<i class="bi bi-x-lg" aria-hidden="true"></i>'
  searchLabel.append(searchIcon, search, clear)
  const suggestions = document.createElement('div')
  suggestions.className = 'knowledge-autocomplete-popper'
  suggestions.setAttribute('role', 'listbox')
  suggestions.setAttribute('aria-label', '搜索建议')
  searchWrap.append(searchLabel, suggestions)
  controls.append(categoryNav, subcategoryNav, searchWrap)
  toolbar.append(controls)
  toolbarSection.append(toolbar)

  const resultsSection = document.createElement('section')
  resultsSection.className = 'directory-published-section knowledge-results-section'
  const heading = document.createElement('div')
  heading.className = 'directory-section-heading knowledge-results-heading'
  const headingGroup = document.createElement('div')
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  eyebrow.textContent = 'KNOWLEDGE LIBRARY'
  const title = document.createElement('h2')
  title.textContent = '文章列表'
  headingGroup.append(eyebrow, title)
  const count = document.createElement('span')
  heading.append(headingGroup, count)
  const results = document.createElement('div')
  resultsSection.append(heading, results)
  main.append(toolbarSection, resultsSection)
  host.replaceChildren(main)

  const render = (state: KnowledgeDirectoryState) => {
    search.value = state.query
    categoryNav.replaceChildren()
    for (const category of options.categories) {
      const button = document.createElement('button')
      button.type = 'button'
      button.className = `knowledge-category-button${state.category === category.value ? ' is-active' : ''}`
      button.setAttribute('aria-pressed', String(state.category === category.value))
      button.textContent = category.label
      button.addEventListener('click', () => options.chooseCategory(category.value))
      categoryNav.append(button)
    }
    subcategoryNav.replaceChildren()
    const selectedCategory = options.categories.find(category => category.value === state.category)
    if (state.category !== 'recent' && selectedCategory?.children.length) {
      const all = document.createElement('button')
      all.type = 'button'
      all.className = `knowledge-subcategory-button${state.subcategory ? '' : ' is-active'}`
      all.setAttribute('aria-pressed', String(!state.subcategory))
      all.textContent = '全部'
      all.addEventListener('click', () => options.chooseCategory(state.category))
      subcategoryNav.append(all)
      for (const child of selectedCategory.children) {
        const button = document.createElement('button')
        button.type = 'button'
        button.className = `knowledge-subcategory-button${state.subcategory === child ? ' is-active' : ''}`
        button.setAttribute('aria-pressed', String(state.subcategory === child))
        button.textContent = child
        button.addEventListener('click', () => options.chooseCategory(state.category, child))
        subcategoryNav.append(button)
      }
      subcategoryNav.hidden = false
    } else subcategoryNav.hidden = true

    clear.hidden = !state.query
    suggestions.replaceChildren()
    suggestions.hidden = !state.query.trim() || !state.suggestions.length
    state.suggestions.forEach((item, index) => {
      const button = document.createElement('button')
      button.type = 'button'
      button.className = `knowledge-autocomplete-option${state.highlightedSuggestion === index ? ' highlighted' : ''}`
      button.setAttribute('role', 'option')
      button.setAttribute('aria-selected', String(state.highlightedSuggestion === index))
      button.addEventListener('mouseenter', () => options.setHighlightedSuggestion(index))
      button.addEventListener('click', () => options.chooseSuggestion({ value: item.label, kind: item.kind }))
      const kind = document.createElement('span')
      kind.className = 'knowledge-suggestion-kind'
      kind.textContent = item.kind === 'theme' ? '主题' : '文章'
      const copy = document.createElement('span')
      copy.className = 'knowledge-suggestion-copy'
      const label = document.createElement('strong')
      label.textContent = item.label
      copy.append(label)
      if ('summary' in item && item.summary) { const summary = document.createElement('small'); summary.textContent = item.summary; copy.append(summary) }
      const arrow = document.createElement('span')
      arrow.className = 'knowledge-suggestion-arrow'
      arrow.setAttribute('aria-hidden', 'true')
      const arrowIcon = document.createElement('i')
      arrowIcon.className = 'bi bi-arrow-up-right'
      arrow.append(arrowIcon)
      button.append(kind, copy, arrow)
      suggestions.append(button)
    })

    count.textContent = `${state.filteredItems.length} 篇文章`
    results.replaceChildren()
    if (state.loading && !state.filteredItems.length) { const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = '正在读取已发布内容…'; results.append(empty); return }
    if (state.error) { const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = state.error; results.append(empty); return }
    if (state.visibleItems.length) {
      const list = document.createElement('div')
      list.className = 'knowledge-infinite-list'
      state.visibleItems.forEach((item, index) => {
        const link = document.createElement('a')
        link.className = 'knowledge-list-item'
        link.href = `/content/${encodeURIComponent(item.slug)}`
        link.addEventListener('click', event => { event.preventDefault(); options.navigate(link.getAttribute('href') || '/') })
        const number = document.createElement('span')
        number.className = 'knowledge-list-index'
        number.textContent = String(index + 1).padStart(2, '0')
        const copy = document.createElement('span')
        copy.className = 'knowledge-list-copy'
        const itemTitle = document.createElement('strong')
        itemTitle.textContent = item.title
        copy.append(itemTitle)
        if (item.summary) { const summary = document.createElement('span'); summary.textContent = item.summary; copy.append(summary) }
        const date = document.createElement('time')
        date.textContent = item.publishedAt ? new Date(item.publishedAt).toLocaleDateString('zh-CN') : '已发布'
        const arrow = document.createElement('span')
        arrow.className = 'knowledge-list-arrow'
        arrow.setAttribute('aria-hidden', 'true')
        const icon = document.createElement('i')
        icon.className = 'bi bi-arrow-up-right'
        arrow.append(icon)
        link.append(number, copy, date, arrow)
        list.append(link)
      })
      const status = document.createElement('p')
      status.className = 'knowledge-list-status'
      status.textContent = state.visibleItems.length < state.filteredItems.length ? '向下滚动加载更多' : `已显示全部 ${state.filteredItems.length} 篇文章`
      list.append(status)
      results.append(list)
      options.ensureListFilled()
      return
    }
    const empty = document.createElement('p')
    empty.className = 'directory-empty'
    if (state.query.trim()) empty.textContent = `没有找到匹配“${state.query.trim()}”的文章。试试其他关键词或选择下方主题。`
    else if (state.category !== 'recent') empty.textContent = '当前分类暂时没有匹配的文章。现有文章尚未单独标注分类，筛选暂按标题和摘要中的主题词匹配。'
    else empty.textContent = '这里暂时还没有已发布的内容。内容发布后会自动显示在这里。'
    results.append(empty)
  }

  const onSearchInput = () => { options.setQuery(search.value); options.setVisibleCount(); render(options.getState()) }
  const onSearchKeydown = (event: KeyboardEvent) => { options.onSearchKeydown(event); render(options.getState()) }
  const onClear = () => { options.setQuery(''); options.setVisibleCount(); options.setHighlightedSuggestion(-1); search.focus(); render(options.getState()) }
  render(options.getState())
  search.addEventListener('input', onSearchInput)
  search.addEventListener('keydown', onSearchKeydown)
  clear.addEventListener('click', onClear)

  return {
    update: render,
    dispose() { search.removeEventListener('input', onSearchInput); search.removeEventListener('keydown', onSearchKeydown); clear.removeEventListener('click', onClear); host.replaceChildren() },
  }
}
