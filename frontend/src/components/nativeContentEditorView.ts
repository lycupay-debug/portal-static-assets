/**
 * Responsibility: provide the full content-authoring workspace using Tiptap core and native DOM.
 * Dependencies: shared editor schema/tools, native drag handle, image upload helper, and heading projection.
 * Collaboration: App.vue owns the Markdown value and disabled state; this view emits Markdown changes and owns
 * transient menu/outline/zoom state. The Go API remains the authority for image upload and document persistence.
 * Boundaries: no Vue adapter, refs, watchers, or component lifecycle. Server-provided Markdown is parsed by Tiptap;
 * user-visible dynamic labels are assigned with textContent, while links are restricted to supported URL schemes.
 * Cleanup: remove all delegated/document listeners and destroy the ProseMirror editor on dispose.
 */
import { Editor } from '@tiptap/core'
import { createContentEditorExtensions, createContentEditorTools, executeContentEditorTool, normalizeEditorImageUrl, normalizeEditorLinkUrl, type ContentEditorTool } from '../utils/contentEditor'
import { createContentEditorDragHandle } from '../utils/contentEditorDragHandle'
import { collectContentEditorOutline } from '../utils/contentEditorOutline'
import { uploadAndInsertEditorImage } from '../utils/contentEditorImageUpload'

export type NativeContentEditorViewOptions = {
  host: HTMLElement
  markdown: string
  editable?: boolean
  onMarkdownChange?: (markdown: string) => void
}

const backgroundColors = [{ name: '浅灰', value: '#F1F1F1' }, { name: '中浅灰', value: '#E5E5E5' }, { name: '灰白', value: '#F8F8F8' }, { name: '中灰', value: '#D6D6D6' }, { name: '深浅灰', value: '#C9C9C9' }, { name: '无背景', value: '' }]
const textColors = [{ name: '默认', value: '' }, { name: '深黑', value: '#111111' }, { name: '深灰', value: '#444444' }, { name: '中灰', value: '#707070' }, { name: '浅灰', value: '#929292' }]
const calloutColors = ['#F0F0F0', '#E6E6E6', '#DCDCDC', '#F7F7F7', '#D2D2D2', '#EAEAEA', '#F1F1F1']
const imageWidths = ['25%', '50%', '75%', '100%']

function element<K extends keyof HTMLElementTagNameMap>(doc: Document, tag: K, className?: string, text?: string): HTMLElementTagNameMap[K] {
  const node = doc.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}
function actionButton(doc: Document, label: string, action: string, className = ''): HTMLButtonElement {
  const button = element(doc, 'button', className, label)
  button.type = 'button'
  button.dataset.action = action
  return button
}

