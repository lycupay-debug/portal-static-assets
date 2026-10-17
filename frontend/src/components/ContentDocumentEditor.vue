<script setup lang="ts">
import type { Editor as CoreEditor } from '@tiptap/core'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import DragHandle from '@tiptap/extension-drag-handle'
import { createContentEditorExtensions, createContentEditorTools, executeContentEditorTool, normalizeEditorImageUrl, normalizeEditorLinkUrl } from '../utils/contentEditor'
import { portalNotify } from '../utils/notification'

const props = defineProps<{ modelValue: string; disabled?: boolean; workspace?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const slashMenuOpen = ref(false)
const slashQuery = ref('')
const slashIndex = ref(0)
const imageZoom = ref('')
const slashPosition = ref({ left: 12, top: 40 })
const imageMenuOpen = ref(false)
const imageUrl = ref('')
const imageUploadError = ref('')
const imagePosition = ref({ left: 12, top: 40 })
const imageFileInput = ref<HTMLInputElement | null>(null)
const calloutColors = ['#F0F0F0', '#E6E6E6', '#DCDCDC', '#F7F7F7', '#D2D2D2', '#EAEAEA', '#F1F1F1']
const imageWidths = ['25%', '50%', '75%', '100%']
const contextMenu = ref({ open: false, x: 0, y: 0, target: "text" })
const backgroundColors = [{ name: '浅灰', value: '#F1F1F1' }, { name: '中浅灰', value: '#E5E5E5' }, { name: '灰白', value: '#F8F8F8' }, { name: '中灰', value: '#D6D6D6' }, { name: '深浅灰', value: '#C9C9C9' }, { name: '无背景', value: '' }]
const textColors = [{ name: '默认', value: '#292929' }, { name: '深黑', value: '#111111' }, { name: '深灰', value: '#444444' }, { name: '中灰', value: '#707070' }, { name: '浅灰', value: '#929292' }]
const outlineItems = ref<Array<{ level: number; text: string; index: number }>>([])

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
        button.title = '拖动排序'
        button.setAttribute('aria-label', '块操作菜单')
        button.setAttribute('draggable', 'true')
        button.innerHTML = '<span>⠿</span>'
        return button
      },
    }),
  ],
  editorProps: {
    attributes: { class: 'content-editor-document', 'aria-label': '文档正文编辑器', spellcheck: 'true' },
    // Intercept slash-menu keys inside ProseMirror before its default Enter handler inserts a new paragraph.
    handleKeyDown: (_view, event) => {
      if (props.disabled || !slashMenuOpen.value) return false
      if (event.key === 'Escape') {
        event.preventDefault()
        slashMenuOpen.value = false
        return true
      }
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        if (!slashItems.value.length) return false
        event.preventDefault()
        moveSlashSelection(event.key === 'ArrowDown' ? 1 : -1)
        return true
      }
      if (event.key === 'Enter') {
        // Let the IME finish committing a Chinese candidate; the next Enter confirms the menu item.
        if (event.isComposing || event.keyCode === 229) return false
        if (!slashItems.value.length) return false
        event.preventDefault()
        runSlashItem()
        return true
      }
      return false
    },
  },
  onUpdate: ({ editor: current }) => {
    emit('update:modelValue', current.getMarkdown())
    updateSlashMenu(current)
    refreshOutline(current)
  },
  onSelectionUpdate: ({ editor: current }) => updateSlashMenu(current),
})

function refreshOutline(current: CoreEditor | null | undefined = editor.value) {
  if (!current) { outlineItems.value = []; return }
  const items: Array<{ level: number; text: string; index: number }> = []
  let headingIndex = 0
  current.state.doc.descendants((node) => {
    if (node.type.name === 'heading') {
      const index = headingIndex++
      const text = node.textContent.trim()
      if (text) items.push({ level: Number(node.attrs.level) || 1, text, index })
    }
  })
  outlineItems.value = items
}
function jumpToHeading(index: number) {
  const root = editor.value?.view.dom
  const headings = root?.querySelectorAll('h1,h2,h3')
  const heading = headings?.item(index) as HTMLElement | null | undefined
  heading?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  if (heading) editor.value?.commands.focus()
}
function runToolbarItem(item: (typeof toolbarItems.value)[number]) {
  if (props.disabled || !editor.value) return
  item.run()
  editor.value.commands.focus()
}
function handleEditorDoubleClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  const image = target?.closest('img') as HTMLImageElement | null
  if (!image || !editor.value?.view.dom.contains(image)) return
  event.preventDefault()
  imageZoom.value = image.currentSrc || image.src
}

function handleEditorClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  const anchor = target?.closest('a[href]') as HTMLAnchorElement | null
  if (!anchor || !editor.value?.view.dom.contains(anchor)) return
  const url = normalizeEditorLinkUrl(anchor.href)
  if (!url) return
  event.preventDefault()
  event.stopPropagation()
  window.open(url, '_blank', 'noopener,noreferrer')
}

function updateSlashMenu(current: CoreEditor) {
  const { $from } = current.state.selection
  const text = $from.parent.textContent
  const before = text.slice(0, $from.parentOffset)
  const match = before.match(/^\/([^\s/]*)$/)
  slashQuery.value = match?.[1] || ''
  slashMenuOpen.value = !props.disabled && Boolean(match) && $from.parentOffset === text.length
  slashIndex.value = 0
  if (slashMenuOpen.value) {
    try {
      const coords = current.view.coordsAtPos(current.state.selection.from)
      const canvas = current.view.dom.closest('.content-editor-canvas') as HTMLElement | null
      const rect = canvas?.getBoundingClientRect()
      if (rect) slashPosition.value = { left: Math.max(8, Math.min(coords.left - rect.left, rect.width - 300)), top: Math.max(8, coords.bottom - rect.top + 8) }
    } catch { /* selection can briefly be unavailable during editor transactions */ }
  }
}

const toolActions = {
  addImage: (current: CoreEditor) => {
    const coords = current.view.coordsAtPos(current.state.selection.from)
    const canvas = current.view.dom.closest('.content-editor-canvas') as HTMLElement | null
    const rect = canvas?.getBoundingClientRect()
    if (rect) imagePosition.value = { left: Math.max(8, Math.min(coords.left - rect.left, rect.width - 300)), top: Math.max(8, coords.bottom - rect.top + 8) }
    imageMenuOpen.value = true
    imageUrl.value = ''
    imageUploadError.value = ''
  },
  addLink: (current: CoreEditor) => {
    const value = window.prompt('输入链接地址', 'https://')
    const url = value ? normalizeEditorLinkUrl(value) : null
    if (url) current.chain().focus().insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url } }] }).run()
  },
}
const toolbarItems = computed(() => createContentEditorTools(editor.value, toolActions))
const slashItems = computed(() => toolbarItems.value.filter(item => !slashQuery.value || (item.label + ' ' + item.description + ' ' + item.keywords).toLocaleLowerCase().includes(slashQuery.value.toLocaleLowerCase())))

function runSlashItem(index = slashIndex.value) {
  const current = editor.value
  const item = slashItems.value[index]
  if (!current || !item || props.disabled) return
  // Use one command path so slash cleanup and block insertion share the same live selection.
  current.commands.focus()
  executeContentEditorTool(current, item)
  slashMenuOpen.value = false
  slashQuery.value = ''
  slashIndex.value = 0
}

