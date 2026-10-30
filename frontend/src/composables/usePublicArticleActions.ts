import type { MutableValue } from '../state/valueContracts'
import { ApiRequestError, apiRequest } from '../api/client'
import { portalNotify } from '../utils/notification'

type ArticleInteraction = 'like' | 'favorite'
type InteractiveArticle = {
  slug: string
  likeCount: number
  favoriteCount: number
  viewerLiked: boolean
  viewerFavorited: boolean
}
type Notifier = {
  success(message: string, title?: string): void
  error(message: string, title?: string): void
}
type Requester = typeof apiRequest

interface PublicArticleActionOptions<T extends InteractiveArticle> {
  article: MutableValue<T | null>
  busy: MutableValue<boolean>
  authNotice: MutableValue<string>
  shareDialogOpen: MutableValue<boolean>
  shareUrl: MutableValue<string>
  navigate: (target: string) => Promise<unknown> | unknown
  request?: Requester
  notify?: Notifier
}

/** Keeps public article interaction and share-link behavior out of the route shell. */
export function createPublicArticleActions<T extends InteractiveArticle>(options: PublicArticleActionOptions<T>) {
  const request = options.request || apiRequest
  const notify = options.notify || portalNotify

  async function toggleInteraction(type: ArticleInteraction) {
    const article = options.article.value
    if (!article || options.busy.value) return
    options.busy.value = true
    try {
      const result = await request<{ active?: boolean; likeCount?: number; favoriteCount?: number; error?: string }>(`/public/content/${encodeURIComponent(article.slug)}/interactions`, {
        method: 'POST', body: JSON.stringify({ type }),
      }, '操作失败，请稍后重试。')
      options.article.value = {
        ...article,
        likeCount: result.likeCount ?? article.likeCount,
        favoriteCount: result.favoriteCount ?? article.favoriteCount,
        ...(type === 'like' ? { viewerLiked: Boolean(result.active) } : { viewerFavorited: Boolean(result.active) }),
      }
      notify.success(type === 'like' ? (result.active ? '已点赞' : '已取消点赞') : (result.active ? '已收藏' : '已取消收藏'))
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 401) {
        options.authNotice.value = '请先登录，再进行点赞或收藏。'
        await options.navigate('/login')
        return
      }
      notify.error(error instanceof Error ? error.message : '操作失败。')
    } finally {
      options.busy.value = false
    }
  }

  function shareArticle() {
    if (!options.article.value) return
    options.shareUrl.value = window.location.href
    options.shareDialogOpen.value = true
  }

  async function copyShareUrl() {
    if (!options.shareUrl.value) return
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(options.shareUrl.value)
      } else {
        const input = document.querySelector<HTMLInputElement>('.share-dialog-url')
        if (!input) throw new Error('无法读取分享链接。')
        input.focus()
        input.select()
        if (!document.execCommand('copy')) throw new Error('浏览器未能复制链接，请手动选择并复制。')
      }
      notify.success('分享链接已复制')
      options.shareDialogOpen.value = false
    } catch (error) {
      notify.error(error instanceof Error ? error.message : '复制失败，请手动复制链接。')
    }
  }

  return { toggleInteraction, shareArticle, copyShareUrl }
}
