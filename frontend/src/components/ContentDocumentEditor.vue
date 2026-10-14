<script setup lang="ts">
import type { Editor as CoreEditor } from '@tiptap/core'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { BubbleMenu } from '@tiptap/vue-3/menus'
import DragHandle from '@tiptap/extension-drag-handle'
import { createContentEditorExtensions, createContentEditorTools, executeContentEditorTool, normalizeEditorImageUrl, normalizeEditorLinkUrl } from '../utils/contentEditor'

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const slashMenuOpen = ref(false)
const slashQuery = ref('')
const slashIndex = ref(0)
const blockMenuOpen = ref(false)

const editor = useEditor({
  content: props.modelValue || '',
  contentType: 'markdown',
  editable: !props.disabled,
  extensions: [
    ...createContentEditorExtensions(),
    DragHandle.configure({
      render() {
        const button = document.createElement('button')
        button.type = 'button'
        button.className = 'content-editor-drag-handle'
        button.title = '拖动排序或打开块菜单'
        button.setAttribute('aria-label', '块操作菜单')
        button.setAttribute('draggable', 'true')
        button.innerHTML = '<span>⠿</span>'
        button.addEventListener('click', (event) => {
          event.preventDefault()
          event.stopPropagation()
          document.dispatchEvent(new CustomEvent('content-editor-open-block-menu'))
        })
        return button
      },
    }),
  ],
  editorProps: {
    attributes: { class: 'content-editor-document', 'aria-label': '文档正文编辑器', spellcheck: 'true' },
  },
  onUpdate: ({ editor: current }) => {
    emit('update:modelValue', current.getMarkdown())
    updateSlashMenu(current)
  },
  onSelectionUpdate: ({ editor: current }) => updateSlashMenu(current),
})

function updateSlashMenu(current: CoreEditor) {
  const { $from } = current.state.selection
  const text = $from.parent.textContent
  const before = text.slice(0, $from.parentOffset)
  const match = before.match(/^\/([^\s/]*)$/)
  slashQuery.value = match?.[1] || ''
  slashMenuOpen.value = !props.disabled && Boolean(match) && $from.parentOffset === text.length
  slashIndex.value = 0
}

const slashItems = computed(() => createContentEditorTools(editor.value, {
  addImage: current => {
    const value = window.prompt('输入图片 URL（当前支持在线图片链接）', 'https://')
    const url = value ? normalizeEditorImageUrl(value) : null
    if (url) current.chain().focus().setImage({ src: url, alt: '文档图片' }).run()
  },
  addLink: current => {
    const value = window.prompt('输入链接地址', 'https://')
    const url = value ? normalizeEditorLinkUrl(value) : null
    if (url) current.chain().focus().insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url } }] }).run()
  },
}).filter(item => !slashQuery.value || `${item.label} ${item.description} ${item.keywords}`.toLocaleLowerCase().includes(slashQuery.value.toLocaleLowerCase())))

function runSlashItem(index = slashIndex.value) {
  const current = editor.value
  const item = slashItems.value[index]
  if (!current || !item || props.disabled) return
  executeContentEditorTool(current, item)
  slashMenuOpen.value = false
  slashQuery.value = ''
}

function onKeydown(event: KeyboardEvent) {
  if (props.disabled || !slashMenuOpen.value) return
  if (event.key === 'Escape') { event.preventDefault(); slashMenuOpen.value = false; return }
  if (!slashItems.value.length) return
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    slashIndex.value = (slashIndex.value + 1) % slashItems.value.length
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    slashIndex.value = (slashIndex.value - 1 + slashItems.value.length) % slashItems.value.length
  } else if (event.key === 'Enter') {
    event.preventDefault()
    runSlashItem()
  } else if (event.key === 'Escape') {
    event.preventDefault()
    slashMenuOpen.value = false
  }
}

function addLinkToSelection() {
  const current = editor.value
  if (!current || props.disabled) return
  const value = window.prompt('输入链接地址', 'https://')
  const url = value ? normalizeEditorLinkUrl(value) : null
  if (url) current.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}

function onBlockMenuRequest() {
  if (!props.disabled) blockMenuOpen.value = !blockMenuOpen.value
}

function applyBlock(type: string) {
  const current = editor.value
  if (!current || props.disabled) return
  const chain = current.chain().focus()
  if (type === 'paragraph') chain.setParagraph().run()
  else if (type === 'heading') chain.setNode('heading', { level: 2 }).run()
  else if (type === 'bulletList') chain.toggleBulletList().run()
  else if (type === 'orderedList') chain.toggleOrderedList().run()
  else if (type === 'taskList') chain.toggleTaskList().run()
  else if (type === 'blockquote') chain.toggleBlockquote().run()
  else if (type === 'codeBlock') chain.toggleCodeBlock().run()
  else if (type === 'delete') chain.deleteNode(current.state.selection.$from.parent.type.name).run()
  blockMenuOpen.value = false
}

