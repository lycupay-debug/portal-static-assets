/**
 * Responsibility: render a teaching-resource detail drawer using native DOM APIs.
 * Dependencies: the generated TeachingResource contract and shared resource file-size formatter.
 * Collaboration: portalApplication.ts owns resource selection and action behavior; this module renders the selected resource
 * and reports close/edit/download intent through callbacks.
 * Boundaries: no API calls, application state mutation, framework dependency, or HTML-string parsing. User-controlled
 * values are assigned through textContent; only HTTP(S) resource URLs become links.
 * Cleanup: remove document listeners and detach the rendered drawer when the owner disposes this mount.
 */
import type { TeachingResource } from '../api/generated'
import { formatResourceFileSize } from '../utils/resourceFiles'

export type TeachingResourceViewMode = 'active' | 'trash'

export type TeachingResourceDrawerActions = {
  onClose: () => void
  onEdit: (id: string) => void
  onDownload: (id: string) => void
}

function formatResourceDate(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

function formatDisplayedFileSize(size?: number | null): string {
  return formatResourceFileSize(size) || '—'
}

function appendField(document: Document, list: HTMLElement, label: string, value: string): HTMLElement {
  const row = document.createElement('div')
  const term = document.createElement('dt')
  term.textContent = label
  const description = document.createElement('dd')
  description.textContent = value
  row.append(term, description)
  list.append(row)
  return description
}

function appendResourceLink(document: Document, list: HTMLElement, value?: string | null): void {
  const row = document.createElement('div')
  const term = document.createElement('dt')
  term.textContent = '资源链接'
  const description = document.createElement('dd')
  if (!value) {
    description.textContent = '站内文件'
  } else {
    let safeUrl: URL | null = null
    try {
      const candidate = new URL(value, document.baseURI)
      if (candidate.protocol === 'https:' || candidate.protocol === 'http:') safeUrl = candidate
    } catch {
      safeUrl = null
    }
    if (safeUrl) {
      const link = document.createElement('a')
      link.href = safeUrl.href
      link.target = '_blank'
      link.rel = 'noopener noreferrer'
      link.textContent = value
      description.append(link)
    } else {
      description.textContent = value
    }
  }
  row.append(term, description)
  list.append(row)
}

export function mountTeachingResourceDrawer(
  container: HTMLElement,
  resource: TeachingResource,
  viewMode: TeachingResourceViewMode,
  actions: TeachingResourceDrawerActions,
): () => void {
  const document = container.ownerDocument
  const backdrop = document.createElement('div')
  backdrop.className = 'resource-detail-drawer-backdrop'

  const drawer = document.createElement('aside')
  drawer.className = 'resource-detail-drawer'
  drawer.setAttribute('role', 'dialog')
  drawer.setAttribute('aria-modal', 'true')
  drawer.setAttribute('aria-labelledby', 'resource-detail-title')

  const header = document.createElement('header')
  header.className = 'resource-detail-drawer-heading'
  const headingCopy = document.createElement('div')
  const eyebrow = document.createElement('span')
  eyebrow.className = 'share-dialog-eyebrow'
  eyebrow.textContent = 'TEACHING RESOURCE'
  const heading = document.createElement('h2')
  heading.id = 'resource-detail-title'
  heading.textContent = '资源详情'
  headingCopy.append(eyebrow, heading)

  const closeButton = document.createElement('button')
  closeButton.type = 'button'
  closeButton.setAttribute('aria-label', '关闭详情')
  const closeIcon = document.createElement('i')
  closeIcon.className = 'bi bi-x-lg'
  closeIcon.setAttribute('aria-hidden', 'true')
  closeButton.append(closeIcon)
  header.append(headingCopy, closeButton)

  const body = document.createElement('div')
  body.className = 'resource-detail-drawer-body'
  const title = document.createElement('h3')
  title.textContent = resource.title || '—'
  const fields = document.createElement('dl')
  fields.className = 'resource-detail-fields'
  appendField(document, fields, '资源名称', resource.title || '—')
  appendField(document, fields, '文件名称', resource.filename || '未上传文件')
  appendField(document, fields, '资源类型', resource.type || '其他')
  appendField(document, fields, '适用年级', resource.grade || '不限定年级')
  appendField(document, fields, '公开状态', resource.isPublic ? '公开展示' : '仅自己可见')
  appendField(document, fields, '文件大小', formatDisplayedFileSize(resource.fileSize))
  appendField(document, fields, 'MIME 类型', resource.mimeType || '—')
  appendResourceLink(document, fields, resource.url)
  appendField(document, fields, '资源说明', resource.description || '暂无说明')
  appendField(document, fields, '创建时间', formatResourceDate(resource.createdAt))
  appendField(document, fields, '更新时间', formatResourceDate(resource.updatedAt))
  if (resource.deletedAt) appendField(document, fields, '移入回收站时间', formatResourceDate(resource.deletedAt))
  const idField = appendField(document, fields, '资源 ID', resource.id)
  idField.classList.add('resource-detail-id')

  body.append(title, fields)
  const footer = document.createElement('div')
  footer.className = 'resource-detail-actions'
  if (viewMode === 'active') {
    const editButton = document.createElement('button')
    editButton.type = 'button'
    editButton.className = 'btn content-secondary-button'
    editButton.textContent = '编辑资源'
    editButton.addEventListener('click', () => actions.onEdit(resource.id))

    const downloadButton = document.createElement('button')
    downloadButton.type = 'button'
    downloadButton.className = 'btn content-primary-button'
    downloadButton.textContent = resource.url ? '打开资源链接' : '下载文件'
    downloadButton.addEventListener('click', () => actions.onDownload(resource.id))
    footer.append(editButton, downloadButton)
    body.append(footer)
  }

  drawer.append(header, body)
  backdrop.append(drawer)
  container.replaceChildren(backdrop)

  const onBackdropClick = (event: MouseEvent) => {
    if (event.target === backdrop) actions.onClose()
  }
  const onDocumentKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') actions.onClose()
  }
  const onCloseClick = () => actions.onClose()
  backdrop.addEventListener('click', onBackdropClick)
  closeButton.addEventListener('click', onCloseClick)
  document.addEventListener('keydown', onDocumentKeydown)

  let mounted = true
  return () => {
    if (!mounted) return
    mounted = false
    backdrop.removeEventListener('click', onBackdropClick)
    closeButton.removeEventListener('click', onCloseClick)
    document.removeEventListener('keydown', onDocumentKeydown)
    if (backdrop.parentElement === container) container.replaceChildren()
  }
}
