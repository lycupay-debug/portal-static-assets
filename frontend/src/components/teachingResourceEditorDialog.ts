/**
 * Responsibility: render the teaching-resource create/edit dialog, validate files, and upload resources.
 * Dependencies: generated ResourceWrite/TeachingResource contracts, the native virtualized select,
 * FileReader/XMLHttpRequest browser APIs, and the on-demand resource-validation utility.
 * Collaboration: App.vue owns whether the dialog is open, receives busy/saved/completed/close callbacks,
 * and reloads the resource list after successful creation or update.
 * Boundaries: preserves existing /api/resources POST/PATCH contracts and sequential batch semantics;
 * backend validation and authorization remain authoritative. Dynamic text is always written as text.
 * Side effects: reads selected files locally, sends JSON upload requests only after form submission,
 * and does not cancel an in-flight request if the parent unmounts the dialog.
 */
import type { ResourceWrite, TeachingResource } from '../api/generated'
import { mountVirtualizedSelect, type VirtualizedSelectOption } from './virtualizedSelect'
import { formatFileSize } from '../utils/resourceFiles'

type Options = {
  resource: TeachingResource | null
  resourceTypeOptions: string[]
  resourceGradeOptions: string[]
  onClose: () => void
  onBusyChange: (busy: boolean) => void
  onSaved: () => void
  onCompleted: () => void
}
type DialogState = {
  title: string
  description: string
  file: File | null
  files: File[]
  batchMode: boolean
  type: string
  grade: string
  isPublic: boolean
  url: string
  progress: number
  uploadDone: boolean
  uploadResult: string
  error: string
  busy: boolean
}
type SelectController = ReturnType<typeof mountVirtualizedSelect>

export type TeachingResourceEditorController = { dispose: () => void }

function el<K extends keyof HTMLElementTagNameMap>(document: Document, tag: K, className = '', text = ''): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag)
  if (className) element.className = className
  if (text) element.textContent = text
  return element
}

function fieldLabel(document: Document, text: string, input: HTMLElement, id: string, wide = false): HTMLLabelElement {
  const label = el(document, 'label', wide ? 'task-wide' : '')
  label.htmlFor = id
  label.append(el(document, 'span', 'share-dialog-label', text), input)
  return label
}

