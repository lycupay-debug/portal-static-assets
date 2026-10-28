/**
 * Responsibility: load a public article and render its Markdown body for the article view.
 * Dependencies: shared API client/generated article types; Markdown renderer is imported only on demand.
 * Collaboration: App.vue owns route and presentation refs; this loader owns request sequencing and
 * only commits a result while both the request sequence and route still identify the same article.
 * Side effects: performs a read-only API request and updates the supplied article/loading/error refs.
 */
import type { Ref } from 'vue'
import { apiRequest } from '../api/client'
import type { Content } from '../api/generated'

type PublicContentCard = Pick<Content, 'id' | 'title' | 'slug' | 'contentType' | 'tags' | 'parentCategory' | 'childCategory' | 'summary' | 'publishedAt'>
export type PublicContentArticle = PublicContentCard & Pick<Content, 'body' | 'createdAt' | 'updatedAt'> & { authorName: string; authorJoinedAt: string; likeCount: number; favoriteCount: number; viewerLiked: boolean; viewerFavorited: boolean }
type Requester = typeof apiRequest

type PublicArticleOptions = {
  route: Ref<string>
  article: Ref<PublicContentArticle | null>
  html: Ref<string>
  loading: Ref<boolean>
  error: Ref<string>
  request?: Requester
  renderMarkdown?: (markdown: string) => string | Promise<string>
}

export function createPublicArticleLoader(options: PublicArticleOptions) {
  const request = options.request || apiRequest
  let requestSequence = 0

  async function loadPublicArticle(slug: string): Promise<void> {
    const requestId = ++requestSequence
    const requestedRoute = options.route.value
    options.loading.value = true
    options.error.value = ''
    options.article.value = null
    options.html.value = ''
    try {
      const result = await request<{ item?: PublicContentArticle; error?: string }>(`/public/content/${slug}`, {}, '内容不存在或尚未公开。')
      if (!result.item) throw new Error(result.error || '内容不存在或尚未公开。')
      if (requestId !== requestSequence || options.route.value !== requestedRoute) return
      const renderMarkdown = options.renderMarkdown || (async (markdown: string) => {
        const { renderMarkdown: render } = await import('../utils/markdown')
        return render(markdown)
      })
      const html = await renderMarkdown(result.item.body)
      if (requestId !== requestSequence || options.route.value !== requestedRoute) return
      options.html.value = html
      options.article.value = result.item
    } catch (error) {
      if (requestId === requestSequence) options.error.value = error instanceof Error ? error.message : '读取内容失败。'
    } finally {
      if (requestId === requestSequence) options.loading.value = false
    }
  }

  return { loadPublicArticle }
}
