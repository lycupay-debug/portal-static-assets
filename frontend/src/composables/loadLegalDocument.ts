import type { Ref } from 'vue'

type LegalDocument = { slug: string; title: string; bodyMarkdown: string; effectiveAt?: string | null }
type Request = <T>(path: string, init?: RequestInit, fallbackMessage?: string) => Promise<T>
type MarkdownRenderer = (markdown: string) => string | Promise<string>
type State = {
  document: Ref<LegalDocument | null>
  html: Ref<string>
  loading: Ref<boolean>
  error: Ref<string>
}

/** Loads one public policy document and ignores stale route/request results. */
export async function loadLegalDocument(
  slug: string,
  route: Ref<string>,
  requestId: number,
  currentRequestId: () => number,
  state: State,
  request: Request,
  renderMarkdown: MarkdownRenderer,
): Promise<void> {
  const requestedRoute = `/legal/${slug}`
  state.loading.value = true
  state.error.value = ''
  state.document.value = null
  state.html.value = ''
  try {
    const { item, error } = await request<{ item?: LegalDocument; error?: string }>(
      `/public/legal/${encodeURIComponent(slug)}`, {}, '政策文件不存在或尚未发布。',
    )
    if (!item) throw new Error(error || '政策文件不存在或尚未发布。')
    if (requestId !== currentRequestId() || route.value !== requestedRoute) return
    state.html.value = await renderMarkdown(item.bodyMarkdown)
    if (requestId !== currentRequestId() || route.value !== requestedRoute) return
    state.document.value = item
  } catch (error) {
    if (requestId === currentRequestId() && route.value === requestedRoute) {
      state.error.value = error instanceof Error ? error.message : '读取政策文件失败。'
    }
  } finally {
    if (requestId === currentRequestId()) state.loading.value = false
  }
}
