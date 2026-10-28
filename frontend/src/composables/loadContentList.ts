/**
 * Responsibility: fetch active or trashed content records for the authenticated content-management view.
 * Dependencies: the shared API client and generated content type; App.vue supplies reactive state.
 * Collaboration: callers select the search term/view; this loader owns request sequencing and prevents
 * older searches from overwriting a newer list. It does not perform mutations or navigation.
 * Side effects: issues same-origin read requests and updates list/loading/error refs.
 */
import type { Ref } from 'vue'
import { apiRequest } from '../api/client'
import type { Content } from '../api/generated'

type Requester = typeof apiRequest

type ContentListOptions = {
  viewMode: Ref<'active' | 'trash'>
  activeItems: Ref<Content[]>
  trashItems: Ref<Content[]>
  loading: Ref<boolean>
  error: Ref<string>
  request?: Requester
}

export function createContentListLoader(options: ContentListOptions) {
  const request = options.request || apiRequest
  let requestSequence = 0

  async function loadContentList(searchTerm = ''): Promise<void> {
    const requestId = ++requestSequence
    options.loading.value = true
    options.error.value = ''
    try {
      const isTrash = options.viewMode.value === 'trash'
      const params = new URLSearchParams()
      if (searchTerm.trim()) params.set('q', searchTerm.trim())
      const endpoint = isTrash ? '/content/trash' : '/content'
      const result = await request<{ items?: Content[]; error?: string }>(`${endpoint}${params.size ? `?${params}` : ''}`, {}, '读取内容列表失败。')
      if (requestId !== requestSequence) return
      if (isTrash) options.trashItems.value = result.items || []
      else options.activeItems.value = result.items || []
    } catch (error) {
      if (requestId === requestSequence) options.error.value = error instanceof Error ? error.message : '网络连接失败，无法读取内容。'
    } finally {
      if (requestId === requestSequence) options.loading.value = false
    }
  }

  return { loadContentList }
}
