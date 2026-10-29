<!--
Responsibility: own the teaching-resource create/edit dialog, file selection, client validation, upload progress, and completion state.
Dependencies: Vue, the existing virtualized select, generated API types, and the on-demand resource-validation utility.
Collaboration: App.vue opens this lazy component with an optional resource, synchronizes the shared busy flag, and refreshes the resource list after a save.
Boundaries: sends the existing same-origin /api/resources requests; server validation and authorization remain authoritative. No production state is changed directly.
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ResourceWrite, TeachingResource } from '../api/generated'
import { mountVirtualizedSelect, type VirtualizedSelectOption } from './virtualizedSelect'
import { formatFileSize } from '../utils/resourceFiles'

const props = defineProps<{
  resource: TeachingResource | null
  resourceTypeOptions: string[]
  resourceGradeOptions: string[]
  busy: boolean
}>()
const emit = defineEmits<{
  close: []
  'update:busy': [value: boolean]
  saved: []
  completed: []
}>()

const editingResourceId = computed(() => props.resource?.id ?? null)
const resourceTitle = ref(props.resource?.title ?? '')
const resourceDescription = ref(props.resource?.description ?? '')
const resourceFile = ref<File | null>(null)
const resourceFiles = ref<File[]>([])
const resourceBatchMode = ref(false)
const resourceType = ref(props.resource?.type || '听力材料')
const resourceGrade = ref(props.resource?.grade || '')
const resourceIsPublic = ref(props.resource?.isPublic === true)
const resourceUrl = ref(props.resource?.url || '')
const resourceUploadProgress = ref(0)
const resourceUploadDone = ref(false)
const resourceUploadResult = ref('')
const resourceError = ref('')
const isLocked = computed(() => props.busy || resourceUploadDone.value)
const resourceTypeSelectHost = ref<HTMLElement | null>(null)
const resourceGradeSelectHost = ref<HTMLElement | null>(null)
let disposeResourceTypeSelect: (() => void) | null = null
let disposeResourceGradeSelect: (() => void) | null = null
const resourceTypeSelectOptions = computed<VirtualizedSelectOption[]>(() => props.resourceTypeOptions.map(type => ({ label: type, value: type })))
const resourceGradeSelectOptions = computed<VirtualizedSelectOption[]>(() => props.resourceGradeOptions.map(grade => ({ label: grade, value: grade === '不限定年级' ? '' : grade })))
onMounted(() => {
  if (resourceTypeSelectHost.value) {
    disposeResourceTypeSelect = mountVirtualizedSelect(resourceTypeSelectHost.value, { label: '教学资源类型', options: resourceTypeSelectOptions.value, value: resourceType.value, onChange: value => { resourceType.value = value } }).dispose
  }
  if (resourceGradeSelectHost.value) {
    disposeResourceGradeSelect = mountVirtualizedSelect(resourceGradeSelectHost.value, { label: '年级', placeholder: '选择适用年级', options: resourceGradeSelectOptions.value, value: resourceGrade.value, onChange: value => { resourceGrade.value = value } }).dispose
  }
})
onBeforeUnmount(() => { disposeResourceTypeSelect?.(); disposeResourceGradeSelect?.() })

function resetResourceUpload() {
  resourceUploadProgress.value = 0
  resourceUploadDone.value = false
  resourceUploadResult.value = ''
}

function handleResourceFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  if (resourceBatchMode.value && !editingResourceId.value) {
    resourceFiles.value = files
    resourceFile.value = files[0] || null
  } else {
    resourceFile.value = files[0] || null
    resourceFiles.value = []
  }
  resourceError.value = ''
}

function clearResourceSelectionForMode() {
  resourceFile.value = null
  resourceFiles.value = []
  resourceError.value = ''
}

/** Send one existing resource mutation and report upload progress; HTTP failures reject with a safe server message. */
function requestWithUploadProgress(url: string, method: string, body: ResourceWrite): Promise<{ item?: TeachingResource; error?: string }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open(method, url)
    xhr.withCredentials = true
    xhr.setRequestHeader('Content-Type', 'application/json')
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) resourceUploadProgress.value = Math.min(99, Math.round(event.loaded / event.total * 100))
    }
    xhr.onerror = () => reject(new Error('网络连接失败，资源未能保存。'))
    xhr.onload = () => {
      let result: { item?: TeachingResource; error?: string } = {}
      try { result = JSON.parse(xhr.responseText) } catch { /* The status check below reports an invalid/error response. */ }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(new Error(result.error || '保存资源失败。'))
        return
      }
      resourceUploadProgress.value = 100
      resolve(result)
    }
    xhr.send(JSON.stringify(body))
  })
}

