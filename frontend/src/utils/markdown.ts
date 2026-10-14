import DOMPurify from 'dompurify'
import { marked } from 'marked'

function escapeHtml(value: string): string {
  return value.replace(/[&<>\"']/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '\"': '&quot;', "'": '&#39;',
  })[character] || character)
}

const renderer = new marked.Renderer()
renderer.html = ({ text }) => escapeHtml(text)

const markdownOptions = {
  async: false as const,
  breaks: false,
  gfm: true,
  pedantic: false,
  renderer,
  html: false,
}

export function renderMarkdown(source: string, sanitizer = DOMPurify): string {
  const markdown = typeof source === 'string' ? source : ''
  const html = marked.parse(markdown, markdownOptions)
  return sanitizer.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'script', 'iframe', 'object', 'embed', 'form', 'input', 'button', 'svg', 'math'],
    FORBID_ATTR: ['style', 'onerror', 'onload', 'onclick'],
  })
}
