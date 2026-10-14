<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import { createContentEditorExtensions, createContentEditorTools, executeContentEditorTool, normalizeEditorImageUrl, normalizeEditorLinkUrl } from '../utils/contentEditor'

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
  extensions: createContentEditorExtensions(),
  editorProps: {
    attributes: { class: 'notion-document-content', 'aria-label': '文档正文编辑器', spellcheck: 'true' },
    handleKeyDown: (_view, event) => {
      if (!slashMenuOpen.value) return false
      if (event.key === 'ArrowDown') { event.preventDefault(); slashSelectedIndex.value = slashFilteredItems.value.length ? Math.min(slashSelectedIndex.value + 1, slashFilteredItems.value.length - 1) : 0; return true }
      if (event.key === 'ArrowUp') { event.preventDefault(); slashSelectedIndex.value = slashFilteredItems.value.length ? Math.max(slashSelectedIndex.value - 1, 0) : 0; return true }
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
      const canvas = current.view.dom.closest('.notion-editor')?.querySelector('.notion-editor-canvas')
      const coords = current.view.coordsAtPos($from.start())
      if (canvas) { const bounds = canvas.getBoundingClientRect(); slashMenuStyle.value = { top: String(Math.max(8, coords.bottom - bounds.top + 5)) + 'px', left: String(Math.max(8, Math.min(coords.left - bounds.left, bounds.width - 292))) + 'px' } }
    }
  },
})
const slashItems = computed(() => createContentEditorTools(editor.value, { addImage, addLink }))
const slashFilteredItems = computed(() => {
  const query = slashQuery.value.trim().toLocaleLowerCase()
  if (!query) return slashItems.value
  return slashItems.value.filter((item) => (item.label + ' ' + item.description + ' ' + item.keywords).toLocaleLowerCase().includes(query))
})
function runSlashCommand(item: typeof slashItems.value[number]) {
  const current = editor.value
  if (!current) return
  slashMenuOpen.value = false
  executeContentEditorTool(current, item)
}
function addLink() {
  const current = editor.value
  if (!current) return
  const previous = current.getAttributes('link').href as string | undefined
  const value = window.prompt('输入链接地址', previous || 'https://')
  if (value === null) return
  if (!value.trim()) {
    current.chain().focus().extendMarkRange('link').unsetLink().run()
    return
  }
  const url = normalizeEditorLinkUrl(value)
  if (!url) {
    window.alert('链接仅支持 http、https、mailto 或 tel 地址。')
    return
  }
  if (current.state.selection.empty) {
    current.chain().focus().insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url } }] }).run()
  } else {
    current.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
  }
}
function addImage() {
  const current = editor.value
  if (!current) return
  const value = window.prompt('输入图片 URL（当前仅支持在线图片链接）', 'https://')
  if (value === null || !value.trim()) return
  const url = normalizeEditorImageUrl(value)
  if (!url) {
    window.alert('图片地址必须是有效的 http 或 https 链接。')
    return
  }
  current.chain().focus().setImage({ src: url, alt: '文档图片' }).run()
}
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