export function mountTeachingResourceEditorDialog(host: HTMLElement, options: Options): TeachingResourceEditorController {
  const document = host.ownerDocument
  const window = document.defaultView!
  const resource = options.resource
  const editingId = resource?.id ?? null
  const state: DialogState = {
    title: resource?.title ?? '',
    description: resource?.description ?? '',
    file: null,
    files: [],
    batchMode: false,
    type: resource?.type || '听力材料',
    grade: resource?.grade || '',
    isPublic: resource?.isPublic === true,
    url: resource?.url || '',
    progress: 0,
    uploadDone: false,
    uploadResult: '',
    error: '',
    busy: false,
  }
  const backdrop = el(document, 'div', 'share-dialog-backdrop resource-upload-backdrop')
  let disposed = false
  let typeSelect: SelectController | null = null
  let gradeSelect: SelectController | null = null
  let form: HTMLFormElement | null = null
  let errorNode: HTMLParagraphElement | null = null
  let selectedFileNode: HTMLParagraphElement | null = null
  let progressTrack: HTMLDivElement | null = null
  let progressBar: HTMLDivElement | null = null
  let progressText: HTMLSpanElement | null = null
  let progressNumber: HTMLElement | null = null
  let liveProgress: HTMLElement | null = null
  let submitButton: HTMLButtonElement | null = null
  let closeButton: HTMLButtonElement | null = null
  let cancelButton: HTMLButtonElement | null = null
  let batchInput: HTMLInputElement | null = null
  let fileInput: HTMLInputElement | null = null

  host.replaceChildren(backdrop)

  const isLocked = () => state.busy || state.uploadDone
  const typeOptions: VirtualizedSelectOption[] = options.resourceTypeOptions.map(value => ({ label: value, value }))
  const gradeOptions: VirtualizedSelectOption[] = options.resourceGradeOptions.map(value => ({ label: value, value: value === '不限定年级' ? '' : value }))

  function disposeSelects() {
    typeSelect?.dispose()
    gradeSelect?.dispose()
    typeSelect = null
    gradeSelect = null
  }

  function setError(message: string) {
    state.error = message
    if (disposed || !form) return
    if (message) {
      if (!errorNode) {
        errorNode = el(document, 'p', 'resource-error')
        const actions = form.querySelector('.resource-form-actions')
        form.insertBefore(errorNode, actions || null)
      }
      errorNode.textContent = message
    } else {
      errorNode?.remove()
      errorNode = null
    }
  }

  function setBusy(busy: boolean) {
    state.busy = busy
    if (disposed) return
    options.onBusyChange(busy)
    const locked = isLocked()
    form?.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLButtonElement>('input, textarea, button').forEach(control => {
      control.disabled = locked
    })
    typeSelect?.update({ disabled: locked })
    gradeSelect?.update({ disabled: locked })
    if (submitButton) submitButton.disabled = locked
    if (cancelButton) cancelButton.disabled = locked
    if (closeButton) { closeButton.hidden = locked; closeButton.disabled = locked }
    if (liveProgress) liveProgress.hidden = !busy
  }

  function updateProgress() {
    if (disposed) return
    if (progressTrack) progressTrack.setAttribute('aria-valuenow', String(state.progress))
    if (progressBar) progressBar.style.width = `${state.progress}%`
    if (progressNumber) progressNumber.textContent = `${state.progress}%`
    if (progressText) progressText.textContent = state.progress >= 100 ? '正在等待服务器确认保存' : '正在上传并保存，请勿关闭窗口'
  }

  function updateSelectedFile() {
    if (!selectedFileNode) return
    if (state.batchMode && !editingId && state.files.length) {
      selectedFileNode.textContent = `已选择 ${state.files.length} 个文件：${state.files.map(file => file.name).join('、')}`
      selectedFileNode.hidden = false
    } else if (state.file) {
      selectedFileNode.textContent = `${state.file.name} · ${formatFileSize(state.file.size)}${editingId ? ' · 上传后替换原文件' : ''}`
      selectedFileNode.hidden = false
    } else {
      selectedFileNode.textContent = ''
      selectedFileNode.hidden = true
    }
  }

  function resetUpload() {
    state.progress = 0
    state.uploadDone = false
    state.uploadResult = ''
    updateProgress()
  }

  function makeProgress(): HTMLElement {
    const wrapper = el(document, 'div', 'resource-upload-live-progress')
    progressTrack = el(document, 'div', 'resource-upload-progress-track') as HTMLDivElement
    progressTrack.setAttribute('role', 'progressbar')
    progressTrack.setAttribute('aria-valuemin', '0')
    progressTrack.setAttribute('aria-valuemax', '100')
    progressBar = el(document, 'div', 'resource-upload-progress-bar') as HTMLDivElement
    progressTrack.append(progressBar)
    const label = el(document, 'div', 'resource-upload-progress-label')
    progressText = el(document, 'span') as HTMLSpanElement
    progressNumber = el(document, 'strong')
    label.append(progressText, progressNumber)
    wrapper.append(progressTrack, label)
    updateProgress()
    return wrapper
  }

  function renderCompletion() {
    disposeSelects()
    form = null
    closeButton = null
    backdrop.replaceChildren()
    const dialog = el(document, 'section', 'share-dialog resource-create-dialog')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'resource-create-title')
    const completionTitle = el(document, 'h2', '', editingId ? '编辑教学资源' : '创建教学资源')
    completionTitle.id = 'resource-create-title'
    dialog.append(
      el(document, 'span', 'share-dialog-eyebrow', 'TEACHING LIBRARY'),
      completionTitle,
      el(document, 'p', '', state.uploadResult),
    )
    const completion = el(document, 'div', 'resource-upload-completion')
    progressTrack = el(document, 'div', 'resource-upload-progress-track') as HTMLDivElement
    progressTrack.setAttribute('role', 'progressbar')
    progressTrack.setAttribute('aria-valuemin', '0')
    progressTrack.setAttribute('aria-valuemax', '100')
    progressBar = el(document, 'div', 'resource-upload-progress-bar') as HTMLDivElement
    progressTrack.append(progressBar)
    const label = el(document, 'div', 'resource-upload-progress-label')
    progressText = el(document, 'span', '', state.progress >= 100 ? '处理完成' : '正在处理') as HTMLSpanElement
    progressNumber = el(document, 'strong', '', `${state.progress}%`)
    label.append(progressText, progressNumber)
    const finish = el(document, 'button', 'btn content-primary-button', '完成')
    finish.type = 'button'
    finish.disabled = state.progress < 100 || !state.uploadDone
    finish.addEventListener('click', finishUpload)
    completion.append(progressTrack, label, finish)
    dialog.append(completion)
    backdrop.append(dialog)
    updateProgress()
  }

  function renderForm() {
    disposeSelects()
    backdrop.replaceChildren()
    const dialog = el(document, 'section', 'share-dialog resource-create-dialog')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'resource-create-title')
    const close = el(document, 'button', 'share-dialog-close')
    closeButton = close
    close.type = 'button'
    close.setAttribute('aria-label', '关闭')
    const closeIcon = el(document, 'i', 'bi bi-x-lg')
    closeIcon.setAttribute('aria-hidden', 'true')
    close.append(closeIcon)
    close.addEventListener('click', options.onClose)
    close.hidden = isLocked()
    dialog.append(close, el(document, 'span', 'share-dialog-eyebrow', 'TEACHING LIBRARY'))
    const title = el(document, 'h2', '', editingId ? '编辑教学资源' : '创建教学资源')
    title.id = 'resource-create-title'
    dialog.append(title, el(document, 'p', '', editingId ? '可更新资源名称、类型、说明、分享链接；选择新文件才会替换原文件。' : '文件上传与 URL 分享二选一，也可以同时填写。'))

    form = el(document, 'form', 'resource-create-form')
    const titleInput = el(document, 'input', 'form-control share-dialog-url')
    titleInput.id = 'resource-title'
    titleInput.name = 'resourceTitle'
    titleInput.maxLength = 200
    titleInput.required = true
    titleInput.value = state.title
    titleInput.dataset.field = 'title'
    form.append(fieldLabel(document, '资源名称', titleInput, titleInput.id))

    const typeHost = el(document, 'div')
    form.append(el(document, 'label', 'share-dialog-label', '类型'), typeHost)
    const gradeHost = el(document, 'div')
    form.append(el(document, 'label', 'share-dialog-label', '年级'), gradeHost)

    const publicLabel = el(document, 'label', 'resource-public-toggle')
    const publicInput = el(document, 'input')
    publicInput.type = 'checkbox'
    publicInput.checked = state.isPublic
    publicInput.dataset.field = 'isPublic'
    const publicCopy = el(document, 'span')
    publicCopy.append(el(document, 'strong', '', '公开展示在教育资源页'), el(document, 'small', '', '公开后访客可查看并访问资源。请确保你有权分享所提供的内容。'))
    publicLabel.append(publicInput, publicCopy)
    form.append(publicLabel)

    const description = el(document, 'textarea', 'form-control share-dialog-url resource-description-input')
    description.id = 'resource-description'
    description.name = 'resourceDescription'
    description.maxLength = 2000
    description.placeholder = '资源内容、课程、章节或使用说明（选填）'
    description.value = state.description
    description.dataset.field = 'description'
    form.append(fieldLabel(document, '资源说明', description, description.id))

    if (!editingId) {
      const batchLabel = el(document, 'label', 'resource-batch-toggle')
      batchInput = el(document, 'input')
      batchInput.type = 'checkbox'
      batchInput.checked = state.batchMode
      batchInput.dataset.field = 'batchMode'
      batchLabel.append(batchInput, el(document, 'span', '', '批量上传（每个文件单独创建一条资源）'))
      form.append(batchLabel)
    } else batchInput = null

    const fileLabel = el(document, 'label', 'share-dialog-label', state.batchMode && !editingId ? '上传文件（可多选）' : '上传文件（选填）')
    fileLabel.htmlFor = 'resource-file'
    fileInput = el(document, 'input', 'form-control resource-file-input')
    fileInput.id = 'resource-file'
    fileInput.type = 'file'
    fileInput.multiple = state.batchMode && !editingId
    fileInput.accept = '.ppt,.pptx,.mp3,.pdf,.doc,.docx,.txt'
    form.append(fileLabel, fileInput)
    form.append(el(document, 'p', 'resource-file-hint', '支持 PPT / PPTX、MP3、PDF、DOC / DOCX、TXT；单个文件不超过 25 MB。批量模式下，每个文件都会创建一条独立资源，其他填写信息共用。也可复制文件后在表单中按 Ctrl+V 粘贴（取决于浏览器支持）。不上传文件时可直接使用 URL。'))
    selectedFileNode = el(document, 'p', 'resource-selected-file')
    selectedFileNode.hidden = true
    form.append(selectedFileNode)

    const urlInput = el(document, 'input', 'form-control share-dialog-url')
    urlInput.id = 'resource-url'
    urlInput.name = 'resourceUrl'
    urlInput.type = 'url'
    urlInput.placeholder = '粘贴可分享的 https:// 链接'
    urlInput.value = state.url
    urlInput.dataset.field = 'url'
    form.append(fieldLabel(document, 'URL 链接（选填）', urlInput, urlInput.id))

    liveProgress = makeProgress()
    liveProgress.hidden = true
    form.append(liveProgress)
    errorNode = null
    const actions = el(document, 'div', 'resource-form-actions')
    cancelButton = el(document, 'button', 'btn content-secondary-button', '取消')
    cancelButton.type = 'button'
    cancelButton.disabled = isLocked()
    cancelButton.addEventListener('click', options.onClose)
    submitButton = el(document, 'button', 'btn content-primary-button', isLocked() ? '正在保存…' : editingId ? '保存并处理' : '保存资源')
    submitButton.type = 'submit'
    submitButton.disabled = isLocked()
    actions.append(cancelButton, submitButton)
    form.append(actions)
    backdrop.append(dialog)
    dialog.append(form)

    typeSelect = mountVirtualizedSelect(typeHost, {
      label: '教学资源类型', options: typeOptions, value: state.type,
      disabled: isLocked(), onChange: value => { state.type = value },
    })
    gradeSelect = mountVirtualizedSelect(gradeHost, {
      label: '年级', placeholder: '选择适用年级', options: gradeOptions, value: state.grade,
      disabled: isLocked(), onChange: value => { state.grade = value },
    })
    updateSelectedFile()
    if (state.error) setError(state.error)
    form.addEventListener('input', onInput)
    form.addEventListener('change', onChange)
    fileInput.addEventListener('change', onFileChange)
    form.addEventListener('submit', onSubmit)
    form.addEventListener('paste', onPaste)
  }

  function onInput(event: Event) {
    const target = event.target as HTMLInputElement | HTMLTextAreaElement | null
    const field = target?.dataset.field
    if (!target || !field) return
    if (field === 'title') state.title = target.value
    else if (field === 'description') state.description = target.value
    else if (field === 'url') state.url = target.value
  }

  function onChange(event: Event) {
    const target = event.target as HTMLInputElement | null
    const field = target?.dataset.field
    if (!target || !field) return
    if (field === 'isPublic') state.isPublic = target.checked
    else if (field === 'batchMode') {
      state.batchMode = target.checked
      state.file = null
      state.files = []
      state.error = ''
      renderForm()
    }
  }

  function onFileChange(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files || [])
    if (state.batchMode && !editingId) {
      state.files = files
      state.file = files[0] || null
    } else {
      state.file = files[0] || null
      state.files = []
    }
    setError('')
    updateSelectedFile()
  }

  async function onPaste(event: ClipboardEvent) {
    if (isLocked()) return
    const clipboard = event.clipboardData
    if (!clipboard) return
    const files = Array.from(clipboard.files || [])
    if (!files.length) {
      for (const item of Array.from(clipboard.items || [])) {
        if (item.kind === 'file') {
          const file = item.getAsFile()
          if (file) files.push(file)
        }
      }
    }
    if (!files.length) return
    event.preventDefault()
    try {
      const { filterPastedResourceFiles } = await import('../utils/teachingResourceValidation')
      if (disposed) return
      const { accepted, errors } = filterPastedResourceFiles(files)
      if (state.batchMode && !editingId) {
        state.files = accepted
        state.file = accepted[0] || null
      } else {
        state.file = accepted[0] || null
        state.files = []
      }
      setError(errors.length ? `部分文件未添加：${errors.join('；')}` : '')
      updateSelectedFile()
    } catch {
      setError('无法加载资源校验模块，请刷新页面后重试。')
    }
  }

  function requestWithUploadProgress(url: string, method: string, body: ResourceWrite): Promise<{ item?: TeachingResource; error?: string }> {
    return new Promise((resolve, reject) => {
      const xhr = new window.XMLHttpRequest()
      xhr.open(method, url)
      xhr.withCredentials = true
      xhr.setRequestHeader('Content-Type', 'application/json')
      xhr.setRequestHeader('Accept', 'application/json')
      xhr.upload.onprogress = event => {
        if (event.lengthComputable) {
          state.progress = Math.min(99, Math.round(event.loaded / event.total * 100))
          updateProgress()
        }
      }
      xhr.onerror = () => reject(new Error('网络连接失败，资源未能保存。'))
      xhr.onload = () => {
        let result: { item?: TeachingResource; error?: string } = {}
        try { result = JSON.parse(xhr.responseText) } catch { /* Status handling below covers invalid response bodies. */ }
        if (xhr.status < 200 || xhr.status >= 300) {
          reject(new Error(result.error || '保存资源失败。'))
          return
        }
        state.progress = 100
        updateProgress()
        resolve(result)
      }
      xhr.send(JSON.stringify(body))
    })
  }

  async function readFile(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new window.FileReader()
      reader.onerror = () => reject(new Error('读取文件失败。'))
      reader.onload = () => resolve(String(reader.result).split(',')[1] || '')
      reader.readAsDataURL(file)
    })
  }

  async function createResource() {
    if (state.busy || state.uploadDone) return
    const batch = state.batchMode && !editingId
    const files = batch ? state.files : (state.file ? [state.file] : [])
    try {
      const { validateResourceForm } = await import('../utils/teachingResourceValidation')
      if (disposed) return
      const validationError = validateResourceForm({ title: state.title, batchMode: batch, editing: Boolean(editingId), files, url: state.url })
      if (validationError) {
        setError(validationError)
        return
      }
    } catch {
      setError('无法加载资源校验模块，请刷新页面后重试。')
      return
    }

    setBusy(true)
    setError('')
    resetUpload()
    if (liveProgress) liveProgress.hidden = false
    if (submitButton) submitButton.textContent = '正在保存…'
    const common = {
      title: state.title.trim(),
      description: state.description.trim(),
      type: state.type,
      grade: (state.grade || null) as ResourceWrite['grade'],
      isPublic: state.isPublic,
      url: state.url.trim() || null,
    }
    const created: TeachingResource[] = []
    const failures: string[] = []
    let shouldRefresh = false
    try {
      for (const file of (files.length ? files : [null])) {
        try {
          const data = file ? await readFile(file) : ''
          const payload: ResourceWrite = { ...common, ...(file ? { filename: file.name, data } : {}) }
          const endpoint = editingId ? `/api/resources/${encodeURIComponent(editingId)}` : '/api/resources'
          const result = await requestWithUploadProgress(endpoint, editingId ? 'PATCH' : 'POST', payload)
          if (!result.item) throw new Error('服务器未返回已保存的资源。')
          created.push(result.item)
        } catch (error) {
          failures.push(`${file?.name || '资源链接'}：${error instanceof Error ? error.message : '保存失败'}`)
        }
      }
      if (!created.length) throw new Error(failures.join('；') || '资源保存失败。')
      shouldRefresh = true
      state.file = null
      state.files = []
      state.uploadDone = true
      state.uploadResult = failures.length
        ? `已成功创建 ${created.length} 条资源，失败 ${failures.length} 条。失败详情：${failures.join('；')}`
        : batch
          ? `批量上传完成：${created.length} 个文件已分别创建为独立资源。请点击下方“完成”关闭窗口。`
          : editingId
            ? '资源已更新。请点击下方“完成”关闭窗口。'
            : '资源已保存。请点击下方“完成”关闭窗口。'
      if (!disposed) renderCompletion()
    } catch (error) {
      state.error = error instanceof Error ? error.message : '保存资源失败。'
      state.progress = 0
      updateProgress()
      setError(state.error)
    } finally {
      if (!disposed) {
        setBusy(false)
        if (!state.uploadDone && submitButton) submitButton.textContent = editingId ? '保存并处理' : '保存资源'
        if (shouldRefresh) options.onSaved()
      }
    }
  }

  function finishUpload() {
    if (!state.uploadDone || state.progress < 100 || disposed) return
    options.onCompleted()
    options.onClose()
  }

  function onSubmit(event: Event) {
    event.preventDefault()
    void createResource()
  }

  function onBackdropClick(event: MouseEvent) {
    if (event.target === backdrop && !isLocked()) options.onClose()
  }
  function onKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape' && !isLocked()) options.onClose()
  }

  backdrop.addEventListener('click', onBackdropClick)
  backdrop.addEventListener('keydown', onKeyDown)
  renderForm()

  return {
    dispose() {
      if (disposed) return
      disposed = true
      if (state.busy) options.onBusyChange(false)
      disposeSelects()
      backdrop.removeEventListener('click', onBackdropClick)
      backdrop.removeEventListener('keydown', onKeyDown)
      host.replaceChildren()
    },
  }
}
