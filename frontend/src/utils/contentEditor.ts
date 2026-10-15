import { Extension, Mark, Node, type AnyExtension, type Editor, type MarkdownLexerConfiguration, type MarkdownParseHelpers, type MarkdownToken } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Paragraph } from '@tiptap/extension-paragraph'
import { Heading } from '@tiptap/extension-heading'
import { Markdown } from '@tiptap/markdown'
import { Table } from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import { TaskList, TaskItem } from '@tiptap/extension-list'
import Placeholder from '@tiptap/extension-placeholder'
import Image from '@tiptap/extension-image'
import Highlight from '@tiptap/extension-highlight'
import TextAlign from '@tiptap/extension-text-align'
import Link from '@tiptap/extension-link'

const ColorText = Mark.create({
  name: 'textColor',
  addAttributes() { return { color: { default: '#25334D', parseHTML: element => element.getAttribute('data-text-color') || '#25334D', renderHTML: attributes => ({ 'data-text-color': attributes.color, style: 'color: ' + attributes.color }) } } },
  parseHTML() { return [{ tag: 'span[data-text-color]' }] },
  renderHTML({ HTMLAttributes }) { return ['span', HTMLAttributes, 0] },
  markdownTokenName: 'coloredText',
  markdownTokenizer: {
    name: 'coloredText', level: 'inline', start: (source: string) => source.indexOf('{{color:'),
    tokenize(source: string, _tokens: MarkdownToken[], lexer: MarkdownLexerConfiguration): MarkdownToken | undefined {
      const match = source.match(/^\{\{color:(#[0-9a-fA-F]{6})\|([\s\S]+?)\}\}/)
      if (!match) return undefined
      return { type: 'coloredText', raw: match[0], color: match[1], tokens: lexer.inlineTokens(match[2] || '') }
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => helpers.applyMark('textColor', helpers.parseInline(token.tokens || []), { color: token.color }),
  renderMarkdown(node, helpers) { return '{{color:' + (node.attrs?.color || '#25334D') + '|' + helpers.renderChildren(node.content || []) + '}}' },
})

const ColorHighlight = Highlight.extend({
  addOptions() { return { ...(this.parent?.() ?? { multicolor: false, HTMLAttributes: {} }), multicolor: true } },
  markdownTokenName: 'coloredHighlight',
  markdownTokenizer: {
    name: 'coloredHighlight', level: 'inline', start: (source: string) => {
      const custom = source.indexOf('{{bg:')
      const standard = source.indexOf('==')
      return custom < 0 ? standard : standard < 0 ? custom : Math.min(custom, standard)
    },
    tokenize(source: string, _tokens: MarkdownToken[], lexer: MarkdownLexerConfiguration): MarkdownToken | undefined {
      const custom = source.match(/^\{\{bg:(#[0-9a-fA-F]{6})\|([\s\S]+?)\}\}/)
      if (custom) return { type: 'coloredHighlight', raw: custom[0], color: custom[1], tokens: lexer.inlineTokens(custom[2] || '') }
      const standard = source.match(/^==([\s\S]+?)==/)
      if (standard) return { type: 'coloredHighlight', raw: standard[0], tokens: lexer.inlineTokens(standard[1] || '') }
      return undefined
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => helpers.applyMark('highlight', helpers.parseInline(token.tokens || []), token.color ? { color: token.color } : {}),
  renderMarkdown(node, helpers) { const content = helpers.renderChildren(node.content || []); return node.attrs?.color ? '{{bg:' + node.attrs.color + '|' + content + '}}' : '==' + content + '==' },
})


const Callout = Node.create({
  name: 'callout', group: 'block', content: 'block+', defining: true,
  addAttributes() { return { backgroundColor: { default: '#FFF2CC', parseHTML: element => element.getAttribute('data-background-color') || '#FFF2CC', renderHTML: attrs => ({ 'data-background-color': attrs.backgroundColor, style: '--callout-bg:' + attrs.backgroundColor }) } } },
  parseHTML() { return [{ tag: 'div[data-callout]' }] },
  renderHTML({ HTMLAttributes }) { return ['div', { ...HTMLAttributes, 'data-callout': 'true', class: 'content-callout' }, 0] },
  markdownTokenName: 'calloutBlock',
  markdownTokenizer: {
    name: 'calloutBlock', level: 'block', start: (source: string) => source.match(/^:::callout(?:\s+#[0-9a-fA-F]{6})?\s*$/m)?.index ?? -1,
    tokenize(source: string, _tokens: MarkdownToken[], lexer: MarkdownLexerConfiguration): MarkdownToken | undefined {
      const match = source.match(/^:::callout(?:\s+(#[0-9a-fA-F]{6}))?\s*\n([\s\S]*?)\n:::[ \t]*(?:\n|$)/)
      if (!match) return undefined
      return { type: 'calloutBlock', raw: match[0], color: match[1] || '#FFF2CC', tokens: lexer.blockTokens(match[2] || '') }
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => ({ type: 'callout', attrs: { backgroundColor: token.color || '#FFF2CC' }, content: helpers.parseChildren(token.tokens || []) }),
  renderMarkdown(node, helpers) { return ':::callout ' + (node.attrs?.backgroundColor || '#FFF2CC') + '\n' + helpers.renderChildren(node.content || []) + '\n:::' },
})

export type ContentEditorAction = (editor: Editor) => void

export function normalizeEditorLinkUrl(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  const candidate = /^[a-z][a-z\d+.-]*:/i.test(trimmed) ? trimmed : 'https://' + trimmed
  try {
    const parsed = new URL(candidate)
    return ['http:', 'https:', 'mailto:', 'tel:'].includes(parsed.protocol) ? candidate : null
  } catch {
    return null
  }
}

export function normalizeEditorImageUrl(value: string): string | null {
  try {
    const parsed = new URL(value.trim())
    return ['http:', 'https:'].includes(parsed.protocol) ? parsed.toString() : null
  } catch {
    return null
  }
}

const AlignedParagraph = Paragraph.extend({
  renderMarkdown(node, helpers, context) {
    const markdown = this.parent?.(node, helpers, context) ?? helpers.renderChildren(node.content || [])
    const align = node.attrs?.textAlign
    return align === 'center' || align === 'right' ? ':::align-' + align + '\n' + markdown + '\n:::' : markdown
  },
})

const AlignedHeading = Heading.extend({
  renderMarkdown(node, helpers, context) {
    const markdown = this.parent?.(node, helpers, context) ?? helpers.renderChildren(node.content || [])
    const align = node.attrs?.textAlign
    return align === 'center' || align === 'right' ? ':::align-' + align + '\n' + markdown + '\n:::' : markdown
  },
})

const AlignmentMarkdown = Extension.create({
  name: 'alignmentMarkdown',
  markdownTokenName: 'alignedBlock',
  markdownTokenizer: {
    name: 'alignedBlock',
    level: 'block',
    start: (source: string) => source.match(/^:::align-(?:center|right)[ \t]*$/m)?.index ?? -1,
    tokenize(source: string, _tokens: MarkdownToken[], lexer: MarkdownLexerConfiguration): MarkdownToken | undefined {
      const match = source.match(/^:::align-(center|right)[ \t]*\n([\s\S]*?)\n:::[ \t]*(?:\n|$)/)
      if (!match) return undefined
      return { type: 'alignedBlock', raw: match[0], align: match[1], tokens: lexer.blockTokens(match[2] ?? '') }
    },
  },
  parseMarkdown: (token: MarkdownToken, helpers: MarkdownParseHelpers) => {
    return helpers.parseChildren(token.tokens || []).map(node => {
      if (node.type === 'paragraph' || node.type === 'heading') {
        return { ...node, attrs: { ...node.attrs, textAlign: token.align } }
      }
      return node
    })
  },
})

export interface ContentEditorTool {
  label: string
  description: string
  icon: string
  keywords: string
  run: () => void
}

export interface ContentEditorToolActions {
  addImage: ContentEditorAction
  addLink: ContentEditorAction
}

/** Keep the live editor and test suite on one authoritative extension configuration. */
export function createContentEditorExtensions(): AnyExtension[] {
  return [
    StarterKit.configure({ heading: false, paragraph: false, link: false }),
    AlignedParagraph,
    AlignedHeading.configure({ levels: [1, 2, 3] }),
    Markdown,
    AlignmentMarkdown,
    Callout,
    Table.configure({ resizable: true, allowTableNodeSelection: true }),
    TableRow,
    TableHeader,
    TableCell,
    TaskList,
    TaskItem.configure({
      nested: true,
      a11y: { checkboxLabel: node => `待办事项：${node.textContent || '未命名任务'}` },
    }),
    Placeholder.configure({
      placeholder: ({ node }) => node.type.name === 'heading' ? '标题' : '输入正文，输入 / 插入内容块…',
    }),
    Image.extend({
      addAttributes() {
        return {
          ...this.parent?.(),
          displayWidth: {
            default: null,
            parseHTML: element => element.getAttribute('data-image-width'),
            renderHTML: attrs => attrs.displayWidth
              ? { 'data-image-width': attrs.displayWidth, style: 'width:' + attrs.displayWidth + ';height:auto;max-width:100%' }
              : { style: 'width:100%;height:auto;max-width:100%' },
          },
        }
      },
    }).configure({ inline: false, allowBase64: true }),
    ColorHighlight,
    ColorText,
    Link.configure({ openOnClick: true, linkOnPaste: true, autolink: true, defaultProtocol: 'https', isAllowedUri: url => normalizeEditorLinkUrl(url) !== null }),
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
  ]
}

/** All slash-menu tools live here so each tool can be exercised in integration tests. */
export function createContentEditorTools(
  editor: Editor | null | undefined,
  actions: ContentEditorToolActions,
): ContentEditorTool[] {
  const run = (command: (current: Editor) => void) => () => {
    if (editor) command(editor)
  }

  return [
    { label: '文本', description: '普通段落', icon: 'T', keywords: 'text paragraph plain', run: run(current => { current.chain().focus().setParagraph().run() }) },
    { label: '一级标题', description: '大标题', icon: 'H1', keywords: 'h1 heading title', run: run(current => { current.chain().focus().toggleHeading({ level: 1 }).run() }) },
    { label: '二级标题', description: '章节标题', icon: 'H2', keywords: 'h2 heading title', run: run(current => { current.chain().focus().toggleHeading({ level: 2 }).run() }) },
    { label: '三级标题', description: '小节标题', icon: 'H3', keywords: 'h3 heading subtitle', run: run(current => { current.chain().focus().toggleHeading({ level: 3 }).run() }) },
    { label: '项目符号列表', description: '无序列表', icon: '•', keywords: 'bullet list unordered', run: run(current => { current.chain().focus().toggleBulletList().run() }) },
    { label: '编号列表', description: '有序列表', icon: '1.', keywords: 'number ordered list', run: run(current => { current.chain().focus().toggleOrderedList().run() }) },
    { label: '待办清单', description: '可勾选任务', icon: '☑', keywords: 'todo task checkbox checklist', run: run(current => { current.chain().focus().toggleTaskList().run() }) },
    { label: '引用', description: '引用一段内容', icon: '❞', keywords: 'quote blockquote', run: run(current => { current.chain().focus().toggleBlockquote().run() }) },
    { label: '提醒', description: '插入可自定义背景色的提醒卡片', icon: '💡', keywords: 'callout tip note warning info 提醒', run: run(current => { current.chain().focus().insertContent({ type: 'callout', attrs: { backgroundColor: '#FFF2CC' }, content: [{ type: 'paragraph' }] }).run() }) },
    { label: '代码块', description: '格式化代码片段', icon: '</>', keywords: 'code block snippet', run: run(current => { current.chain().focus().toggleCodeBlock().run() }) },
    { label: '分割线', description: '分隔文档内容', icon: '—', keywords: 'divider horizontal rule separator', run: run(current => { current.chain().focus().setHorizontalRule().run() }) },
    { label: '表格', description: '插入 3 × 3 表格', icon: '▦', keywords: 'table grid database', run: run(current => { current.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() }) },
    { label: '图片', description: '通过图片 URL 插入', icon: '▧', keywords: 'image photo picture', run: run(actions.addImage) },
    { label: '链接', description: '给选中文本添加链接', icon: '↗', keywords: 'link url hyperlink', run: run(actions.addLink) },
    { label: '加粗', description: '将文字设为粗体', icon: 'B', keywords: 'bold strong 粗体', run: run(current => { current.chain().focus().toggleBold().run() }) },
    { label: '斜体', description: '将文字设为斜体', icon: 'I', keywords: 'italic emphasis 斜体', run: run(current => { current.chain().focus().toggleItalic().run() }) },
    { label: '下划线', description: '为文字添加下划线', icon: 'U', keywords: 'underline 下划线', run: run(current => { current.chain().focus().toggleUnderline().run() }) },
    { label: '删除线', description: '为文字添加删除线', icon: 'S', keywords: 'strike strikethrough 删除线', run: run(current => { current.chain().focus().toggleStrike().run() }) },
    { label: '行内代码', description: '以代码样式显示文字', icon: '</>', keywords: 'inline code monospace 行内代码', run: run(current => { current.chain().focus().toggleCode().run() }) },
    { label: '高亮文本', description: '使用默认背景突出显示文字', icon: '▨', keywords: 'highlight mark color', run: run(current => { current.chain().focus().toggleHighlight({ color: '#FFF2A8' }).run() }) },
    { label: '背景色', description: '为选中文字设置背景色', icon: '▧', keywords: 'background highlight background color', run: run(current => { current.chain().focus().setHighlight({ color: '#FFF2A8' }).run() }) },
    { label: '文字颜色', description: '修改选中文字颜色', icon: 'A', keywords: 'text color font color', run: run(current => { current.chain().focus().setMark('textColor', { color: '#315CF5' }).run() }) },
    { label: '左对齐', description: '段落或标题左对齐', icon: '≡', keywords: 'align left', run: run(current => { current.chain().focus().setTextAlign('left').run() }) },
    { label: '居中对齐', description: '段落或标题居中', icon: '≣', keywords: 'align center middle', run: run(current => { current.chain().focus().setTextAlign('center').run() }) },
    { label: '右对齐', description: '段落或标题右对齐', icon: '≡', keywords: 'align right', run: run(current => { current.chain().focus().setTextAlign('right').run() }) },
    { label: '清除格式', description: '清除文字和段落格式', icon: 'Tx', keywords: 'clear format reset plain', run: run(current => { current.chain().focus().clearNodes().unsetAllMarks().unsetTextAlign().run() }) },
    { label: '撤销', description: '撤销上一步编辑', icon: '↶', keywords: 'undo back', run: run(current => { current.chain().focus().undo().run() }) },
    { label: '重做', description: '恢复已撤销的编辑', icon: '↷', keywords: 'redo forward', run: run(current => { current.chain().focus().redo().run() }) },
  ]
}

export function executeContentEditorTool(editor: Editor, tool: ContentEditorTool): void {
  const { $from } = editor.state.selection
  const textBeforeCursor = $from.parent.textContent.slice(0, $from.parentOffset)
  const slashMatch = textBeforeCursor.match(/^\/([^\n]*)$/)

  if (slashMatch) {
    editor.chain().focus().deleteRange({ from: $from.start(), to: $from.pos }).run()
  }

  tool.run()
}