/** Validate once, upload selected files sequentially, preserve partial batch outcomes, then release busy before refreshing the list. */
async function createTeachingResource() {
  const batch = resourceBatchMode.value && !editingResourceId.value
  const files = batch ? resourceFiles.value : (resourceFile.value ? [resourceFile.value] : [])
  try {
    const { validateResourceForm } = await import('../utils/teachingResourceValidation')
    const validationError = validateResourceForm({
      title: resourceTitle.value,
      batchMode: batch,
      editing: Boolean(editingResourceId.value),
      files,
      url: resourceUrl.value,
    })
    if (validationError) {
      resourceError.value = validationError
      return
    }
  } catch {
    resourceError.value = '无法加载资源校验模块，请刷新页面后重试。'
    return
  }

  emit('update:busy', true)
  let shouldRefresh = false
  resourceError.value = ''
  resetResourceUpload()
  try {
    const common = {
      title: resourceTitle.value.trim(),
      description: resourceDescription.value.trim(),
      type: resourceType.value,
      grade: (resourceGrade.value || null) as ResourceWrite['grade'],
      isPublic: resourceIsPublic.value,
      url: resourceUrl.value.trim() || null,
    }
    const created: TeachingResource[] = []
    const failures: string[] = []
    for (const file of (files.length ? files : [null])) {
      try {
        let data = ''
        if (file) {
          data = await new Promise<string>((resolve, reject) => {
            const reader = new FileReader()
            reader.onerror = () => reject(new Error('读取文件失败。'))
            reader.onload = () => resolve(String(reader.result).split(',')[1] || '')
            reader.readAsDataURL(file)
          })
        }
        const payload: ResourceWrite = { ...common, ...(file ? { filename: file.name, data } : {}) }
        const endpoint = editingResourceId.value ? `/api/resources/${encodeURIComponent(editingResourceId.value)}` : '/api/resources'
        const result = await requestWithUploadProgress(endpoint, editingResourceId.value ? 'PATCH' : 'POST', payload)
        if (!result.item) throw new Error('服务器未返回已保存的资源。')
        created.push(result.item)
      } catch (error) {
        failures.push(`${file?.name || '资源链接'}：${error instanceof Error ? error.message : '保存失败'}`)
      }
    }

    if (!created.length) throw new Error(failures.join('；') || '资源保存失败。')
    shouldRefresh = true
    resourceFile.value = null
    resourceFiles.value = []
    resourceUploadDone.value = true
    resourceUploadResult.value = failures.length
      ? `已成功创建 ${created.length} 条资源，失败 ${failures.length} 条。失败详情：${failures.join('；')}`
      : batch
        ? `批量上传完成：${created.length} 个文件已分别创建为独立资源。请点击下方“完成”关闭窗口。`
        : editingResourceId.value
          ? '资源已更新。请点击下方“完成”关闭窗口。'
          : '资源已保存。请点击下方“完成”关闭窗口。'
  } catch (error) {
    resourceError.value = error instanceof Error ? error.message : '保存资源失败。'
    resourceUploadProgress.value = 0
  } finally {
    emit('update:busy', false)
    if (shouldRefresh) emit('saved')
  }
}

/** Capture file paste synchronously, then apply the shared extension/size rules without blocking the browser event. */
async function handleResourcePaste(event: ClipboardEvent) {
  if (props.busy || resourceUploadDone.value) return
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
    const { accepted, errors } = filterPastedResourceFiles(files)
    if (resourceBatchMode.value && !editingResourceId.value) {
      resourceFiles.value = accepted
      resourceFile.value = accepted[0] || null
    } else {
      resourceFile.value = accepted[0] || null
      resourceFiles.value = []
    }
    resourceError.value = errors.length ? `部分文件未添加：${errors.join('；')}` : ''
  } catch {
    resourceError.value = '无法加载资源校验模块，请刷新页面后重试。'
  }
}

function finishResourceUpload() {
  if (!resourceUploadDone.value || resourceUploadProgress.value < 100) return
  emit('completed')
  emit('close')
}
</script>

