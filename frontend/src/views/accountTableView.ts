/**
 * Responsibility: render account table pages, content-version history and long-cell details in native DOM.
 * Dependencies: immutable table snapshots, account table metadata and explicit action callbacks.
 * Collaboration: App.vue owns session/API actions, table filtering and virtual-window calculations; this view
 * renders those snapshots and forwards user intent without mutating domain records.
 * Boundaries: server data is rendered through textContent, all destructive actions are explicit buttons, and
 * row virtualization remains owned by accountTableModel. dispose releases the scroll host and DOM listeners.
 */
import type { AccountTableDefinition } from '../config/accountTables'
import type { ContentVersion } from '../api/generated'

type TableRecord = Record<string, string | number>
type TableCell = { label: string; value: string } | null
export type AccountTableViewState = {
  section: string
  itemLabel: string
  itemDescription: string
  definition: AccountTableDefinition | null
  versionsContentId: string | null
  versionsContentTitle: string
  versions: ContentVersion[]
  contentBusy: boolean
  contentLoading: boolean
  resourceBusy: boolean
  records: TableRecord[]
  visibleRecords: TableRecord[]
  columnTemplate: string
  topSpace: number
  bottomSpace: number
  scrollTop: number
  query: string
  contentViewMode: 'active' | 'trash'
  resourceViewMode: 'active' | 'trash'
  selectedCell: TableCell
}

