import type { Ref } from 'vue'
import { apiRequest } from '../api/client'
import { mergeLegacyTaxonomyIntoTags } from '../utils/knowledgeTags'
import type { Content as APIContent, ContentPatch, ContentVersion as APIContentVersion, ContentWrite } from '../api/generated'

type PortalContent = APIContent
type ContentVersion = APIContentVersion
type ContentForm = {
  title: string
  slug: string
  contentType: PortalContent['contentType']
  tags: string[]
  summary: string
  body: string
  visibility: PortalContent['visibility']
}

type Requester = typeof apiRequest

interface ContentManagementActionOptions {
  routeContentId: Ref<string | null>
  contentItems: Ref<PortalContent[]>
  contentTrashItems: Ref<PortalContent[]>
  contentBusy: Ref<boolean>
  contentError: Ref<string>
  contentNotice: Ref<string>
  contentEditorOpen: Ref<boolean>
  editingContentId: Ref<string | null>
  contentForm: Ref<ContentForm>
  contentVersions: Ref<ContentVersion[]>
  versionsContentId: Ref<string | null>
  versionsContentTitle: Ref<string>
  emptyContentForm: () => ContentForm
  loadContentList: () => Promise<unknown> | unknown
  navigate: (target: string) => Promise<unknown> | unknown
  request?: Requester
}

