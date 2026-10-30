/**
 * Responsibility: fetch public knowledge/article and education-resource directory lists.
 * Dependencies: the shared same-origin API client and generated API response types.
 * Collaboration: portalApplication.ts owns route selection and presentation state; this module updates the
 * supplied refs and owns request sequencing so an older search cannot replace a newer result.
 * Side effects: performs HTTP GET requests and updates loading/error/list refs; it does not
 * navigate, mutate server data, or own component lifecycle.
 */
import type { MutableValue } from '../state/valueContracts'
import { apiRequest } from '../api/client'
import type { Content, TeachingResource } from '../api/generated'

type PublicContentCard = Pick<Content, 'id' | 'title' | 'slug' | 'contentType' | 'tags' | 'parentCategory' | 'childCategory' | 'summary' | 'publishedAt'>
type PublicTeachingResource = Pick<TeachingResource, 'id' | 'title' | 'description' | 'type' | 'grade' | 'url' | 'filename' | 'fileSize' | 'createdAt' | 'updatedAt'> & { downloadUrl?: string | null }
type Requester = typeof apiRequest

interface PublicDirectoryDataOptions {
  publicContents: MutableValue<PublicContentCard[]>
  publicTeachingResources: MutableValue<PublicTeachingResource[]>
  publicContentLoading: MutableValue<boolean>
  publicContentError: MutableValue<string>
  educationResourceLoading: MutableValue<boolean>
  educationResourceError: MutableValue<string>
  knowledgeVisibleCount: MutableValue<number>
  knowledgePageSize: number
  isContentViewCurrent?: () => boolean
  request?: Requester
}

/** Loads public directory lists; request sequences prevent older responses overwriting newer searches. */
export function createPublicDirectoryLoader(options: PublicDirectoryDataOptions) {
  const request = options.request || apiRequest
  const isContentViewCurrent = options.isContentViewCurrent || (() => true)
  let contentRequestSequence = 0
  let resourceRequestSequence = 0

  async function loadPublicContents(searchTerm = ''): Promise<void> {
    const requestId = ++contentRequestSequence
    options.publicContentLoading.value = true
    options.publicContentError.value = ''
    try {
      const params = new URLSearchParams()
      if (searchTerm.trim()) params.set('q', searchTerm.trim())
      const result = await request<{ items?: PublicContentCard[]; error?: string }>(`/public/content${params.size ? `?${params}` : ''}`, {}, '读取公开内容失败。')
      if (requestId !== contentRequestSequence || !isContentViewCurrent()) return
      options.publicContents.value = result.items || []
      options.knowledgeVisibleCount.value = options.knowledgePageSize
    } catch (error) {
      if (requestId === contentRequestSequence && isContentViewCurrent()) {
        options.publicContentError.value = error instanceof Error ? error.message : '读取公开内容失败。'
      }
    } finally {
      if (requestId === contentRequestSequence && isContentViewCurrent()) options.publicContentLoading.value = false
    }
  }

  async function loadPublicTeachingResources(searchTerm = ''): Promise<void> {
    const requestId = ++resourceRequestSequence
    options.educationResourceLoading.value = true
    options.educationResourceError.value = ''
    try {
      const params = new URLSearchParams()
      if (searchTerm.trim()) params.set('q', searchTerm.trim())
      const result = await request<{ items?: PublicTeachingResource[]; error?: string }>(`/public/resources${params.size ? `?${params}` : ''}`, {}, '读取公开教育资源失败。')
      if (requestId !== resourceRequestSequence) return
      options.publicTeachingResources.value = result.items || []
      options.knowledgeVisibleCount.value = options.knowledgePageSize
    } catch (error) {
      if (requestId === resourceRequestSequence) options.educationResourceError.value = error instanceof Error ? error.message : '读取公开教育资源失败。'
    } finally {
      if (requestId === resourceRequestSequence) options.educationResourceLoading.value = false
    }
  }

  return { loadPublicContents, loadPublicTeachingResources }
}