<template>
  <div class="share-dialog-backdrop resource-upload-backdrop" @click.self="!isLocked && emit('close')" @keydown.esc="!isLocked && emit('close')">
    <section class="share-dialog resource-create-dialog" role="dialog" aria-modal="true" aria-labelledby="resource-create-title">
      <button v-if="!isLocked && !resourceUploadDone" type="button" class="share-dialog-close" aria-label="关闭" @click="emit('close')"><i class="bi bi-x-lg" aria-hidden="true"></i></button>
      <span class="share-dialog-eyebrow">TEACHING LIBRARY</span>
      <h2 id="resource-create-title">{{ editingResourceId ? '编辑教学资源' : '创建教学资源' }}</h2>
      <p>{{ editingResourceId ? '可更新资源名称、类型、说明、分享链接；选择新文件才会替换原文件。' : '文件上传与 URL 分享二选一，也可以同时填写。' }}</p>
      <form v-if="!resourceUploadDone" class="resource-create-form" @submit.prevent="createTeachingResource" @paste="handleResourcePaste">
        <label class="share-dialog-label" for="resource-title">资源名称</label>
        <input id="resource-title" v-model="resourceTitle" class="form-control share-dialog-url" maxlength="200" required placeholder="例如：七年级英语听力 Unit 1" />
        <label class="share-dialog-label" for="resource-type">类型</label>
        <div ref="resourceTypeSelectHost"></div>
        <label class="share-dialog-label" for="resource-grade">年级</label>
        <div ref="resourceGradeSelectHost"></div>
        <label class="resource-public-toggle"><input v-model="resourceIsPublic" type="checkbox" /><span><strong>公开展示在教育资源页</strong><small>公开后访客可查看并访问资源。请确保你有权分享所提供的内容。</small></span></label>
        <label class="share-dialog-label" for="resource-description">资源说明</label>
        <textarea id="resource-description" v-model="resourceDescription" class="form-control share-dialog-url resource-description-input" maxlength="2000" placeholder="资源内容、课程、章节或使用说明（选填）"></textarea>
        <label v-if="!editingResourceId" class="resource-batch-toggle"><input v-model="resourceBatchMode" type="checkbox" @change="clearResourceSelectionForMode" /><span>批量上传（每个文件单独创建一条资源）</span></label>
        <label class="share-dialog-label" for="resource-file">{{ resourceBatchMode && !editingResourceId ? '上传文件（可多选）' : '上传文件（选填）' }}</label>
        <input id="resource-file" class="form-control resource-file-input" type="file" :multiple="resourceBatchMode && !editingResourceId" accept=".ppt,.pptx,.mp3,.pdf,.doc,.docx,.txt" @change="handleResourceFileChange" />
        <p class="resource-file-hint">支持 PPT / PPTX、MP3、PDF、DOC / DOCX、TXT；单个文件不超过 25 MB。批量模式下，每个文件都会创建一条独立资源，其他填写信息共用。也可复制文件后在表单中按 Ctrl+V 粘贴（取决于浏览器支持）。不上传文件时可直接使用 URL。</p>
        <p v-if="resourceBatchMode && resourceFiles.length" class="resource-selected-file">已选择 {{ resourceFiles.length }} 个文件：{{ resourceFiles.map(file => file.name).join('、') }}</p>
        <p v-else-if="resourceFile" class="resource-selected-file">{{ resourceFile.name }} · {{ formatFileSize(resourceFile.size) }}{{ editingResourceId ? ' · 上传后替换原文件' : '' }}</p>
        <label class="share-dialog-label" for="resource-url">URL 链接（选填）</label>
        <input id="resource-url" v-model="resourceUrl" type="url" class="form-control share-dialog-url" placeholder="粘贴可分享的 https:// 链接" />
        <div v-if="props.busy" class="resource-upload-live-progress">
          <div class="resource-upload-progress-track" role="progressbar" :aria-valuenow="resourceUploadProgress" aria-valuemin="0" aria-valuemax="100"><div class="resource-upload-progress-bar" :style="{ width: resourceUploadProgress + '%' }"></div></div>
          <div class="resource-upload-progress-label"><span>{{ resourceUploadProgress >= 100 ? '正在等待服务器确认保存' : '正在上传并保存，请勿关闭窗口' }}</span><strong>{{ resourceUploadProgress }}%</strong></div>
        </div>
        <p v-if="resourceError" class="resource-error">{{ resourceError }}</p>
        <div class="resource-form-actions"><button type="button" class="btn content-secondary-button" :disabled="props.busy" @click="emit('close')">取消</button><button type="submit" class="btn content-primary-button" :disabled="props.busy">{{ props.busy ? '正在保存…' : (editingResourceId ? '保存并处理' : '保存资源') }}</button></div>
      </form>
      <div v-else class="resource-upload-completion">
        <p>{{ resourceUploadResult }}</p>
        <div class="resource-upload-progress-track" role="progressbar" :aria-valuenow="resourceUploadProgress" aria-valuemin="0" aria-valuemax="100"><div class="resource-upload-progress-bar" :style="{ width: resourceUploadProgress + '%' }"></div></div>
        <div class="resource-upload-progress-label"><span>{{ resourceUploadProgress >= 100 ? '处理完成' : '正在处理' }}</span><strong>{{ resourceUploadProgress }}%</strong></div>
        <button type="button" class="btn content-primary-button" :disabled="resourceUploadProgress < 100 || !resourceUploadDone" @click="finishResourceUpload">完成</button>
      </div>
    </section>
  </div>
</template>
