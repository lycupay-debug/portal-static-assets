import type { Content, TeachingResource } from '../api/generated'
import type { AccountTableRecord } from './accountTableFiltering'

type ContentRecord = Content & { deletedAt?: string | null }
type ResourceRecord = TeachingResource & { deletedAt?: string | null }

export type AccountTableRecordOptions = {
  section: string
  contentViewMode: 'active' | 'trash'
  contentItems: ContentRecord[]
  contentTrashItems: ContentRecord[]
  resourceViewMode: 'active' | 'trash'
  teachingResources: ResourceRecord[]
  teachingResourceTrash: ResourceRecord[]
  contentTypeLabels: Record<string, string>
  contentStatusLabels: Record<string, string>
}

function formatTableDate(value: string): string {
  return new Date(value).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
}

export function buildAccountTableRecords(options: AccountTableRecordOptions): AccountTableRecord[] {
  if (options.section === 'content-management') {
    const isTrash = options.contentViewMode === 'trash'
    const items = isTrash ? options.contentTrashItems : options.contentItems
    return items.map(item => ({
      id: item.id,
      title: item.title,
      type: item.tags?.length ? item.tags.join(' · ') : options.contentTypeLabels[item.contentType]!,
      status: options.contentStatusLabels[item.status]!,
      visibility: item.visibility === 'public' ? '公开' : '仅自己可见',
      updatedAt: formatTableDate(isTrash && 'deletedAt' in item ? String(item.deletedAt) : item.updatedAt),
      likeCount: item.likeCount ?? 0,
      favoriteCount: item.favoriteCount ?? 0,
      actions: isTrash ? '已删除' : item.status === 'published' ? '已发布' : '待发布',
      searchText: [item.title, item.slug, item.summary, item.body, ...(item.tags || []), item.parentCategory, item.childCategory].filter(Boolean).join(' '),
    }))
  }

  if (options.section === 'teaching-resources') {
    const isTrash = options.resourceViewMode === 'trash'
    const items = isTrash ? options.teachingResourceTrash : options.teachingResources
    return items.map(item => ({
      id: item.id,
      title: item.title,
      filename: item.filename || '—',
      type: item.type || '其他',
      grade: item.grade || '不限定年级',
      visibility: item.isPublic ? '公开' : '仅自己可见',
      description: item.description || '—',
      updatedAt: formatTableDate(isTrash ? (item.deletedAt || item.createdAt) : (item.updatedAt || item.createdAt)),
      actions: isTrash ? '已删除' : '可用',
      searchText: [item.title, item.filename, item.description, item.type, item.grade, item.url].filter(Boolean).join(' '),
    }))
  }

  return []
}
