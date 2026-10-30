/**
 * Responsibility: load the authenticated teaching-resource list or trash collection.
 * Dependencies: the shared API client and generated resource type; portalApplication.ts owns view and UI state.
 * Collaboration: callers select active/trash mode and search term. A monotonically increasing request
 * sequence prevents a slower request from replacing a newer result; the captured view is also checked.
 * Side effects: performs read-only API requests and updates the supplied refs; it does not mutate records.
 */
import type { MutableValue } from '../state/valueContracts'
import { apiRequest } from '../api/client'
import type { TeachingResource } from '../api/generated'

type Requester = typeof apiRequest

type TeachingResourceListOptions = {
  viewMode: MutableValue<'active' | 'trash'>
  activeItems: MutableValue<TeachingResource[]>
  trashItems: MutableValue<TeachingResource[]>
  loading: MutableValue<boolean>
  error: MutableValue<string>
  request?: Requester
  onError?: (message: string) => void
}

export function createTeachingResourceListLoader(options: TeachingResourceListOptions) {
  const request = options.request || apiRequest
  let requestSequence = 0

  async function loadTeachingResources(searchTerm = ''): Promise<void> {
    const requestId = ++requestSequence
    const requestedView = options.viewMode.value
    options.loading.value = true
    options.error.value = ''
    try {
      const params = new URLSearchParams()
      if (searchTerm.trim()) params.set('q', searchTerm.trim())
      const endpoint = requestedView === 'trash' ? '/resources/trash' : '/resources'
      const result = await request<{ items?: TeachingResource[]; error?: string }>(`${endpoint}${params.size ? `?${params}` : ''}`, {}, '读取教学资源失败。')
      if (requestId !== requestSequence || options.viewMode.value !== requestedView) return
      if (requestedView === 'trash') options.trashItems.value = result.items || []
      else options.activeItems.value = result.items || []
    } catch (error) {
      if (requestId === requestSequence && options.viewMode.value === requestedView) {
        options.error.value = error instanceof Error ? error.message : '读取教学资源失败。'
        options.onError?.(options.error.value)
      }
    } finally {
      if (requestId === requestSequence) options.loading.value = false
    }
  }

  return { loadTeachingResources }
}