export function mountNativeContentEditorView(options: NativeContentEditorViewOptions): {
  editor: Editor
  setMarkdown: (markdown: string) => void
  setEditable: (editable: boolean) => void
  dispose: () => void
} {
  const { host } = options
  const doc = host.ownerDocument
  let editable = options.editable !== false
  let slashOpen = false
  let slashQuery = ''
  let slashIndex = 0
  let imageMenuOpen = false
  let imageError = ''
  let imageUrl = ''
  let imageZoomUrl = ''
  let contextOpen = false
  let disposed = false
  let toolbarItems: ContentEditorTool[] = []

  const root = element(doc, 'div', 'content-workspace-native-root')
  const outlineAside = element(doc, 'aside', 'content-outline-sidebar')
  const outlineFold = element(doc, 'details', 'workspace-fold workspace-outline-fold')
  outlineFold.open = true
  const outlineSummary = element(doc, 'summary')
  outlineSummary.append(element(doc, 'strong', '', '文档目录'), element(doc, 'span', 'workspace-fold-chevron bi bi-chevron-down'))
  const outlineEmpty = element(doc, 'p', 'outline-empty', '添加标题后，目录会自动生成。')
  const outlineNav = element(doc, 'nav', 'content-outline-list')
  outlineNav.setAttribute('aria-label', '文档目录')
  const outlineFoot = element(doc, 'div', 'outline-sidebar-foot', '标题目录自动更新')
  outlineFoot.prepend(element(doc, 'i', 'bi bi-journal-text'))
  outlineFold.append(outlineSummary, outlineEmpty, outlineNav, outlineFoot)
  outlineAside.append(outlineFold)

  const stage = element(doc, 'section', 'content-paper-stage')
  const paper = element(doc, 'div', 'content-a4-paper')
  const hint = element(doc, 'div', 'content-editor-hint')
  hint.append(element(doc, 'span', '', '正文编辑'))
  const hintText = element(doc, 'span')
  hintText.append(doc.createTextNode('输入 '), element(doc, 'kbd', '', '/'), doc.createTextNode(' 插入内容 · 选中文字可快速格式化'))
  hint.append(hintText)
  const canvas = element(doc, 'div', 'content-editor-canvas')
  const editorMount = element(doc, 'div')
  const imageZoom = element(doc, 'div', 'content-editor-image-zoom')
  imageZoom.setAttribute('role', 'dialog'); imageZoom.setAttribute('aria-label', '图片预览'); imageZoom.hidden = true
  const zoomClose = actionButton(doc, '×', 'zoom-close'); zoomClose.setAttribute('aria-label', '关闭图片预览')
  const zoomImage = element(doc, 'img'); zoomImage.alt = '放大图片预览'
  imageZoom.append(zoomClose, zoomImage)

  const imageMenu = element(doc, 'div', 'content-editor-image-menu')
  imageMenu.setAttribute('role', 'dialog'); imageMenu.setAttribute('aria-label', '插入图片')
  imageMenu.append(element(doc, 'strong', '', '插入图片'))
  const imageErrorNode = element(doc, 'p', 'content-editor-image-error'); imageErrorNode.setAttribute('role', 'alert')
  const uploadButton = actionButton(doc, '上传本地文件', 'choose-image')
  const imageFileInput = element(doc, 'input'); imageFileInput.type = 'file'; imageFileInput.accept = 'image/*'; imageFileInput.hidden = true; imageFileInput.name = 'imageFile'
  const imageUrlRow = element(doc, 'div', 'content-editor-image-url')
  const imageUrlInput = element(doc, 'input'); imageUrlInput.type = 'url'; imageUrlInput.placeholder = 'https:// 图片 URL'; imageUrlInput.className = 'form-control'; imageUrlInput.name = 'imageUrl'
  const insertUrlButton = actionButton(doc, '插入 URL', 'insert-image-url')
  imageUrlRow.append(imageUrlInput, insertUrlButton)
  imageMenu.append(imageErrorNode, uploadButton, imageFileInput, imageUrlRow, actionButton(doc, '取消', 'cancel-image', 'content-editor-image-cancel'))

  const contextMenu = element(doc, 'div', 'content-editor-context-menu')
  contextMenu.setAttribute('role', 'menu'); contextMenu.setAttribute('aria-label', '统一编辑快捷菜单')
  const slashMenu = element(doc, 'div', 'content-editor-slash-menu')
  slashMenu.setAttribute('role', 'menu'); slashMenu.setAttribute('aria-label', '插入内容块')
  const slashCaption = element(doc, 'div', 'content-editor-menu-caption')
  slashCaption.append(doc.createTextNode('插入内容块 '), element(doc, 'small', '', '↑↓ 选择 · Enter 确认'))
  const slashInput = element(doc, 'input', 'form-control content-editor-slash-search'); slashInput.type = 'search'; slashInput.placeholder = '搜索工具，例如：待办清单'; slashInput.setAttribute('aria-label', '搜索插入工具')
  const slashList = element(doc, 'div')
  slashMenu.append(slashCaption, slashInput, slashList)

  canvas.append(editorMount, imageZoom, imageMenu, contextMenu, slashMenu)
  paper.append(hint, canvas)
  stage.append(paper)
  root.append(outlineAside, stage)
  host.classList.add('content-editor-native-main')
  host.replaceChildren(root)

  const toolActions = {
    addImage: (current: Editor) => {
      const coords = current.view.coordsAtPos(current.state.selection.from)
      const rect = canvas.getBoundingClientRect()
      imageMenu.style.left = `${Math.max(8, Math.min(coords.left - rect.left, rect.width - 300))}px`
      imageMenu.style.top = `${Math.max(8, coords.bottom - rect.top + 8)}px`
      imageUrl = ''; imageUrlInput.value = ''; imageError = ''; imageErrorNode.textContent = ''
      imageMenuOpen = true; renderMenus()
    },
    addLink: (current: Editor) => {
      const value = doc.defaultView?.prompt('输入链接地址', 'https://')
      const url = value ? normalizeEditorLinkUrl(value) : null
      if (url) current.chain().focus().insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url } }] }).run()
    },
  }

  function refreshOutline(current: Editor): void {
    const items = collectContentEditorOutline(current)
    outlineNav.replaceChildren()
    outlineEmpty.hidden = items.length > 0
    for (const item of items) {
      const button = element(doc, 'button', `outline-level-${item.level}`)
      button.type = 'button'
      button.append(element(doc, 'span'), doc.createTextNode(item.text))
      button.addEventListener('click', () => {
        const heading = current.view.dom.querySelectorAll('h1,h2,h3').item(item.index) as HTMLElement | null
        heading?.scrollIntoView({ behavior: 'smooth', block: 'center' })
        if (heading) current.commands.focus()
      })
      outlineNav.append(button)
    }
  }
  function getSlashItems(): ContentEditorTool[] {
    return toolbarItems.filter(item => !slashQuery || `${item.label} ${item.description} ${item.keywords}`.toLocaleLowerCase().includes(slashQuery.toLocaleLowerCase()))
  }
  function closeSlash(): void { slashOpen = false; slashQuery = ''; slashIndex = 0; renderMenus() }
  function runSlashItem(index = slashIndex): void {
    const item = getSlashItems()[index]
    if (!item || !editable) return
    editor.commands.focus()
    executeContentEditorTool(editor, item)
    closeSlash()
  }
  function updateSlashMenu(current: Editor): void {
    const { $from } = current.state.selection
    const text = $from.parent.textContent
    const before = text.slice(0, $from.parentOffset)
    const match = before.match(/^\/([^\s/]*)$/)
    slashQuery = match?.[1] || ''
    slashOpen = editable && Boolean(match) && $from.parentOffset === text.length
    slashIndex = 0
    if (slashOpen) {
      try {
        const coords = current.view.coordsAtPos(current.state.selection.from)
        const rect = canvas.getBoundingClientRect()
        slashMenu.style.left = `${Math.max(8, Math.min(coords.left - rect.left, rect.width - 300))}px`
        slashMenu.style.top = `${Math.max(8, coords.bottom - rect.top + 8)}px`
      } catch { /* Selection may be unavailable during an editor transaction. */ }
    }
    renderMenus()
  }
  function moveSlashSelection(delta: number): void {
    const items = getSlashItems()
    slashIndex = items.length ? (slashIndex + delta + items.length) % items.length : 0
    renderSlashList()
  }
  function renderSlashList(): void {
    slashList.replaceChildren()
    const items = getSlashItems()
    items.forEach((item, index) => {
      const button = element(doc, 'button', index === slashIndex ? 'selected' : '')
      button.type = 'button'
      button.setAttribute('role', 'menuitem')
      button.append(element(doc, 'span', 'content-editor-menu-icon', item.icon))
      const copy = element(doc, 'span')
      copy.append(element(doc, 'strong', '', item.label), element(doc, 'small', '', item.description))
      button.append(copy)
      button.addEventListener('mousedown', event => { event.preventDefault(); runSlashItem(index) })
      slashList.append(button)
    })
    if (!items.length) slashList.append(element(doc, 'p', 'content-editor-no-results', '没有匹配的插入项'))
  }
  function renderContextMenu(): void {
    contextMenu.replaceChildren()
    const addHeading = (label: string) => contextMenu.append(element(doc, 'div', 'context-menu-heading', label))
    const actions = (items: Array<[string,string]>) => {
      const group = element(doc, 'div', 'context-menu-actions')
      for (const [label, action] of items) group.append(actionButton(doc, label, action))
      contextMenu.append(group)
    }
    if (editor.isActive('image')) {
      addHeading('图片操作')
      const imageActions=element(doc,'div','context-menu-actions')
      imageWidths.forEach(width=>{const b=actionButton(doc,`宽度 ${width}`,'image-width');b.dataset.value=width;imageActions.append(b)})
      contextMenu.append(imageActions)
    } else if (editor.isActive('table')) {
      addHeading('表格操作')
      actions([['添加列','table-add-column'],['删除列','table-delete-column'],['添加行','table-add-row'],['删除行','table-delete-row'],['删除表格','table-delete']])
    } else if (editor.isActive('callout')) {
      addHeading('提醒卡片背景')
      const callout=element(doc,'div','context-color-row')
      calloutColors.forEach(color=>{const b=actionButton(doc,'','callout-color');b.title=color;b.setAttribute('aria-label',`提醒背景 ${color}`);b.style.backgroundColor=color;b.dataset.value=color;callout.append(b)})
      contextMenu.append(callout)
    }
    addHeading('文本格式')
    actions([['加粗','bold'],['斜体','italic'],['下划线','underline'],['删除线','strike'],['行内代码','code'],['引用','quote'],['添加链接','link'],['清除格式','clear']])
    addHeading('背景色')
    const backgrounds = element(doc, 'div', 'context-color-row')
    backgroundColors.forEach(color => { const b=actionButton(doc,'', 'background'); b.title=color.name;b.setAttribute('aria-label',color.name);if(color.value)b.style.backgroundColor=color.value;else b.classList.add('color-none');b.dataset.value=color.value;backgrounds.append(b) })
    contextMenu.append(backgrounds)
    addHeading('文字颜色')
    const colors=element(doc,'div','context-color-row')
    textColors.forEach(color=>{const b=actionButton(doc,'','text-color');b.title=color.name;b.setAttribute('aria-label',color.name);b.style.backgroundColor=color.value;b.dataset.value=color.value;colors.append(b)})
    contextMenu.append(colors)
    addHeading('段落与内容块')
    const list=element(doc,'div','context-menu-tool-list')
    toolbarItems.forEach((item,index)=>{const b=actionButton(doc,'','tool');b.setAttribute('role','menuitem');b.dataset.index=String(index);b.append(element(doc,'span','context-tool-icon',item.icon));const copy=element(doc,'span','context-tool-copy');copy.append(element(doc,'strong','',item.label),element(doc,'small','',item.description));b.append(copy);list.append(b)})
    contextMenu.append(list,actionButton(doc,'关闭菜单','close-context','context-menu-close'))
  }
  function renderMenus(): void {
    imageMenu.hidden = !imageMenuOpen || !editable
    imageErrorNode.hidden = !imageError
    imageErrorNode.textContent = imageError
    slashMenu.hidden = !slashOpen || !editable
    if (doc.activeElement !== slashInput) slashInput.value = slashQuery
    renderSlashList()
    contextMenu.hidden = !contextOpen || !editable
  }
  function insertImageUrl(): void {
    const url=normalizeEditorImageUrl(imageUrlInput.value)
    if(!url)return
    editor.chain().focus().setImage({src:url,alt:'文档图片'}).run()
    imageMenuOpen=false;imageUrlInput.value='';renderMenus()
  }
  function applyContextAction(action: string, value = ''): void {
    if (!editable) return
    const chain=editor.chain().focus()
    if(action==='bold')chain.toggleBold().run()
    else if(action==='italic')chain.toggleItalic().run()
    else if(action==='underline')chain.toggleUnderline().run()
    else if(action==='strike')chain.toggleStrike().run()
    else if(action==='code')chain.toggleCode().run()
    else if(action==='quote')chain.toggleBlockquote().run()
    else if(action==='link'){const value=doc.defaultView?.prompt('输入链接地址','https://');const url=value?normalizeEditorLinkUrl(value):null;if(url)chain.extendMarkRange('link').setLink({href:url}).run()}
    else if(action==='clear')chain.unsetAllMarks().run()
    else if(action==='background'&&value)chain.setHighlight({color:value}).run()
    else if(action==='background')chain.unsetHighlight().run()
    else if(action==='text-color'&&value)chain.setMark('textColor',{color:value}).run()
    else if(action==='text-color')chain.unsetMark('textColor').run()
    contextOpen=false;renderMenus()
  }
  function openContextMenu(event: MouseEvent): void {
    if(!editable)return
    const rect=canvas.getBoundingClientRect()
    contextMenu.style.left=`${Math.max(8,Math.min(event.clientX-rect.left,Math.max(8,rect.width-320)))}px`
    contextMenu.style.top=`${Math.max(8,Math.min(event.clientY-rect.top,Math.max(8,rect.height-420)))}px`
    renderContextMenu()
    contextOpen=true;renderMenus()
  }
  function onEditorClick(event: MouseEvent): void {
    const target=event.target instanceof Element?event.target:null
    const anchor=target?.closest('a[href]') as HTMLAnchorElement|null
    if(!anchor||!editor.view.dom.contains(anchor))return
    const url=normalizeEditorLinkUrl(anchor.href)
    if(!url)return
    event.preventDefault();event.stopPropagation();doc.defaultView?.open(url,'_blank','noopener,noreferrer')
  }
  function onEditorDoubleClick(event: MouseEvent): void {
    const target=event.target instanceof Element?event.target:null
    const image=target?.closest('img') as HTMLImageElement|null
    if(!image||!editor.view.dom.contains(image))return
    event.preventDefault();imageZoomUrl=image.currentSrc||image.src;zoomImage.src=imageZoomUrl;imageZoom.hidden=false
  }
  function onCanvasContextMenu(event: MouseEvent): void { event.preventDefault();openContextMenu(event) }
  function onCanvasClick(event: MouseEvent): void { if(!(event.target instanceof Element&&event.target.closest('.content-editor-context-menu'))) {contextOpen=false;renderMenus()} }
  function onOutsideClick(event: MouseEvent): void {
    if(!(event.target instanceof Element&&event.target.closest('.content-editor-context-menu'))) {contextOpen=false;renderMenus()}
  }
  function onSlashInput(): void { slashQuery=slashInput.value;slashIndex=0;renderSlashList() }
  function onSlashKeydown(event: KeyboardEvent): void {
    if(event.key==='ArrowDown'){event.preventDefault();moveSlashSelection(1)}
    else if(event.key==='ArrowUp'){event.preventDefault();moveSlashSelection(-1)}
    else if(event.key==='Enter'){if(event.isComposing||event.keyCode===229)return;event.preventDefault();event.stopPropagation();if(getSlashItems().length)runSlashItem()}
    else if(event.key==='Escape'){event.preventDefault();closeSlash()}
  }
  function onEditorKeyDown(_view: any, event: KeyboardEvent): boolean {
    if(!editable||!slashOpen)return false
    if(event.key==='Escape'){event.preventDefault();closeSlash();return true}
    if(event.key==='ArrowDown'||event.key==='ArrowUp'){if(!getSlashItems().length)return false;event.preventDefault();moveSlashSelection(event.key==='ArrowDown'?1:-1);return true}
    if(event.key==='Enter'){if(event.isComposing||event.keyCode===229||!getSlashItems().length)return false;event.preventDefault();runSlashItem();return true}
    return false
  }

  const editor = new Editor({
    element: editorMount,
    content: options.markdown || '',
    contentType: 'markdown',
    editable,
    extensions: [...createContentEditorExtensions(), createContentEditorDragHandle()],
    editorProps: { attributes: { class: 'content-editor-document', 'aria-label': '文档正文编辑器', spellcheck: 'true' }, handleKeyDown: onEditorKeyDown },
    onUpdate: ({ editor: current }) => { options.onMarkdownChange?.(current.getMarkdown());updateSlashMenu(current);refreshOutline(current) },
    onSelectionUpdate: ({ editor: current }) => updateSlashMenu(current),
  })
  toolbarItems=createContentEditorTools(editor,{addImage:toolActions.addImage,addLink:toolActions.addLink})
  refreshOutline(editor)
  renderContextMenu();renderMenus()

  const onClick = (event: MouseEvent): void => {
    const target=event.target instanceof Element?event.target.closest<HTMLElement>('[data-action]'):null
    if(!target)return
    const action=target.dataset.action
    if(action==='zoom-close'){imageZoomUrl='';imageZoom.hidden=true;zoomImage.removeAttribute('src')}
    else if(action==='choose-image')imageFileInput.click()
    else if(action==='insert-image-url')insertImageUrl()
    else if(action==='cancel-image'){imageMenuOpen=false;renderMenus()}
    else if(action==='close-context'){contextOpen=false;renderMenus()}
    else if(action==='slash-item')runSlashItem(Number(target.dataset.index))
    else if(action==='image-width'){const width=target.dataset.value;const pos=editor.state.selection.from;const node=editor.state.doc.nodeAt(pos);if(node?.type.name==='image'&&width)editor.chain().focus().updateAttributes('image',{displayWidth:width}).run();contextOpen=false;renderMenus()}
    else if(action==='callout-color'){editor.chain().focus().updateAttributes('callout',{backgroundColor:target.dataset.value}).run();contextOpen=false;renderMenus()}
    else if(action==='table-add-column'){editor.chain().focus().addColumnAfter().run();contextOpen=false;renderMenus()}
    else if(action==='table-delete-column'){editor.chain().focus().deleteColumn().run();contextOpen=false;renderMenus()}
    else if(action==='table-add-row'){editor.chain().focus().addRowAfter().run();contextOpen=false;renderMenus()}
    else if(action==='table-delete-row'){editor.chain().focus().deleteRow().run();contextOpen=false;renderMenus()}
    else if(action==='table-delete'){editor.chain().focus().deleteTable().run();contextOpen=false;renderMenus()}
    else if(action==='tool'){const item=toolbarItems[Number(target.dataset.index)];if(item){item.run();editor.commands.focus();contextOpen=false;renderMenus()}}
    else if(['bold','italic','underline','strike','code','quote','link','clear','background','text-color'].includes(action||''))applyContextAction(action||'',target.dataset.value||'')
  }
  const onImageFileChange = async (): Promise<void> => {
    const file=imageFileInput.files?.[0]
    imageFileInput.value=''
    if(!file)return
    const success=await uploadAndInsertEditorImage(file,editor,message=>{imageError=message;renderMenus()})
    if(success){imageMenuOpen=false;renderMenus()}
  }
  const onImageUrlKeydown=(event:KeyboardEvent):void=>{if(event.key==='Enter'){event.preventDefault();insertImageUrl()}}
  const onDocumentKeydown=(event:KeyboardEvent):void=>{if(event.key==='Escape'){if(imageZoomUrl){imageZoomUrl='';imageZoom.hidden=true;zoomImage.removeAttribute('src')}else if(imageMenuOpen){imageMenuOpen=false;renderMenus()}else if(contextOpen){contextOpen=false;renderMenus()}else if(slashOpen)closeSlash()}}

  canvas.addEventListener('contextmenu',onCanvasContextMenu)
  canvas.addEventListener('click',onCanvasClick)
  canvas.addEventListener('click',onEditorClick,true)
  canvas.addEventListener('dblclick',onEditorDoubleClick)
  canvas.addEventListener('click',onClick)
  doc.addEventListener('click',onOutsideClick)
  doc.addEventListener('keydown',onDocumentKeydown)
  slashInput.addEventListener('input',onSlashInput)
  slashInput.addEventListener('keydown',onSlashKeydown)
  imageFileInput.addEventListener('change',()=>{void onImageFileChange()})
  imageUrlInput.addEventListener('keydown',onImageUrlKeydown)
  imageZoom.addEventListener('click',event=>{if(event.target===imageZoom){imageZoomUrl='';imageZoom.hidden=true;zoomImage.removeAttribute('src')}})

  return {
    editor,
    setMarkdown: markdown => { if(editor.getMarkdown()!==(markdown||'')){editor.commands.setContent(markdown||'',{contentType:'markdown',emitUpdate:false});refreshOutline(editor)} },
    setEditable: value => { editable=value;editor.setEditable(value);if(!value){slashOpen=false;imageMenuOpen=false;contextOpen=false}renderMenus() },
    dispose: () => {
      if(disposed)return
      disposed=true
      canvas.removeEventListener('contextmenu',onCanvasContextMenu)
      canvas.removeEventListener('click',onCanvasClick)
      canvas.removeEventListener('click',onEditorClick,true)
      canvas.removeEventListener('dblclick',onEditorDoubleClick)
      canvas.removeEventListener('click',onClick)
      doc.removeEventListener('click',onOutsideClick)
      doc.removeEventListener('keydown',onDocumentKeydown)
      slashInput.removeEventListener('input',onSlashInput)
      slashInput.removeEventListener('keydown',onSlashKeydown)
      imageUrlInput.removeEventListener('keydown',onImageUrlKeydown)
      editor.destroy()
      host.replaceChildren()
      host.classList.remove('content-editor-native-main')
    },
  }
}
