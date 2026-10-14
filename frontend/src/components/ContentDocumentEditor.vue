<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { Table } from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import TaskList from '@tiptap/extension-task-list'
import TaskItem from '@tiptap/extension-task-item'
import Placeholder from '@tiptap/extension-placeholder'
import Image from '@tiptap/extension-image'
import Highlight from '@tiptap/extension-highlight'
import TextAlign from '@tiptap/extension-text-align'
import Link from '@tiptap/extension-link'

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const slashMenuOpen = ref(false)
const slashQuery = ref('')
const slashSelectedIndex = ref(0)
const slashMenuStyle = ref<Record<string, string>>({ top: '8px', left: '8px' })
const editor = useEditor({
  content: props.modelValue,
  contentType: 'markdown',
  editable: !props.disabled,
  extensions: [
    StarterKit.configure({ heading: { levels: [1, 2, 3] } }), Markdown,
    Table.configure({ resizable: true, allowTableNodeSelection: true }), TableRow, TableHeader, TableCell,
    TaskList, TaskItem.configure({ nested: true }),
    Placeholder.configure({ placeholder: ({ node }) => node.type.name === 'heading' ? '标题' : '输入正文，输入 / 插入内容块…' }),
    Image.configure({ inline: false, allowBase64: false }), Highlight,
    Link.configure({ openOnClick: false, autolink: true, defaultProtocol: 'https' }),
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
  ],
  editorProps: {
    attributes: { class: 'notion-document-content', 'aria-label': '文档正文编辑器', spellcheck: 'true' },
    handleKeyDown: (_view, event) => {
      if (!slashMenuOpen.value) return false
      if (event.key === 'ArrowDown') { event.preventDefault(); slashSelectedIndex.value = Math.min(slashSelectedIndex.value + 1, slashFilteredItems.value.length - 1); return true }
      if (event.key === 'ArrowUp') { event.preventDefault(); slashSelectedIndex.value = Math.max(slashSelectedIndex.value - 1, 0); return true }
      if (event.key === 'Escape') { event.preventDefault(); slashMenuOpen.value = false; return true }
      if (event.key === 'Enter' && slashFilteredItems.value.length) { event.preventDefault(); const item = slashFilteredItems.value[slashSelectedIndex.value]; if (item) runSlashCommand(item); return true }
      return false
    },
  },
  onUpdate: ({ editor: current }) => {
    emit('update:modelValue', current.getMarkdown())
    const { $from } = current.state.selection
    const textBeforeCursor = $from.parent.textContent.slice(0, $from.parentOffset)
    const slashMatch = textBeforeCursor.match(/^\/([^\n]*)$/)
    slashQuery.value = slashMatch?.[1] || ''
    slashMenuOpen.value = Boolean(slashMatch)
    slashSelectedIndex.value = 0
    if (slashMatch) {
      const canvas = document.querySelector('.notion-editor-canvas')
      const coords = current.view.coordsAtPos($from.start())
      if (canvas) { const bounds = canvas.getBoundingClientRect(); slashMenuStyle.value = { top: String(Math.max(8, coords.bottom - bounds.top + 5)) + 'px', left: String(Math.max(8, Math.min(coords.left - bounds.left, bounds.width - 292))) + 'px' } }
    }
  },
})
const slashItems = [
  { label: '文本', description: '普通段落', icon: 'T', keywords: 'text paragraph plain', run: () => editor.value?.chain().focus().setParagraph().run() },
  { label: '一级标题', description: '大标题', icon: 'H1', keywords: 'h1 heading title', run: () => editor.value?.chain().focus().toggleHeading({ level: 1 }).run() },
  { label: '二级标题', description: '章节标题', icon: 'H2', keywords: 'h2 heading title', run: () => editor.value?.chain().focus().toggleHeading({ level: 2 }).run() },
  { label: '三级标题', description: '小节标题', icon: 'H3', keywords: 'h3 heading subtitle', run: () => editor.value?.chain().focus().toggleHeading({ level: 3 }).run() },
  { label: '项目符号列表', description: '无序列表', icon: '•', keywords: 'bullet list unordered', run: () => editor.value?.chain().focus().toggleBulletList().run() },
  { label: '编号列表', description: '有序列表', icon: '1.', keywords: 'number ordered list', run: () => editor.value?.chain().focus().toggleOrderedList().run() },
  { label: '待办清单', description: '可勾选任务', icon: '☑', keywords: 'todo task checkbox checklist', run: () => editor.value?.chain().focus().toggleTaskList().run() },
  { label: '引用', description: '引用一段内容', icon: '❞', keywords: 'quote blockquote', run: () => editor.value?.chain().focus().toggleBlockquote().run() },
  { label: '提示块', description: '创建醒目的提示内容', icon: '💡', keywords: 'callout tip note warning info', run: () => editor.value?.chain().focus().toggleBlockquote().insertContent('💡 提示：').run() },
  { label: '代码块', description: '格式化代码片段', icon: '</>', keywords: 'code block snippet', run: () => editor.value?.chain().focus().toggleCodeBlock().run() },
  { label: '分割线', description: '分隔文档内容', icon: '—', keywords: 'divider horizontal rule separator', run: () => editor.value?.chain().focus().setHorizontalRule().run() },
  { label: '表格', description: '插入 3 × 3 表格', icon: '▦', keywords: 'table grid database', run: () => editor.value?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
  { label: '图片', description: '通过图片 URL 插入', icon: '▧', keywords: 'image photo picture', run: addImage },
  { label: '链接', description: '给选中文本添加链接', icon: '↗', keywords: 'link url hyperlink', run: addLink },
  { label: '加粗', description: '将接下来输入的文字加粗', icon: 'B', keywords: 'bold strong 粗体', run: () => editor.value?.chain().focus().toggleBold().run() },
  { label: '斜体', description: '将接下来输入的文字设为斜体', icon: 'I', keywords: 'italic emphasis 斜体', run: () => editor.value?.chain().focus().toggleItalic().run() },
  { label: '下划线', description: '为接下来输入的文字添加下划线', icon: 'U', keywords: 'underline 下划线', run: () => editor.value?.chain().focus().toggleUnderline().run() },
  { label: '删除线', description: '为接下来输入的文字添加删除线', icon: 'S', keywords: 'strike strikethrough 删除线', run: () => editor.value?.chain().focus().toggleStrike().run() },
  { label: '行内代码', description: '以代码样式输入文字', icon: '</>', keywords: 'inline code monospace 行内代码', run: () => editor.value?.chain().focus().toggleCode().run() },
  { label: '高亮文本', description: '突出显示选中的文字', icon: '▨', keywords: 'highlight mark color', run: () => editor.value?.chain().focus().toggleHighlight().run() },
  { label: '左对齐', description: '段落或标题左对齐', icon: '≡', keywords: 'align left', run: () => editor.value?.chain().focus().setTextAlign('left').run() },
  { label: '居中对齐', description: '段落或标题居中', icon: '≣', keywords: 'align center middle', run: () => editor.value?.chain().focus().setTextAlign('center').run() },
  { label: '右对齐', description: '段落或标题右对齐', icon: '≡', keywords: 'align right', run: () => editor.value?.chain().focus().setTextAlign('right').run() },
  { label: '清除格式', description: '恢复为普通文本样式', icon: 'Tx', keywords: 'clear format reset plain', run: () => editor.value?.chain().focus().clearNodes().unsetAllMarks().run() },
]
const slashFilteredItems = computed(() => {
  const query = slashQuery.value.trim().toLocaleLowerCase()
  if (!query) return slashItems
  return slashItems.filter((item) => (item.label + ' ' + item.description + ' ' + item.keywords).toLocaleLowerCase().includes(query))
})
function runSlashCommand(item: typeof slashItems[number]) {
  const current = editor.value
  if (!current) return
  const { $from } = current.state.selection
  current.chain().focus().deleteRange({ from: $from.start(), to: $from.pos }).run()
  slashMenuOpen.value = false
  item.run()
}
function addLink() {
  const current = editor.value
  if (!current) return
  const previous = current.getAttributes('link').href as string | undefined
  const url = window.prompt('输入链接地址', previous || 'https://')
  if (url === null) return
  if (!url.trim()) current.chain().focus().extendMarkRange('link').unsetLink().run()
  else if (current.state.selection.empty) current.chain().focus().insertContent({ type: 'text', text: url.trim(), marks: [{ type: 'link', attrs: { href: url.trim() } }] }).run()
  else current.chain().focus().extendMarkRange('link').setLink({ href: url.trim() }).run()
}
function addImage() {
  const url = window.prompt('输入图片 URL（当前仅支持在线图片链接）', 'https://')
  if (url?.trim()) editor.value?.chain().focus().setImage({ src: url.trim(), alt: '文档图片' }).run()
}
function addTable() { editor.value?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() }
watch(() => props.modelValue, (value) => {
  const current = editor.value
  if (current && current.getMarkdown() !== value) current.commands.setContent(value, { contentType: 'markdown', emitUpdate: false })
})
watch(() => props.disabled, (value) => editor.value?.setEditable(!value))
onBeforeUnmount(() => editor.value?.destroy())
</script>

<template>
  <div class="notion-editor" :class="{ 'is-disabled': disabled }">
    <div class="notion-editor-canvas">
      <EditorContent :editor="editor" />
      <div v-if="slashMenuOpen" class="notion-slash-menu" :style="slashMenuStyle" role="menu" aria-label="插入内容块">
        <div class="notion-slash-heading">插入内容块</div>
        <p v-if="!slashFilteredItems.length" class="notion-slash-empty">没有匹配的内容块</p>
        <button v-for="(item, index) in slashFilteredItems" :key="item.label" type="button" role="menuitem" :class="{ 'is-selected': index === slashSelectedIndex }" @mousedown.prevent="runSlashCommand(item)">
          <span class="notion-slash-icon">{{ item.icon }}</span><span class="notion-slash-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
        </button>
      </div>
    </div>
    <div class="notion-editor-footer"><span>Markdown 文档</span><span>输入 <kbd>/</kbd> 插入内容块 · <kbd>Ctrl</kbd> + <kbd>Z</kbd> 撤销</span></div>
  </div>
</template>
