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

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const slashMenuOpen = ref(false)
const slashQuery = ref('')
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
    TextAlign.configure({ types: ['heading', 'paragraph'] }),
  ],
  editorProps: { attributes: { class: 'notion-document-content', 'aria-label': '文档正文编辑器', spellcheck: 'true' } },
  onUpdate: ({ editor: current }) => {
    emit('update:modelValue', current.getMarkdown())
    const { $from } = current.state.selection
    const text = $from.parent.textContent
    slashQuery.value = text.startsWith('/') ? text.slice(1) : ''
    slashMenuOpen.value = text.startsWith('/') && !text.slice(1).includes(' ') && $from.parentOffset === text.length
  },
})
const slashItems = [
  { label: '文本', description: '普通段落', icon: 'T', run: () => editor.value?.chain().focus().setParagraph().run() },
  { label: '一级标题', description: '大标题', icon: 'H1', run: () => editor.value?.chain().focus().toggleHeading({ level: 1 }).run() },
  { label: '二级标题', description: '章节标题', icon: 'H2', run: () => editor.value?.chain().focus().toggleHeading({ level: 2 }).run() },
  { label: '项目符号列表', description: '无序列表', icon: '•', run: () => editor.value?.chain().focus().toggleBulletList().run() },
  { label: '编号列表', description: '有序列表', icon: '1.', run: () => editor.value?.chain().focus().toggleOrderedList().run() },
  { label: '待办清单', description: '可勾选任务', icon: '☑', run: () => editor.value?.chain().focus().toggleTaskList().run() },
  { label: '引用', description: '引用或提示块', icon: '❞', run: () => editor.value?.chain().focus().toggleBlockquote().run() },
  { label: '代码块', description: '格式化代码', icon: '</>', run: () => editor.value?.chain().focus().toggleCodeBlock().run() },
  { label: '分割线', description: '分隔文档内容', icon: '—', run: () => editor.value?.chain().focus().setHorizontalRule().run() },
  { label: '表格', description: '插入 3 × 3 表格', icon: '▦', run: () => editor.value?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
]
const slashFilteredItems = computed(() => {
  const query = slashQuery.value.trim().toLocaleLowerCase()
  if (!query) return slashItems
  const aliases: Record<string, string> = {
    '文本': 'text paragraph', '一级标题': 'h1 heading title', '二级标题': 'h2 heading title',
    '项目符号列表': 'bullet list unordered', '编号列表': 'number ordered list', '待办清单': 'todo task checkbox',
    '引用': 'quote blockquote callout', '代码块': 'code block', '分割线': 'divider horizontal rule', '表格': 'table',
  }
  return slashItems.filter((item) => `${item.label} ${item.description} ${aliases[item.label] || ''}`.toLocaleLowerCase().includes(query))
})
function runSlashCommand(item: typeof slashItems[number]) {
  const current = editor.value
  if (!current) return
  const { $from } = current.state.selection
  current.chain().focus().deleteRange({ from: $from.start(), to: $from.pos }).run()
  item.run()
}
function addLink() {
  const current = editor.value
  if (!current) return
  const previous = current.getAttributes('link').href as string | undefined
  const url = window.prompt('输入链接地址', previous || 'https://')
  if (url === null) return
  if (!url.trim()) current.chain().focus().extendMarkRange('link').unsetLink().run()
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
    <div class="notion-editor-toolbar" role="toolbar" aria-label="文档格式工具栏">
      <div class="notion-toolbar-group">
        <button type="button" title="正文" aria-label="正文" :class="{ 'is-active': editor?.isActive('paragraph') }" :disabled="disabled" @click="editor?.chain().focus().setParagraph().run()">¶</button>
        <button type="button" title="一级标题" aria-label="一级标题" :class="{ 'is-active': editor?.isActive('heading', { level: 1 }) }" :disabled="disabled" @click="editor?.chain().focus().toggleHeading({ level: 1 }).run()">H1</button>
        <button type="button" title="二级标题" aria-label="二级标题" :class="{ 'is-active': editor?.isActive('heading', { level: 2 }) }" :disabled="disabled" @click="editor?.chain().focus().toggleHeading({ level: 2 }).run()">H2</button>
        <button type="button" title="三级标题" aria-label="三级标题" :class="{ 'is-active': editor?.isActive('heading', { level: 3 }) }" :disabled="disabled" @click="editor?.chain().focus().toggleHeading({ level: 3 }).run()">H3</button>
      </div>
      <span class="notion-toolbar-divider" />
      <div class="notion-toolbar-group">
        <button type="button" title="加粗 Ctrl+B" aria-label="加粗" :class="{ 'is-active': editor?.isActive('bold') }" :disabled="disabled" @click="editor?.chain().focus().toggleBold().run()"><strong>B</strong></button>
        <button type="button" title="斜体 Ctrl+I" aria-label="斜体" :class="{ 'is-active': editor?.isActive('italic') }" :disabled="disabled" @click="editor?.chain().focus().toggleItalic().run()"><em>I</em></button>
        <button type="button" title="下划线" aria-label="下划线" :class="{ 'is-active': editor?.isActive('underline') }" :disabled="disabled" @click="editor?.chain().focus().toggleUnderline().run()"><u>U</u></button>
        <button type="button" title="删除线" aria-label="删除线" :class="{ 'is-active': editor?.isActive('strike') }" :disabled="disabled" @click="editor?.chain().focus().toggleStrike().run()"><s>S</s></button>
        <button type="button" title="高亮" aria-label="高亮" :class="{ 'is-active': editor?.isActive('highlight') }" :disabled="disabled" @click="editor?.chain().focus().toggleHighlight().run()">▨</button>
        <button type="button" title="行内代码" aria-label="行内代码" :class="{ 'is-active': editor?.isActive('code') }" :disabled="disabled" @click="editor?.chain().focus().toggleCode().run()">&lt;/&gt;</button>
        <button type="button" title="链接" aria-label="插入链接" :class="{ 'is-active': editor?.isActive('link') }" :disabled="disabled" @click="addLink">↗</button>
      </div>
      <span class="notion-toolbar-divider" />
      <div class="notion-toolbar-group">
        <button type="button" title="项目符号列表" aria-label="项目符号列表" :class="{ 'is-active': editor?.isActive('bulletList') }" :disabled="disabled" @click="editor?.chain().focus().toggleBulletList().run()">•≡</button>
        <button type="button" title="编号列表" aria-label="编号列表" :class="{ 'is-active': editor?.isActive('orderedList') }" :disabled="disabled" @click="editor?.chain().focus().toggleOrderedList().run()">1≡</button>
        <button type="button" title="待办清单" aria-label="待办清单" :class="{ 'is-active': editor?.isActive('taskList') }" :disabled="disabled" @click="editor?.chain().focus().toggleTaskList().run()">☑</button>
        <button type="button" title="引用块" aria-label="引用块" :class="{ 'is-active': editor?.isActive('blockquote') }" :disabled="disabled" @click="editor?.chain().focus().toggleBlockquote().run()">❞</button>
        <button type="button" title="代码块" aria-label="代码块" :class="{ 'is-active': editor?.isActive('codeBlock') }" :disabled="disabled" @click="editor?.chain().focus().toggleCodeBlock().run()">{ }</button>
        <button type="button" title="插入表格" aria-label="插入表格" :disabled="disabled" @click="addTable">▦</button>
        <button type="button" title="插入图片 URL" aria-label="插入图片" :disabled="disabled" @click="addImage">▧</button>
        <button type="button" title="分割线" aria-label="分割线" :disabled="disabled" @click="editor?.chain().focus().setHorizontalRule().run()">―</button>
      </div>
      <span class="notion-toolbar-spacer" />
      <div class="notion-toolbar-group notion-history-tools">
        <button type="button" title="撤销 Ctrl+Z" aria-label="撤销" :disabled="disabled || !editor?.can().undo()" @click="editor?.chain().focus().undo().run()">↶</button>
        <button type="button" title="重做 Ctrl+Shift+Z" aria-label="重做" :disabled="disabled || !editor?.can().redo()" @click="editor?.chain().focus().redo().run()">↷</button>
      </div>
    </div>
    <div class="notion-editor-canvas">
      <EditorContent :editor="editor" />
      <div v-if="slashMenuOpen" class="notion-slash-menu" role="menu" aria-label="插入内容块">
        <div class="notion-slash-heading">插入内容块</div>
        <p v-if="!slashFilteredItems.length" class="notion-slash-empty">没有匹配的内容块</p>
        <button v-for="item in slashFilteredItems" :key="item.label" type="button" role="menuitem" @mousedown.prevent="runSlashCommand(item)">
          <span class="notion-slash-icon">{{ item.icon }}</span><span class="notion-slash-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
        </button>
      </div>
    </div>
    <div class="notion-editor-footer"><span>Markdown 文档</span><span>输入 <kbd>/</kbd> 插入内容块 · <kbd>Ctrl</kbd> + <kbd>Z</kbd> 撤销</span></div>
  </div>
</template>