watch(() => props.modelValue, value => {
  const current = editor.value
  if (current && current.getMarkdown() !== (value || '')) {
    current.commands.setContent(value || '', { contentType: 'markdown', emitUpdate: false })
  }
})
watch(() => props.disabled, disabled => {
  editor.value?.setEditable(!disabled)
  if (disabled) {
    slashMenuOpen.value = false
    blockMenuOpen.value = false
  }
})
watch(editor, current => {
  current?.view.dom.addEventListener('keydown', onKeydown)
}, { immediate: true })
onMounted(() => document.addEventListener('content-editor-open-block-menu', onBlockMenuRequest))
onBeforeUnmount(() => {
  document.removeEventListener('content-editor-open-block-menu', onBlockMenuRequest)
  editor.value?.view.dom.removeEventListener('keydown', onKeydown)
  editor.value?.destroy()
})
</script>

<template>
  <div class="content-editor-shell" :class="{ 'is-disabled': disabled }">
    <div class="content-editor-hint">
      <span>文档编辑</span>
      <span>输入 <kbd>/</kbd> 插入内容 · 悬停段落左侧 <b>⠿</b> 拖动排序或转换块</span>
    </div>
    <div class="content-editor-canvas">
      <BubbleMenu v-if="editor" :editor="editor" :options="{ placement: 'top', offset: 8 }" class="content-editor-bubble-menu">
        <button type="button" title="加粗" aria-label="加粗" :class="{ active: editor.isActive('bold') }" :disabled="disabled" @mousedown.prevent="editor.chain().focus().toggleBold().run()"><b>B</b></button>
        <button type="button" title="斜体" aria-label="斜体" :class="{ active: editor.isActive('italic') }" :disabled="disabled" @mousedown.prevent="editor.chain().focus().toggleItalic().run()"><i>I</i></button>
        <button type="button" title="删除线" aria-label="删除线" :class="{ active: editor.isActive('strike') }" :disabled="disabled" @mousedown.prevent="editor.chain().focus().toggleStrike().run()"><s>S</s></button>
        <button type="button" title="高亮" aria-label="高亮" :class="{ active: editor.isActive('highlight') }" :disabled="disabled" @mousedown.prevent="editor.chain().focus().toggleHighlight().run()">▨</button>
        <button type="button" title="下划线" aria-label="下划线" :class="{ active: editor.isActive('underline') }" :disabled="disabled" @mousedown.prevent="editor.chain().focus().toggleUnderline().run()">U</button>
        <button type="button" title="添加链接" aria-label="添加链接" :disabled="disabled" @mousedown.prevent="addLinkToSelection()">↗</button>
      </BubbleMenu>
      <EditorContent :editor="editor" />
      <div v-if="slashMenuOpen && !disabled" class="content-editor-slash-menu" role="menu" aria-label="插入内容块">
        <div class="content-editor-menu-caption">插入内容块 <small>↑↓ 选择 · Enter 确认</small></div>
        <button v-for="(item, index) in slashItems" :key="item.label" type="button" role="menuitem" :class="{ selected: index === slashIndex }" @mousedown.prevent="runSlashItem(index)">
          <span class="content-editor-menu-icon">{{ item.icon }}</span><span><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
        </button>
        <p v-if="!slashItems.length" class="content-editor-no-results">没有匹配的插入项</p>
      </div>
      <div v-if="blockMenuOpen && !disabled" class="content-editor-block-menu" role="menu" aria-label="块操作">
        <div class="content-editor-menu-caption">块操作</div>
        <button v-for="item in [{label:'转为正文',type:'paragraph'},{label:'转为二级标题',type:'heading'},{label:'转为项目列表',type:'bulletList'},{label:'转为编号列表',type:'orderedList'},{label:'转为待办清单',type:'taskList'},{label:'转为引用块',type:'blockquote'},{label:'转为代码块',type:'codeBlock'},{label:'删除当前块',type:'delete'}]" :key="item.type" type="button" role="menuitem" @mousedown.prevent="applyBlock(item.type)">{{ item.label }}</button>
        <button type="button" role="menuitem" class="content-editor-cancel-action" @mousedown.prevent="blockMenuOpen = false">关闭</button>
      </div>
    </div>
    <div class="content-editor-footer"><span>支持 Markdown 持久化</span><span>待办、表格、代码块、图片与对齐格式</span></div>
  </div>
</template>