function insertImageFromUrl() {
  const current = editor.value
  const url = normalizeEditorImageUrl(imageUrl.value)
  if (!current || !url) return
  current.chain().focus().setImage({ src: url, alt: '文档图片' }).run()
  imageMenuOpen.value = false
  imageUrl.value = ''
}
function onImageFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  const current = editor.value
  if (!file || !current || !file.type.startsWith('image/')) return
  imageUploadError.value = ''
  if (file.size > 8 * 1024 * 1024) { imageUploadError.value = '图片大小不能超过 8 MB。'; portalNotify.error(imageUploadError.value, '图片上传失败'); return }
  if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/avif'].includes(file.type)) { imageUploadError.value = '仅支持 PNG、JPEG、GIF、WebP 或 AVIF 图片。'; portalNotify.error(imageUploadError.value, '图片上传失败'); return }
  const reader = new FileReader()
  reader.onload = async () => {
    if (typeof reader.result !== 'string' || !editor.value) return
    try {
      const data = reader.result.split(',')[1] || ''
      const response = await fetch('/api/uploads', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify({ mime: file.type, data }) })
      const result = await response.json() as { url?: string; error?: string }
      if (!response.ok || !result.url) throw new Error(result.error || '图片上传失败。')
      editor.value.chain().focus().setImage({ src: result.url, alt: file.name }).run()
      imageMenuOpen.value = false
    } catch (error) {
      imageUploadError.value = error instanceof Error ? error.message : '图片上传失败，请重试。'
      portalNotify.error(imageUploadError.value, '图片上传失败')
    }
  }
  reader.readAsDataURL(file)
  ;(event.target as HTMLInputElement).value = ''
}
function resizeSelectedImage(width: string) {
  const current = editor.value
  if (!current || !current.isActive('image')) return
  const { from } = current.state.selection
  const node = current.state.doc.nodeAt(from)
  if (node?.type.name === 'image') current.chain().focus().updateAttributes('image', { displayWidth: width }).run()
}
function setCalloutColor(color: string) {
  editor.value?.chain().focus().updateAttributes('callout', { backgroundColor: color }).run()
}
function moveSlashSelection(delta: number) {
  if (!slashItems.value.length) { slashIndex.value = 0; return }
  slashIndex.value = (slashIndex.value + delta + slashItems.value.length) % slashItems.value.length
}
function onSlashSearchKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') { event.preventDefault(); moveSlashSelection(1) }
  else if (event.key === 'ArrowUp') { event.preventDefault(); moveSlashSelection(-1) }
  else if (event.key === 'Enter') { if (event.isComposing || event.keyCode === 229) return; event.preventDefault(); event.stopPropagation(); if (slashItems.value.length) runSlashItem() }
  else if (event.key === 'Escape') { event.preventDefault(); slashMenuOpen.value = false }
}

function addLinkToSelection() {
  const current = editor.value
  if (!current || props.disabled) return
  const value = window.prompt('输入链接地址', 'https://')
  const url = value ? normalizeEditorLinkUrl(value) : null
  if (url) current.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}

function onOutsideContextMenu(event: MouseEvent) {
  if (!(event.target as HTMLElement | null)?.closest('.content-editor-context-menu')) contextMenu.value.open = false
}
function openContextMenu(event: MouseEvent) {
  if (props.disabled || !editor.value) return
  const root = (event.currentTarget as HTMLElement).getBoundingClientRect()
  contextMenu.value = { open: true, target: 'text', x: Math.max(8, Math.min(event.clientX - root.left, Math.max(8, root.width - 320))), y: Math.max(8, Math.min(event.clientY - root.top, Math.max(8, root.height - 420))) }
}
function applyContextAction(action: string, color = '') {
  const current = editor.value
  if (!current || props.disabled) return
  const chain = current.chain().focus()
  if (action === 'bold') chain.toggleBold().run()
  else if (action === 'quote') chain.toggleBlockquote().run()
  else if (action === 'code') chain.toggleCode().run()
  else if (action === 'background' && color) chain.setHighlight({ color }).run()
  else if (action === 'background' && !color) chain.unsetHighlight().run()
  else if (action === 'text' && color) chain.setMark('textColor', { color }).run()
  else if (action === 'text' && !color) chain.unsetMark('textColor').run()
  else if (action === 'link') addLinkToSelection()
  else if (action === 'clear') chain.unsetAllMarks().run()
  contextMenu.value.open = false
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
    imageMenuOpen.value = false
  }
})
watch(editor, current => {
  refreshOutline(current)
}, { immediate: true })
onMounted(() => { document.addEventListener('click', onOutsideContextMenu) })
onBeforeUnmount(() => {
  document.removeEventListener('click', onOutsideContextMenu)
  editor.value?.destroy()
})
</script>

