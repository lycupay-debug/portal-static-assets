<script setup lang="ts">
type Resource = {
  id: string
  title: string
  description?: string | null
  type?: string | null
  grade?: string | null
  filename?: string | null
  fileSize?: number | null
  url?: string | null
  downloadUrl?: string | null
  createdAt?: string | null
  updatedAt?: string | null
}
defineProps<{ loading: boolean; error: string; resource: Resource | null }>()
const emit = defineEmits<{ back: [] }>()
function formatResourceFileSize(size?: number | null) {
  if (size == null || !Number.isFinite(size) || size < 0) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}
</script>

<template>
  <main class="route-main compliance-resource-preview-page">
    <section v-if="loading" class="public-content-state">正在读取受限资源预览…</section>
    <section v-else-if="error" class="public-content-state"><h1>无法预览资源</h1><p>{{ error }}</p><button type="button" class="education-resource-detail-close" @click="emit('back')">返回版权合规中心</button></section>
    <div v-else-if="resource" class="share-dialog-backdrop education-resource-detail-backdrop">
      <section class="education-resource-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="review-resource-preview-title">
        <div class="education-resource-detail-header"><span class="eyebrow">COPYRIGHT REVIEW · RESTRICTED PREVIEW</span><span class="compliance-preview-badge">仅审核人员可见</span></div>
        <h2 id="review-resource-preview-title">{{ resource.title }}</h2>
        <p v-if="resource.description" class="education-resource-detail-description">{{ resource.description }}</p>
        <dl class="education-resource-detail-meta">
          <div><dt>资源类型</dt><dd>{{ resource.type || '未分类' }}</dd></div>
          <div><dt>适用年级</dt><dd>{{ resource.grade || '不限定年级' }}</dd></div>
          <div v-if="resource.filename"><dt>文件名称</dt><dd>{{ resource.filename }}</dd></div>
          <div v-if="resource.fileSize != null"><dt>文件大小</dt><dd>{{ formatResourceFileSize(resource.fileSize) }}</dd></div>
          <div v-if="resource.url"><dt>在线链接</dt><dd><a class="education-resource-detail-url" :href="resource.url" target="_blank" rel="noopener noreferrer">{{ resource.url }}</a></dd></div>
          <div><dt>更新时间</dt><dd>{{ new Date(resource.updatedAt || resource.createdAt || Date.now()).toLocaleDateString('zh-CN') }}</dd></div>
        </dl>
        <div v-if="resource.filename?.toLowerCase().endsWith('.mp3') && resource.downloadUrl" class="education-resource-audio-preview"><label :for="'review-resource-audio-' + resource.id">MP3 音频预览</label><audio :id="'review-resource-audio-' + resource.id" controls preload="metadata" :src="resource.downloadUrl.replace(/\/download$/, '/preview')">当前浏览器不支持音频播放。</audio></div>
        <p v-if="!resource.url && !resource.downloadUrl" class="education-resource-detail-warning">该资源没有可访问的文件或链接。</p>
        <div class="education-resource-detail-actions">
          <a v-if="resource.url" class="education-resource-detail-action education-resource-detail-open" :href="resource.url" target="_blank" rel="noopener noreferrer">打开链接</a>
          <a v-if="resource.downloadUrl" class="education-resource-detail-action education-resource-detail-download" :href="resource.downloadUrl">下载文件</a>
          <button type="button" class="education-resource-detail-close" @click="emit('back')" aria-label="返回版权合规中心">×</button>
        </div>
      </section>
    </div>
  </main>
</template>
