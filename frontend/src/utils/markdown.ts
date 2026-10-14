import DOMPurify from 'dompurify'
import { Marked, Renderer } from 'marked'

function escapeHtml(value: string): string {
  return value.replace(/[&<>\"']/g, (character) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[character] || character)
}

const renderer = new Renderer()
renderer.html = ({ text }) => escapeHtml(text)

let markdownParser: Marked

const alignmentExtension = {
  name: 'alignedBlock',
  level: 'block' as const,
  start(source: string) {
    return source.match(/^:::align-(?:center|right)[ \t]*$/m)?.index ?? -1
  },
  tokenizer(source: string) {
    const match = source.match(/^:::align-(center|right)[ \t]*\n([\s\S]*?)\n:::[ \t]*(?:\n|$)/)
    if (!match) return undefined
    return { type: 'alignedBlock', raw: match[0], align: match[1], text: match[2] ?? '' }
  },
  renderer(token: { align: string; text: string }): string {
    const align = token.align === 'right' ? 'right' : 'center'
    return '<div class="md-align-' + align + '">' + markdownParser.parse(token.text) + '</div>\n'
  },
}

const markdownOptions = {
  async: false as const,
  breaks: false,
  gfm: true,
  pedantic: false,
  renderer,
  html: false,
}

// Use an isolated parser instance so custom tokenizers do not mutate Marked's global singleton.
markdownParser = new Marked({ ...markdownOptions, extensions: [alignmentExtension] })

export function renderMarkdown(source: string, sanitizer = DOMPurify): string {
  const markdown = typeof source === 'string' ? source : ''
  const html = markdownParser.parse(markdown) as string
  return sanitizer.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'script', 'iframe', 'object', 'embed', 'form', 'input', 'button', 'svg', 'math'],
    FORBID_ATTR: ['style', 'onerror', 'onload', 'onclick'],
  })
}