<template>
  <template v-if="workspace">
    <div class="content-workspace-grid">
      <aside class="content-outline-sidebar">
        <details class="workspace-fold workspace-outline-fold" open>
          <summary><span><strong>文档目录</strong></span><span class="workspace-fold-chevron bi bi-chevron-down" aria-hidden="true"></span></summary>
        <p v-if="!outlineItems.length" class="outline-empty">添加标题后，目录会自动生成。</p>
        <nav v-else class="content-outline-list" aria-label="文档目录">
          <button v-for="item in outlineItems" :key="item.index" type="button" :class="'outline-level-' + item.level" @click="jumpToHeading(item.index)"><span></span>{{ item.text }}</button>
        </nav>
        <div class="outline-sidebar-foot"><i class="bi bi-journal-text" aria-hidden="true"></i> 标题目录自动更新</div>
        </details>
      </aside>
      <section class="content-paper-stage">

        <div class="content-a4-paper">
          <div class="content-editor-hint"><span>正文编辑</span><span>输入 <kbd>/</kbd> 插入内容 · 选中文字可快速格式化</span></div>
          <div class="content-editor-canvas" @contextmenu.prevent="openContextMenu" @click.capture="handleEditorClick" @click="contextMenu.open = false" @dblclick="handleEditorDoubleClick">
            <EditorContent :editor="editor" />
            <div v-if="imageZoom" class="content-editor-image-zoom" role="dialog" aria-label="图片预览" @click.self="imageZoom = ''" @keydown.esc="imageZoom = ''"><button type="button" aria-label="关闭图片预览" @click="imageZoom = ''"><i class="bi bi-x-lg" aria-hidden="true"></i></button><img :src="imageZoom" alt="放大图片预览" /></div>
            <div v-if="imageMenuOpen && !disabled" class="content-editor-image-menu" :style="{ left: imagePosition.left + 'px', top: imagePosition.top + 'px' }" role="dialog" aria-label="插入图片"><strong>插入图片</strong><button type="button" @click="imageFileInput?.click()">上传本地文件</button><input ref="imageFileInput" name="imageFile" type="file" accept="image/*" hidden @change="onImageFileChange" /><div class="content-editor-image-url"><input class="form-control" v-model="imageUrl" name="imageUrl" type="url" placeholder="https:// 图片 URL" @keydown.enter.prevent="insertImageFromUrl" /><button type="button" @click="insertImageFromUrl">插入 URL</button></div><button type="button" class="content-editor-image-cancel" @click="imageMenuOpen = false">取消</button></div>
            <div v-if="contextMenu.open && !disabled" class="content-editor-context-menu" role="menu" aria-label="统一编辑快捷菜单" :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }" @mousedown.prevent @click.stop>
              <template v-if="editor?.isActive('image')"><div class="context-menu-heading">图片操作</div><div class="context-menu-actions"><button v-for="width in imageWidths" :key="width" type="button" role="menuitem" @click="resizeSelectedImage(width); contextMenu.open = false">宽度 {{ width }}</button></div></template>
              <template v-else-if="editor?.isActive('table')"><div class="context-menu-heading">表格操作</div><div class="context-menu-actions"><button type="button" role="menuitem" @click="editor?.chain().focus().addColumnAfter().run(); contextMenu.open = false"><i class="bi bi-plus-lg" aria-hidden="true"></i> 添加列</button><button type="button" role="menuitem" @click="editor?.chain().focus().deleteColumn().run(); contextMenu.open = false">− 删除列</button><button type="button" role="menuitem" @click="editor?.chain().focus().addRowAfter().run(); contextMenu.open = false"><i class="bi bi-plus-lg" aria-hidden="true"></i> 添加行</button><button type="button" role="menuitem" @click="editor?.chain().focus().deleteRow().run(); contextMenu.open = false">− 删除行</button><button type="button" role="menuitem" @click="editor?.chain().focus().deleteTable().run(); contextMenu.open = false">删除表格</button></div></template>
              <template v-else-if="editor?.isActive('callout')"><div class="context-menu-heading">提醒卡片背景</div><div class="context-color-row"><button v-for="color in calloutColors" :key="color" type="button" :title="color" :aria-label="'提醒背景 ' + color" :style="{ backgroundColor: color }" @click="setCalloutColor(color); contextMenu.open = false"></button></div></template>
              <div class="context-menu-heading">文本格式</div>
              <div class="context-menu-actions">
                <button type="button" role="menuitem" @click="applyContextAction('bold')"><b>B</b> 加粗</button>
                <button type="button" role="menuitem" @click="editor?.chain().focus().toggleItalic().run(); contextMenu.open = false"><i>I</i> 斜体</button>
                <button type="button" role="menuitem" @click="editor?.chain().focus().toggleUnderline().run(); contextMenu.open = false">U 下划线</button>
                <button type="button" role="menuitem" @click="editor?.chain().focus().toggleStrike().run(); contextMenu.open = false">S 删除线</button>
                <button type="button" role="menuitem" @click="editor?.chain().focus().toggleCode().run(); contextMenu.open = false">&lt;/&gt; 行内代码</button>
                <button type="button" role="menuitem" @click="applyContextAction('link')"><i class="bi bi-link-45deg" aria-hidden="true"></i> 添加链接</button>
                <button type="button" role="menuitem" @click="applyContextAction('clear')">Tx 清除格式</button>
              </div>
              <div class="context-menu-heading">背景色</div>
              <div class="context-color-row"><button v-for="color in backgroundColors" :key="color.name" type="button" :title="color.name" :aria-label="color.name" :class="{ 'color-none': !color.value }" :style="color.value ? { backgroundColor: color.value } : {}" @click="applyContextAction('background', color.value)"></button></div>
              <div class="context-menu-heading">文字颜色</div>
              <div class="context-color-row"><button v-for="color in textColors" :key="color.name" type="button" :title="color.name" :aria-label="color.name" :style="{ backgroundColor: color.value }" @click="applyContextAction('text', color.value)"></button></div>
              <div class="context-menu-heading">段落与内容块</div>
              <div class="context-menu-tool-list">
                <button v-for="item in toolbarItems" :key="item.label" type="button" role="menuitem" @click="runToolbarItem(item); contextMenu.open = false"><span class="context-tool-icon">{{ item.icon }}</span><span class="context-tool-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span></button>
              </div>
              <button type="button" class="context-menu-close" @click="contextMenu.open = false">关闭菜单</button>
            </div>
            <div v-if="slashMenuOpen && !disabled" class="content-editor-slash-menu" :style="{ left: slashPosition.left + 'px', top: slashPosition.top + 'px' }" role="menu" aria-label="插入内容块">
              <div class="content-editor-menu-caption">插入内容块 <small>↑↓ 选择 · Enter 确认</small></div>
              <input v-model="slashQuery" name="slashSearch" class="form-control content-editor-slash-search" type="search" placeholder="搜索工具，例如：待办清单" aria-label="搜索插入工具" @keydown="onSlashSearchKeydown" />
              <button v-for="(item, index) in slashItems" :key="item.label" type="button" role="menuitem" :class="{ selected: index === slashIndex }" @mousedown.prevent="runSlashItem(index)"><span class="content-editor-menu-icon">{{ item.icon }}</span><span><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span></button>
              <p v-if="!slashItems.length" class="content-editor-no-results">没有匹配的插入项</p>
            </div>
          </div>
        </div>
      </section>
      <aside class="content-tools-sidebar">
        <div class="workspace-sidebar-settings"><slot name="workspace-actions" /></div>
        <p class="context-menu-hint">在正文中单击右键，打开统一编辑菜单。</p>
      </aside>
    </div>
  </template>
  <template v-else>
    <div class="content-editor-shell" :class="{ 'is-disabled': disabled }">
        <div class="content-editor-hint">
          <span>文档编辑</span>
          <span>单击右键打开统一编辑菜单 · 输入 <kbd>/</kbd> 插入内容</span>
        </div>
        <div class="content-editor-canvas" @contextmenu.prevent="openContextMenu" @click.capture="handleEditorClick" @click="contextMenu.open = false" @dblclick="handleEditorDoubleClick">
          <EditorContent :editor="editor" />
          <div v-if="imageZoom" class="content-editor-image-zoom" role="dialog" aria-label="图片预览" @click.self="imageZoom = ''" @keydown.esc="imageZoom = ''"><button type="button" aria-label="关闭图片预览" @click="imageZoom = ''"><i class="bi bi-x-lg" aria-hidden="true"></i></button><img :src="imageZoom" alt="放大图片预览" /></div>
          <div v-if="imageMenuOpen && !disabled" class="content-editor-image-menu" :style="{ left: imagePosition.left + 'px', top: imagePosition.top + 'px' }" role="dialog" aria-label="插入图片"><strong>插入图片</strong><button type="button" @click="imageFileInput?.click()">上传本地文件</button><input ref="imageFileInput" name="imageFile" type="file" accept="image/*" hidden @change="onImageFileChange" /><div class="content-editor-image-url"><input v-model="imageUrl" name="imageUrl" type="url" placeholder="https:// 图片 URL" @keydown.enter.prevent="insertImageFromUrl" /><button type="button" @click="insertImageFromUrl">插入 URL</button></div><button type="button" class="content-editor-image-cancel" @click="imageMenuOpen = false">取消</button></div>
          <div v-if="contextMenu.open && !disabled" class="content-editor-context-menu" role="menu" aria-label="统一编辑快捷菜单" :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }" @mousedown.prevent @click.stop>
            <div class="context-menu-heading">文本格式</div>
            <div class="context-menu-actions">
              <button type="button" role="menuitem" @click="applyContextAction('bold')"><b>B</b> 加粗</button>
              <button type="button" role="menuitem" @click="editor?.chain().focus().toggleItalic().run(); contextMenu.open = false"><i>I</i> 斜体</button>
              <button type="button" role="menuitem" @click="editor?.chain().focus().toggleUnderline().run(); contextMenu.open = false">U 下划线</button>
              <button type="button" role="menuitem" @click="editor?.chain().focus().toggleStrike().run(); contextMenu.open = false">S 删除线</button>
              <button type="button" role="menuitem" @click="editor?.chain().focus().toggleCode().run(); contextMenu.open = false">&lt;/&gt; 行内代码</button>
              <button type="button" role="menuitem" @click="applyContextAction('link')"><i class="bi bi-link-45deg" aria-hidden="true"></i> 添加链接</button>
              <button type="button" role="menuitem" @click="applyContextAction('clear')">Tx 清除格式</button>
            </div>
            <div class="context-menu-heading">背景色</div>
            <div class="context-color-row"><button v-for="color in backgroundColors" :key="color.name" type="button" :title="color.name" :aria-label="color.name" :class="{ 'color-none': !color.value }" :style="color.value ? { backgroundColor: color.value } : {}" @click="applyContextAction('background', color.value)"></button></div>
            <div class="context-menu-heading">文字颜色</div>
            <div class="context-color-row"><button v-for="color in textColors" :key="color.name" type="button" :title="color.name" :aria-label="color.name" :style="{ backgroundColor: color.value }" @click="applyContextAction('text', color.value)"></button></div>
            <div class="context-menu-heading">段落与内容块</div>
            <div class="context-menu-tool-list">
              <button v-for="item in toolbarItems" :key="item.label" type="button" role="menuitem" @click="runToolbarItem(item); contextMenu.open = false"><span class="context-tool-icon">{{ item.icon }}</span><span class="context-tool-copy"><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span></button>
            </div>
            <button type="button" class="context-menu-close" @click="contextMenu.open = false">关闭菜单</button>
          </div>
          <div v-if="slashMenuOpen && !disabled" class="content-editor-slash-menu" :style="{ left: slashPosition.left + 'px', top: slashPosition.top + 'px' }" role="menu" aria-label="插入内容块">
            <div class="content-editor-menu-caption">插入内容块 <small>↑↓ 选择 · Enter 确认</small></div>
            <input v-model="slashQuery" name="slashSearch" class="content-editor-slash-search" type="search" placeholder="搜索工具，例如：待办清单" aria-label="搜索插入工具" @keydown="onSlashSearchKeydown" />
            <button v-for="(item, index) in slashItems" :key="item.label" type="button" role="menuitem" :class="{ selected: index === slashIndex }" @mousedown.prevent="runSlashItem(index)">
              <span class="content-editor-menu-icon">{{ item.icon }}</span><span><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span>
            </button>
            <p v-if="!slashItems.length" class="content-editor-no-results">没有匹配的插入项</p>
          </div>
        </div>
        <div class="content-editor-footer"><span>支持 Markdown 持久化</span><span>待办、表格、代码块、图片与对齐格式</span></div>
      </div>
  </template>
</template>
