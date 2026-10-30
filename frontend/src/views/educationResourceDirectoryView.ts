/**
 * Responsibility: render public education-resource filters, paged results and resource-detail dialog.
 * Dependencies: current API snapshots, filter option lists and small file presentation utilities.
 * Collaboration: App.vue owns API loading, debounced search, infinite paging and body-scroll lock state.
 * Boundaries: resource metadata is text-only; links are restricted to HTTP(S), download URLs remain server-issued,
 * and no file content is fetched or executed by this view. dispose releases host/document listeners and DOM.
 */
import { formatResourceFileSize } from '../utils/resourceFiles'
import { resourceFileIcon } from '../utils/resourceIcons'

type PublicResource = {
  id: string
  title: string
  description?: string | null
  type: string
  grade?: string | null
  url?: string | null
  filename?: string | null
  fileSize?: number | null
  createdAt: string
  updatedAt?: string | null
  downloadUrl?: string | null
}
export type EducationResourceDirectoryState<T extends PublicResource = PublicResource> = {
  category: string
  grade: string
  search: string
  types: string[]
  grades: string[]
  resources: T[]
  visibleResources: T[]
  loading: boolean
  error: string
  selected: T | null
}

export function mountEducationResourceDirectoryView<T extends PublicResource>(host: HTMLElement, options: {
  getState: () => EducationResourceDirectoryState<T>
  setCategory: (value: string) => void
  setGrade: (value: string) => void
  setSearch: (value: string) => void
  resetVisible: () => void
  selectResource: (resource: T | null) => void
  ensureListFilled: () => void
}) {
  const document = host.ownerDocument
  const main = document.createElement('main')
  main.className = 'directory-page education-resource-directory-page'
  const filterSection = document.createElement('section')
  filterSection.className = 'directory-published-section knowledge-filter-section education-resource-filter-section'
  const heading = document.createElement('div')
  heading.className = 'education-resource-heading'
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  eyebrow.textContent = 'LEARNING LIBRARY'
  const title = document.createElement('h1')
  title.append(document.createTextNode('教育资源'))
  const dot = document.createElement('span')
  dot.textContent = '.'
  title.append(dot)
  const description = document.createElement('p')
  description.textContent = '按资源类型与适用年级筛选，快速找到需要的学习资料。'
  heading.append(eyebrow, title, description)
  const toolbar = document.createElement('div')
  toolbar.className = 'knowledge-toolbar'
  toolbar.setAttribute('aria-label', '教育资源筛选与搜索')
  const controls = document.createElement('div')
  controls.className = 'knowledge-toolbar-controls'
  const typeNav = document.createElement('nav')
  typeNav.className = 'knowledge-category-nav'
  typeNav.setAttribute('aria-label', '资源类型')
  const gradeNav = document.createElement('nav')
  gradeNav.className = 'knowledge-subcategory-nav'
  gradeNav.setAttribute('aria-label', '适用年级')
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
  search.placeholder = '搜索资源名称、说明或文件名…'
  search.setAttribute('aria-label', '模糊搜索教育资源')
  search.autocomplete = 'off'
  const clear = document.createElement('button')
  clear.type = 'button'
  clear.className = 'knowledge-search-clear'
  clear.setAttribute('aria-label', '清除搜索')
  const clearIcon = document.createElement('i')
  clearIcon.className = 'bi bi-x-lg'
  clearIcon.setAttribute('aria-hidden', 'true')
  clear.append(clearIcon)
  searchLabel.append(searchIcon, search, clear)
  searchWrap.append(searchLabel)
  controls.append(typeNav, gradeNav, searchWrap)
  toolbar.append(controls)
  filterSection.append(heading, toolbar)

  const resultsSection = document.createElement('section')
  resultsSection.className = 'directory-published-section knowledge-results-section education-resource-results-section'
  const resultsHeading = document.createElement('div')
  resultsHeading.className = 'directory-section-heading knowledge-results-heading'
  const resultsHeadingGroup = document.createElement('div')
  const resultsEyebrow = document.createElement('p')
  resultsEyebrow.className = 'eyebrow'
  resultsEyebrow.textContent = 'PUBLIC LEARNING MATERIALS'
  const resultsTitle = document.createElement('h2')
  resultsTitle.textContent = '资源列表'
  resultsHeadingGroup.append(resultsEyebrow, resultsTitle)
  const count = document.createElement('span')
  resultsHeading.append(resultsHeadingGroup, count)
  const results = document.createElement('div')
  resultsSection.append(resultsHeading, results)
  main.append(filterSection, resultsSection)
  host.replaceChildren(main)

  const safeUrl = (value?: string | null): string | null => {
    if (!value) return null
    try {
      const url = new URL(value, document.location.href)
      return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null
    } catch { return null }
  }

  const safeDownloadUrl = (value?: string | null): string | null => {
    const resolved = safeUrl(value)
    if (!resolved) return null
    try { return new URL(resolved).origin === new URL(document.location.href).origin ? resolved : null } catch { return null }
  }

  const renderDetail = (resource: PublicResource | null) => {
    const existing = host.querySelector('.education-resource-detail-backdrop')
    existing?.remove()
    if (!resource) return
    const backdrop = document.createElement('div')
    backdrop.className = 'share-dialog-backdrop education-resource-detail-backdrop'
    backdrop.tabIndex = -1
    const dialog = document.createElement('section')
    dialog.className = 'share-dialog education-resource-detail-dialog'
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'education-resource-detail-title')
    const close = document.createElement('button')
    close.type = 'button'
    close.className = 'education-resource-detail-close'
    close.setAttribute('aria-label', '关闭资源详情')
    const closeIcon = document.createElement('i')
    closeIcon.className = 'bi bi-x-lg'
    closeIcon.setAttribute('aria-hidden', 'true')
    close.append(closeIcon)
    close.addEventListener('click', () => options.selectResource(null))
    const label = document.createElement('span')
    label.className = 'share-dialog-eyebrow'
    label.textContent = 'LEARNING RESOURCE'
    const title = document.createElement('h2')
    title.id = 'education-resource-detail-title'
    title.textContent = resource.title
    const description = document.createElement('p')
    description.className = 'education-resource-detail-description'
    description.textContent = resource.description || '此资源暂无补充说明。'
    if (!resource.description) description.classList.add('education-resource-detail-empty-description')
    const metadata = document.createElement('dl')
    metadata.className = 'education-resource-detail-meta'
    const addMeta = (name: string, value?: string | null) => {
      if (!value) return
      const row = document.createElement('div')
      const term = document.createElement('dt'); term.textContent = name
      const detail = document.createElement('dd'); detail.textContent = value
      row.append(term, detail); metadata.append(row)
    }
    addMeta('资源类型', resource.type || '未分类')
    addMeta('适用年级', resource.grade || '不限定年级')
    addMeta('文件名称', resource.filename)
    if (resource.fileSize != null) addMeta('文件大小', formatResourceFileSize(resource.fileSize))
    const externalUrl = safeUrl(resource.url)
    const downloadUrl = safeDownloadUrl(resource.downloadUrl)
    if (externalUrl) {
      const row = document.createElement('div')
      const term = document.createElement('dt'); term.textContent = '在线链接'
      const detail = document.createElement('dd')
      const link = document.createElement('a'); link.className = 'education-resource-detail-url'; link.href = externalUrl; link.target = '_blank'; link.rel = 'noopener noreferrer'; link.textContent = resource.url || externalUrl
      detail.append(link); row.append(term, detail); metadata.append(row)
    }
    addMeta('更新时间', new Date(resource.updatedAt || resource.createdAt).toLocaleDateString('zh-CN'))
    dialog.append(close, label, title, description, metadata)

    const isAudio = resource.filename?.toLowerCase().endsWith('.mp3') === true
    const previewUrl = isAudio && downloadUrl ? safeUrl(downloadUrl.replace(/\/download$/, '/preview')) : null
    if (previewUrl) {
      const preview = document.createElement('div')
      preview.className = 'education-resource-audio-preview'
      const previewLabel = document.createElement('label')
      previewLabel.textContent = 'MP3 音频预览'
      const audio = document.createElement('audio')
      audio.controls = true
      audio.preload = 'metadata'
      audio.src = previewUrl
      previewLabel.htmlFor = `education-resource-audio-${resource.id}`
      audio.id = `education-resource-audio-${resource.id}`
      audio.append(document.createTextNode('当前浏览器不支持音频播放，请使用下载功能。'))
      preview.append(previewLabel, audio)
      dialog.append(preview)
    }
    if (!externalUrl && !downloadUrl) {
      const warning = document.createElement('p')
      warning.className = 'education-resource-detail-warning'
      warning.textContent = '该资源当前没有可访问的文件或链接，请联系资源发布者。'
      dialog.append(warning)
    }
    const actions = document.createElement('div')
    actions.className = 'education-resource-detail-actions'
    if (externalUrl) {
      const link = document.createElement('a'); link.className = 'education-resource-detail-action education-resource-detail-open'; link.href = externalUrl; link.target = '_blank'; link.rel = 'noopener noreferrer'
      const icon = document.createElement('i'); icon.className = 'bi bi-box-arrow-up-right'; icon.setAttribute('aria-hidden', 'true'); link.append(icon, document.createTextNode(' 打开链接')); actions.append(link)
    }
    if (downloadUrl) {
      const link = document.createElement('a'); link.className = 'education-resource-detail-action education-resource-detail-download'; link.setAttribute('href', resource.downloadUrl || downloadUrl)
      const icon = document.createElement('i'); icon.className = 'bi bi-download'; icon.setAttribute('aria-hidden', 'true'); link.append(icon, document.createTextNode(' 下载文件')); actions.append(link)
    }
    if (actions.childElementCount) dialog.append(actions)
    backdrop.append(dialog)
    backdrop.addEventListener('click', event => { if (event.target === backdrop) options.selectResource(null) })
    backdrop.addEventListener('keydown', event => { if (event.key === 'Escape') { event.preventDefault(); options.selectResource(null) } })
    host.append(backdrop)
    close.focus()
  }

  const render = (state: EducationResourceDirectoryState<T>) => {
    search.value = state.search
    typeNav.replaceChildren()
    for (const type of state.types) {
      const button = document.createElement('button'); button.type = 'button'; button.className = `knowledge-category-button${state.category === type ? ' is-active' : ''}`; button.setAttribute('aria-pressed', String(state.category === type)); button.textContent = type
      button.addEventListener('click', () => options.setCategory(type))
      typeNav.append(button)
    }
    gradeNav.replaceChildren()
    for (const grade of state.grades) {
      const button = document.createElement('button'); button.type = 'button'; button.className = `knowledge-subcategory-button${state.grade === grade ? ' is-active' : ''}`; button.setAttribute('aria-pressed', String(state.grade === grade)); button.textContent = grade
      button.addEventListener('click', () => options.setGrade(grade))
      gradeNav.append(button)
    }
    clear.hidden = !state.search
    count.textContent = `${state.resources.length} 份资源`
    results.replaceChildren()
    if (state.loading && !state.resources.length) { const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = '正在读取公开教育资源…'; results.append(empty) }
    else if (state.error) { const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = state.error; results.append(empty) }
    else if (state.visibleResources.length) {
      const list = document.createElement('div'); list.className = 'knowledge-infinite-list'
      for (const resource of state.visibleResources) {
        const link = document.createElement('a'); link.className = 'knowledge-list-item education-resource-list-item'; link.href = '#resource-detail'; link.setAttribute('aria-label', `${resource.title}，查看资源详情`)
        link.addEventListener('click', event => { event.preventDefault(); options.selectResource(resource) })
        const fileIcon = document.createElement('span'); fileIcon.className = 'education-resource-file-icon'; fileIcon.setAttribute('aria-hidden', 'true')
        const icon = document.createElement('i'); icon.className = `bi ${resourceFileIcon(resource.filename, resource.url)}`; fileIcon.append(icon)
        const copy = document.createElement('span'); copy.className = 'knowledge-list-copy'
        const title = document.createElement('strong'); title.textContent = resource.title
        copy.append(title)
        if (resource.description) { const description = document.createElement('span'); description.textContent = resource.description; copy.append(description) }
        const meta = document.createElement('span'); meta.className = 'education-resource-meta'
        const type = document.createElement('b'); type.textContent = resource.type
        const grade = document.createElement('b'); grade.textContent = resource.grade || '不限定年级'
        const filename = document.createElement('span'); filename.className = 'education-resource-filename'; filename.textContent = resource.filename || (resource.url ? '在线资源' : '学习资料')
        meta.append(type, grade, filename); copy.append(meta)
        const date = document.createElement('time'); date.textContent = new Date(resource.updatedAt || resource.createdAt).toLocaleDateString('zh-CN')
        const arrow = document.createElement('span'); arrow.className = 'knowledge-list-arrow'; arrow.setAttribute('aria-hidden', 'true')
        const arrowIcon = document.createElement('i'); arrowIcon.className = 'bi bi-arrow-up-right'; arrow.append(arrowIcon)
        link.append(fileIcon, copy, date, arrow); list.append(link)
      }
      const status = document.createElement('p'); status.className = 'knowledge-list-status'; status.textContent = state.visibleResources.length < state.resources.length ? '向下滚动加载更多' : `已显示全部 ${state.resources.length} 份资源`
      list.append(status); results.append(list); options.ensureListFilled()
    } else if (state.search.trim() || state.category !== '全部资源' || state.grade !== '全部年级') {
      const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = '没有找到符合条件的公开资源。请调整筛选条件或搜索关键词。'; results.append(empty)
    } else {
      const empty = document.createElement('p'); empty.className = 'directory-empty'; empty.textContent = '目前还没有公开的教育资源。登录后在“教学资源”中创建资源，并勾选“公开展示”，资源就会显示在这里。'; results.append(empty)
    }
    renderDetail(state.selected)
  }
  const onSearchInput = () => { options.setSearch(search.value); options.resetVisible() }
  const onClear = () => { options.setSearch(''); options.resetVisible(); search.focus() }
  search.addEventListener('input', onSearchInput)
  clear.addEventListener('click', onClear)
  render(options.getState())
  return {
    update: render,
    dispose() { search.removeEventListener('input', onSearchInput); clear.removeEventListener('click', onClear); host.replaceChildren() },
  }
}
