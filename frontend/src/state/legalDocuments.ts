/**
 * Responsibility: own public legal-document state and request sequencing without a UI-framework dependency.
 * Dependencies: structural value factory, API request transport and Markdown renderer.
 * Collaboration: portalApplication.ts currently adapts signal-backed state cells; a native app can supply observable value cells unchanged.
 * Boundaries: requests are read-only; stale route results are ignored; publication and access policy remain server-owned.
 */
import { apiRequest } from '../api/client'
import type { MutableValue, ReadonlyValue } from './valueContracts'

type LegalDocumentSummary = { slug: string; title: string; documentType: string }
type LegalDocument = { slug: string; title: string; bodyMarkdown: string; effectiveAt?: string | null }
type Request = <T>(path: string, init?: RequestInit, fallbackMessage?: string) => Promise<T>
type MarkdownRenderer = (markdown: string) => string | Promise<string>
type StateFactory = <T>(initial: T) => MutableValue<T>

async function renderMarkdownDocument(markdown: string): Promise<string> {
  const { renderMarkdown } = await import('../utils/markdown')
  return renderMarkdown(markdown)
}

export function createLegalDocumentsController(
  route: ReadonlyValue<string>,
  dependencies: { createValue: StateFactory; request?: Request; renderMarkdown?: MarkdownRenderer },
) {
  const request = dependencies.request ?? apiRequest
  const renderMarkdown = dependencies.renderMarkdown ?? renderMarkdownDocument
  const legalSlug: ReadonlyValue<string> = { get value() { return /^\/legal\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value) ? route.value.slice(7) : '' } }
  const publishedLegalDocuments = dependencies.createValue<LegalDocumentSummary[]>([])
  const legalDocument = dependencies.createValue<LegalDocument | null>(null)
  const legalDocumentHtml = dependencies.createValue('')
  const legalDocumentLoading = dependencies.createValue(false)
  const legalDocumentError = dependencies.createValue('')
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
      const { loadLegalDocument: load } = await import('../composables/loadLegalDocument')
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

  return { legalSlug, publishedLegalDocuments, legalDocument, legalDocumentHtml, legalDocumentLoading, legalDocumentError, loadPublishedLegalDocuments, loadLegalDocument }
}