/** Owns content-editor, version-history, publish and recycle-bin actions. */
export function useContentManagementActions(options: ContentManagementActionOptions) {
  const request = options.request || apiRequest

  async function initializeContentEditorRoute() {
    const id = options.routeContentId.value
    options.contentError.value = ''
    options.contentEditorOpen.value = false
    if (!id) {
      options.editingContentId.value = null
      options.contentForm.value = options.emptyContentForm()
      return
    }
    if (options.editingContentId.value === id && options.contentForm.value.title) return
    options.contentBusy.value = true
    try {
      const result = await request<{ item?: PortalContent; error?: string }>(`/content/${encodeURIComponent(id)}`, {}, '读取内容详情失败。')
      if (!result.item) throw new Error(result.error || '读取内容详情失败。')
      const item = result.item
      options.editingContentId.value = item.id
      options.contentForm.value = {
        title: item.title,
        slug: item.slug,
        contentType: item.contentType,
        tags: mergeLegacyTaxonomyIntoTags(item.tags || [], item.parentCategory, item.childCategory),
        summary: item.summary,
        body: item.body,
        visibility: item.visibility,
      }
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '读取内容详情失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function openContentVersions(id: string) {
    const item = options.contentItems.value.find((content) => content.id === id)
    if (!item) return
    options.contentBusy.value = true
    options.contentError.value = ''
    try {
      const result = await request<{ items?: ContentVersion[]; error?: string }>(`/content/${encodeURIComponent(id)}/versions`, {}, '读取版本历史失败。')
      options.contentVersions.value = result.items || []
      options.versionsContentId.value = id
      options.versionsContentTitle.value = item.title
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '读取版本历史失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function restoreContentVersion(version: ContentVersion) {
    const id = options.versionsContentId.value
    if (!id || !window.confirm(`将“${options.versionsContentTitle.value}”恢复到版本 ${version.versionNumber}？当前内容会自动保存为一个历史版本。`)) return
    options.contentBusy.value = true
    options.contentError.value = ''
    try {
      await request<unknown>(`/content/${encodeURIComponent(id)}/versions/${encodeURIComponent(version.versionId)}/restore`, { method: 'POST' }, '恢复历史版本失败。')
      options.versionsContentId.value = null
      options.contentNotice.value = `已恢复到版本 ${version.versionNumber}。恢复前的内容已保存到版本历史。`
      await options.loadContentList()
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '恢复历史版本失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function restoreDeletedContent(id: string) {
    const item = options.contentTrashItems.value.find((content) => content.id === id)
    if (!item || !window.confirm(`将“${item.title}”从回收站恢复？`)) return
    options.contentBusy.value = true
    options.contentError.value = ''
    try {
      await request<unknown>(`/content/${encodeURIComponent(id)}/restore`, { method: 'POST' }, '恢复内容失败。')
      options.contentNotice.value = `“${item.title}”已恢复。`
      await options.loadContentList()
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '恢复内容失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function permanentlyDeleteContent(id: string) {
    const item = options.contentTrashItems.value.find((content) => content.id === id)
    if (!item || !window.confirm(`永久删除“${item.title}”？此操作不可撤销，相关版本历史也会一并删除。`)) return
    options.contentBusy.value = true
    options.contentError.value = ''
    try {
      await request<unknown>(`/content/${encodeURIComponent(id)}/permanent`, { method: 'DELETE' }, '永久删除失败。')
      options.contentNotice.value = `“${item.title}”已永久删除。`
      await options.loadContentList()
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '永久删除失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function saveContent() {
    const form = options.contentForm.value
    if (!form.title.trim()) { options.contentError.value = '请填写内容标题。'; return }
    if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(form.slug.trim())) { options.contentError.value = '请填写有效的 URL 别名，仅允许小写英文字母、数字和连字符。'; return }
    if (!form.body.trim()) { options.contentError.value = '正文不能为空。'; return }
    if (!form.tags.length) { options.contentError.value = '请至少选择一个知识广场标签。'; return }
    options.contentBusy.value = true
    options.contentError.value = ''
    options.contentNotice.value = ''
    try {
      const editing = options.editingContentId.value
      const payload: ContentWrite | ContentPatch = {
        title: form.title.trim(),
        slug: form.slug.trim(),
        contentType: form.contentType,
        tags: mergeLegacyTaxonomyIntoTags(form.tags),
        parentCategory: null,
        childCategory: null,
        summary: form.summary,
        body: form.body,
        visibility: form.visibility,
      }
      const result = await request<{ item?: PortalContent; error?: string }>(editing ? `/content/${encodeURIComponent(editing)}` : '/content', {
        method: editing ? 'PATCH' : 'POST',
        body: JSON.stringify(payload),
      }, '保存内容失败。')
      if (!result.item) throw new Error(result.error || '保存内容失败。')
      if (!editing) options.editingContentId.value = result.item.id
      options.contentEditorOpen.value = false
      options.contentNotice.value = editing ? '内容已保存。' : '草稿已创建。'
      await options.navigate('/content-management')
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '保存内容失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function changeContentStatus(id: string, action: 'publish' | 'unpublish') {
    options.contentBusy.value = true
    options.contentError.value = ''
    options.contentNotice.value = ''
    try {
      const result = await request<{ item?: PortalContent; error?: string }>(`/content/${encodeURIComponent(id)}/${action}`, { method: 'POST' }, '更新内容状态失败。')
      if (!result.item) throw new Error(result.error || '更新内容状态失败。')
      options.contentNotice.value = action === 'publish' ? '内容状态已更新为已发布。' : '内容已下架。'
      await options.loadContentList()
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '更新内容状态失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  async function deleteContent(id: string) {
    const item = options.contentItems.value.find((content) => content.id === id)
    if (!item || !window.confirm(`确定将“${item.title}”移入删除状态吗？`)) return
    options.contentBusy.value = true
    options.contentError.value = ''
    options.contentNotice.value = ''
    try {
      await request<unknown>(`/content/${encodeURIComponent(id)}`, { method: 'DELETE' }, '删除内容失败。')
      options.contentNotice.value = '内容已移入删除状态。'
      await options.loadContentList()
    } catch (error) {
      options.contentError.value = error instanceof Error ? error.message : '删除内容失败。'
    } finally {
      options.contentBusy.value = false
    }
  }

  return {
    initializeContentEditorRoute,
    openContentVersions,
    restoreContentVersion,
    restoreDeletedContent,
    permanentlyDeleteContent,
    saveContent,
    changeContentStatus,
    deleteContent,
  }
}