export function mountAccountTableView(host: HTMLElement, options: {
  getState: () => AccountTableViewState
  setQuery: (value: string) => void
  setScrollTop: (value: number) => void
  setScrollElement: (element: HTMLElement | null) => void
  createContent: () => unknown
  switchContentView: (mode: 'active' | 'trash') => unknown
  createResource: () => unknown
  switchResourceView: () => unknown
  resourceHasUrl: (id: string) => boolean
  openResource: (id: string) => unknown
  editResource: (id: string) => unknown
  downloadResource: (id: string) => unknown
  changeResource: (id: string, action: 'trash' | 'restore' | 'permanent') => unknown
  editContent: (id: string) => unknown
  openVersions: (id: string) => unknown
  changeContentStatus: (id: string, action: 'publish' | 'unpublish') => unknown
  deleteContent: (id: string) => unknown
  restoreContent: (id: string) => unknown
  permanentlyDeleteContent: (id: string) => unknown
  restoreVersion: (version: ContentVersion) => unknown
  backToContentList: () => unknown
  openCell: (label: string, value: string) => unknown
  closeCell: () => unknown
  cellValue: (record: TableRecord, key: string) => string
  cellPreview: (value: string) => string
  contentStatusLabel: (status: string) => string
}) {
  const document = host.ownerDocument
  let scrollElement: HTMLElement | null = null
  let searchElement: HTMLInputElement | null = null
  let searchFocused = false
  let selectionStart: number | null = null
  let selectionEnd: number | null = null

  const button = (label: string, className: string, disabled: boolean, action: () => unknown) => {
    const control = document.createElement('button')
    control.type = 'button'
    control.className = className
    control.disabled = disabled
    control.textContent = label
    control.addEventListener('click', action)
    return control
  }

  const renderVersions = (state: AccountTableViewState, root: HTMLElement) => {
    const section = document.createElement('section')
    section.className = 'content-editor-panel content-versions-panel'
    const heading = document.createElement('div')
    heading.className = 'content-editor-heading'
    const copy = document.createElement('div')
    const title = document.createElement('h2'); title.textContent = '版本历史'
    const description = document.createElement('p'); description.textContent = `“${state.versionsContentTitle}” · 每次有效编辑前自动保存快照，最多显示最近 100 个版本；恢复只替换内容字段，保留当前发布状态。`
    copy.append(title, description)
    heading.append(copy, button('返回内容列表', 'btn content-secondary-button', state.contentBusy, options.backToContentList))
    section.append(heading)
    if (state.versions.length) {
      const list = document.createElement('div'); list.className = 'content-version-list'
      for (const version of state.versions) {
        const row = document.createElement('div'); row.className = 'content-version-row'
        const details = document.createElement('div')
        const name = document.createElement('strong'); name.textContent = `版本 ${version.versionNumber} · ${version.title}`
        const meta = document.createElement('p'); meta.textContent = `${new Date(version.createdAt).toLocaleString('zh-CN')} · ${version.slug} · ${options.contentStatusLabel(version.status)}`
        details.append(name, meta)
        row.append(details, button('恢复此版本', 'btn content-secondary-button', state.contentBusy, () => options.restoreVersion(version)))
        list.append(row)
      }
      section.append(list)
    } else {
      const empty = document.createElement('div'); empty.className = 'virtual-table-empty'
      const name = document.createElement('strong'); name.textContent = '暂无历史版本'
      const note = document.createElement('p'); note.textContent = '保存第一次有效修改后，这里会出现可恢复的历史版本。'
      empty.append(name, note); section.append(empty)
    }
    root.append(section)
  }

  const renderCellDialog = (state: AccountTableViewState, root: HTMLElement) => {
    if (!state.selectedCell) return
    const backdrop = document.createElement('div'); backdrop.className = 'table-cell-detail-backdrop'
    const dialog = document.createElement('section'); dialog.className = 'table-cell-detail'; dialog.setAttribute('role', 'dialog'); dialog.setAttribute('aria-modal', 'true'); dialog.setAttribute('aria-label', state.selectedCell.label)
    const header = document.createElement('div'); header.className = 'table-cell-detail-heading'
    const title = document.createElement('h2'); title.textContent = state.selectedCell.label
    const close = document.createElement('button'); close.type = 'button'; close.setAttribute('aria-label', '关闭')
    const icon = document.createElement('i'); icon.className = 'bi bi-x-lg'; icon.setAttribute('aria-hidden', 'true'); close.append(icon); close.addEventListener('click', options.closeCell)
    header.append(title, close)
    const value = document.createElement('p'); value.textContent = state.selectedCell.value
    dialog.append(header, value, button('关闭', 'btn content-primary-button', false, options.closeCell))
    backdrop.addEventListener('click', event => { if (event.target === backdrop) options.closeCell() })
    backdrop.append(dialog); root.append(backdrop)
  }

  const render = (state: AccountTableViewState) => {
    searchFocused = Boolean(searchElement && document.activeElement === searchElement)
    selectionStart = searchFocused ? searchElement!.selectionStart : null
    selectionEnd = searchFocused ? searchElement!.selectionEnd : null
    const previousScrollTop = scrollElement?.scrollTop ?? state.scrollTop
    const root = document.createElement('div')
    root.className = 'account-table-native-view'
    let nextScrollElement: HTMLElement | null = null
    if (!state.definition) {
      const title = document.createElement('h1'); title.className = 'account-page-title'; title.textContent = state.itemLabel || '业务中心'
      const description = document.createElement('p'); description.className = 'auth-intro'; description.textContent = state.itemDescription || '管理你的业务内容与工作入口。'
      root.append(title, description)
    } else if (state.section === 'content-management' && state.versionsContentId) {
      renderVersions(state, root)
    } else {
      const panel = document.createElement('section'); panel.className = 'virtual-table-panel'; panel.style.marginRight = 'unset'; panel.style.marginLeft = '0px'
      const toolbar = document.createElement('div'); toolbar.className = 'virtual-table-toolbar'
      const heading = document.createElement('div'); heading.className = 'virtual-table-heading'
      const title = document.createElement('h2'); title.textContent = state.definition.title
      const count = document.createElement('span'); count.className = 'virtual-table-count'; count.textContent = state.contentLoading && state.section === 'content-management' ? '正在加载…' : `${state.records.length} 条记录`
      heading.append(title, count)
      const actions = document.createElement('div'); actions.className = 'content-toolbar-actions'
      const searchLabel = document.createElement('label'); searchLabel.className = 'visually-hidden'; searchLabel.textContent = '搜索表格内容'
      const search = document.createElement('input'); search.className = 'form-control virtual-table-search'; search.type = 'search'; search.name = 'tableSearch'; search.placeholder = '搜索表格内容…'; search.setAttribute('aria-label', '搜索表格内容'); search.value = state.query
      search.addEventListener('input', () => { options.setQuery(search.value); options.setScrollTop(0) })
      searchLabel.htmlFor = 'account-table-search'
      search.id = 'account-table-search'
      searchLabel.append(search)
      actions.append(searchLabel)
      if (state.section === 'content-management') {
        actions.append(button(state.contentViewMode === 'active' ? '回收站' : '返回内容列表', 'btn content-secondary-button', false, () => options.switchContentView(state.contentViewMode === 'active' ? 'trash' : 'active')))
        if (state.contentViewMode === 'active') actions.append(button('新建内容', 'btn content-primary-button', false, options.createContent))
      } else if (state.section === 'teaching-resources') {
        actions.append(button(state.resourceViewMode === 'active' ? '回收站' : '返回资源列表', 'btn content-secondary-button', state.resourceBusy, options.switchResourceView))
        if (state.resourceViewMode === 'active') actions.append(button('创建资源', 'btn content-primary-button', state.resourceBusy, options.createResource))
      }
      toolbar.append(heading, actions)
      const scroll = document.createElement('div'); scroll.className = 'virtual-table-scroll'; scroll.style.setProperty('--table-columns', state.columnTemplate)
      scroll.addEventListener('scroll', () => options.setScrollTop(scroll.scrollTop), { passive: true })
      const header = document.createElement('div'); header.className = 'virtual-table-header'; header.setAttribute('role', 'row')
      for (const column of state.definition.columns) { const cell = document.createElement('span'); cell.setAttribute('role', 'columnheader'); cell.textContent = column.label; header.append(cell) }
      scroll.append(header)
      if (state.records.length) {
        const body = document.createElement('div'); body.className = 'virtual-table-body'; body.setAttribute('role', 'rowgroup')
        const top = document.createElement('div'); top.className = 'virtual-table-spacer'; top.style.height = `${state.topSpace}px`; body.append(top)
        state.visibleRecords.forEach((record, index) => {
          const id = String(record.id ?? index)
          const row = document.createElement('div'); row.className = `virtual-table-row${state.section === 'teaching-resources' ? ' resource-clickable-row' : ''}`; row.setAttribute('role', 'row')
          if (state.section === 'teaching-resources') {
            row.tabIndex = 0
            row.addEventListener('click', () => options.openResource(id))
            row.addEventListener('keydown', event => { if (event.key === 'Enter') options.openResource(id) })
          }
          for (const column of state.definition!.columns) {
            if (column.key === 'actions' && (state.section === 'content-management' || state.section === 'teaching-resources')) {
              const cell = document.createElement('div'); cell.className = 'content-row-actions'; cell.setAttribute('role', 'cell')
              cell.addEventListener('click', event => event.stopPropagation())
              if (state.section === 'teaching-resources') {
                if (state.resourceViewMode === 'trash') {
                  cell.append(button('恢复', 'btn btn-sm btn-outline-secondary', state.resourceBusy, () => options.changeResource(id, 'restore')))
                  cell.append(button('永久删除', 'btn btn-sm btn-outline-danger content-danger-action', state.resourceBusy, () => options.changeResource(id, 'permanent')))
                } else {
                  cell.append(button('编辑', 'btn btn-sm btn-outline-secondary', state.resourceBusy, () => options.editResource(id)))
                  cell.append(button(options.resourceHasUrl(id) ? '打开链接' : '下载', 'btn btn-sm btn-outline-secondary', state.resourceBusy, () => options.downloadResource(id)))
                  cell.append(button('删除', 'btn btn-sm btn-outline-danger content-danger-action', state.resourceBusy, () => options.changeResource(id, 'trash')))
                }
              } else if (state.contentViewMode === 'trash') {
                cell.append(button('恢复', 'btn btn-sm btn-outline-secondary', state.contentBusy, () => options.restoreContent(id)))
                cell.append(button('永久删除', 'btn btn-sm btn-outline-danger content-danger-action', state.contentBusy, () => options.permanentlyDeleteContent(id)))
              } else {
                cell.append(button('编辑', 'btn btn-sm btn-outline-secondary', state.contentBusy, () => options.editContent(id)))
                cell.append(button('版本', 'btn btn-sm btn-outline-secondary', state.contentBusy, () => options.openVersions(id)))
                cell.append(button(String(record.status) === '已发布' ? '下架' : '发布', 'btn btn-sm btn-outline-secondary', state.contentBusy, () => options.changeContentStatus(id, String(record.status) === '已发布' ? 'unpublish' : 'publish')))
                cell.append(button('删除', 'btn btn-sm btn-outline-danger content-danger-action', state.contentBusy, () => options.deleteContent(id)))
              }
              row.append(cell)
            } else {
              const cell = document.createElement('span'); cell.setAttribute('role', 'cell')
              const value = options.cellValue(record, column.key)
              if (state.section === 'teaching-resources') cell.textContent = value
              else {
                const preview = button(options.cellPreview(value), 'table-cell-preview', false, () => options.openCell(column.label, value))
                if (Array.from(value).length > 10) preview.title = '点击查看完整内容'
                cell.append(preview)
              }
              row.append(cell)
            }
          }
          body.append(row)
        })
        const bottom = document.createElement('div'); bottom.className = 'virtual-table-spacer'; bottom.style.height = `${state.bottomSpace}px`; body.append(bottom)
        scroll.append(body)
      } else {
        const empty = document.createElement('div'); empty.className = 'virtual-table-empty'
        const icon = document.createElement('i'); icon.className = 'virtual-table-empty-icon bi bi-journal-text'; icon.setAttribute('aria-hidden', 'true')
        const emptyTitle = document.createElement('strong')
        emptyTitle.textContent = state.contentLoading && state.section === 'content-management' ? '正在读取内容' : state.query ? '没有匹配的记录' : '暂无数据'
        const note = document.createElement('p')
        note.textContent = state.contentLoading && state.section === 'content-management' ? '正在从数据库读取内容列表。' : state.query ? '尝试更换搜索关键词。' : state.section === 'content-management' ? (state.contentViewMode === 'trash' ? '回收站为空。已删除内容可以恢复，也可以永久删除。' : '创建第一篇内容，保存为草稿后再发布。') : state.section === 'teaching-resources' ? (state.resourceViewMode === 'trash' ? '资源回收站为空。' : '创建第一份教学资源，支持 PPT、MP3、PDF、Word 等文件。') : state.definition.note
        empty.append(icon, emptyTitle, note); scroll.append(empty)
      }
      panel.append(toolbar, scroll); root.append(panel)
      nextScrollElement = scroll
      searchElement = search
    }
    renderCellDialog(state, root)
    host.replaceChildren(root)
    scrollElement = nextScrollElement
    if (scrollElement) { scrollElement.scrollTop = previousScrollTop; options.setScrollElement(scrollElement) }
    else { searchElement = null; options.setScrollElement(null) }
    if (searchElement && searchFocused) { searchElement.focus(); if (selectionStart !== null && selectionEnd !== null) searchElement.setSelectionRange(selectionStart, selectionEnd) }
  }
  render(options.getState())
  return {
    update: render,
    dispose() { scrollElement = null; searchElement = null; options.setScrollElement(null); host.replaceChildren() },
  }
}
