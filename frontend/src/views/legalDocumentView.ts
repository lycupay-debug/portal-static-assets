/**
 * Responsibility: render a published legal document using native DOM and already-sanitized Markdown HTML.
 * Dependencies: caller-owned legal-document state and route navigation callback.
 * Collaboration: the legal-document controller loads and sanitizes content; this view only presents that result.
 * Boundaries: untrusted Markdown must pass through renderMarkdown/DOMPurify before update; document metadata is assigned as text.
 */
type LegalDocument = { title: string; effectiveAt?: string | null }
type LegalDocumentState = { document: LegalDocument | null; html: string; loading: boolean; error: string }

export function mountLegalDocumentView(host: HTMLElement, navigate: (target: string) => unknown) {
  const document = host.ownerDocument
  const shell = document.createElement('section')
  shell.className = 'legal-document-shell'
  const back = document.createElement('a')
  back.className = 'legal-back-link'
  back.href = '/'
  back.textContent = '← 返回首页'
  const content = document.createElement('section')
  content.className = 'legal-document-content'
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  eyebrow.textContent = 'POLICIES & NOTICES'
  const title = document.createElement('h1')
  const effective = document.createElement('p')
  effective.className = 'legal-document-effective'
  const markdown = document.createElement('div')
  markdown.className = 'markdown-rendered legal-markdown'
  content.append(eyebrow, title, effective, markdown)
  const state = document.createElement('section')
  state.className = 'legal-document-state'
  shell.append(back, state, content)
  host.replaceChildren(shell)

  const onBack = (event: MouseEvent) => { event.preventDefault(); navigate('/') }
  back.addEventListener('click', onBack)

  function update(value: LegalDocumentState) {
    state.replaceChildren()
    content.hidden = true
    state.hidden = false
    if (value.loading) {
      state.textContent = '正在读取政策文件…'
      return
    }
    if (!value.document) {
      const heading = document.createElement('h1')
      heading.textContent = '政策文件暂不可用'
      const message = document.createElement('p')
      message.textContent = value.error || '该政策尚未发布。'
      state.append(heading, message)
      return
    }
    title.textContent = value.document.title
    if (value.document.effectiveAt) {
      const date = new Date(value.document.effectiveAt)
      effective.textContent = `生效日期：${Number.isNaN(date.getTime()) ? value.document.effectiveAt : date.toLocaleDateString('zh-CN')}`
      effective.hidden = false
    } else {
      effective.textContent = ''
      effective.hidden = true
    }
    // The controller's renderer is DOMPurify-backed; raw API Markdown is never assigned here.
    markdown.innerHTML = value.html
    content.hidden = false
    state.hidden = true
  }

  return {
    update,
    dispose() {
      back.removeEventListener('click', onBack)
      host.replaceChildren()
    },
  }
}
