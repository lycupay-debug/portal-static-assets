import { computed, ref, type Ref } from 'vue'
import { apiRequest } from '../api/client'

type LegalDocumentSummary = { slug: string; title: string; documentType: string }
type LegalDocument = { slug: string; title: string; bodyMarkdown: string; effectiveAt?: string | null }
type Request = <T>(path: string, init?: RequestInit, fallbackMessage?: string) => Promise<T>
type MarkdownRenderer = (markdown: string) => string | Promise<string>

async function renderMarkdownDocument(markdown: string): Promise<string> {
  const { renderMarkdown } = await import('../utils/markdown')
  return renderMarkdown(markdown)
}

/** Owns legal-document state and delegates route-specific loading to a lazy module. */
export function useLegalDocuments(
  route: Ref<string>,
  request: Request = apiRequest,
  renderMarkdown: MarkdownRenderer = renderMarkdownDocument,
) {
  const legalSlug = computed(() => /^\/legal\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value) ? route.value.slice(7) : '')
  const publishedLegalDocuments = ref<LegalDocumentSummary[]>([])
  const legalDocument = ref<LegalDocument | null>(null)
  const legalDocumentHtml = ref('')
  const legalDocumentLoading = ref(false)
  const legalDocumentError = ref('')
  let legalDocumentRequestSequence = 0

  async function loadPublishedLegalDocuments(): Promise<void> {
    try {
      publishedLegalDocuments.value = (await request<{ items?: LegalDocumentSummary[] }>('/public/legal')).items || []
    } catch {
      // Footer policy links stay hidden until published documents are available.
    }
  }

  async function loadLegalDocument(slug: string): Promise<void> {
    const requestId = ++legalDocumentRequestSequence
    legalDocumentLoading.value = true
    legalDocumentError.value = ''
    legalDocument.value = null
    legalDocumentHtml.value = ''
    try {
      const { loadLegalDocument: load } = await import('./loadLegalDocument')
      await load(slug, route, requestId, () => legalDocumentRequestSequence, {
        document: legalDocument,
        html: legalDocumentHtml,
        loading: legalDocumentLoading,
        error: legalDocumentError,
      }, request, renderMarkdown)
    } catch (error) {
      if (requestId === legalDocumentRequestSequence && route.value === `/legal/${slug}`) {
        legalDocumentError.value = error instanceof Error ? error.message : '读取政策文件失败。'
        legalDocumentLoading.value = false
      }
    }
  }

  return {
    legalSlug,
    publishedLegalDocuments,
    legalDocument,
    legalDocumentHtml,
    legalDocumentLoading,
    legalDocumentError,
    loadPublishedLegalDocuments,
    loadLegalDocument,
  }
}
