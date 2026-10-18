<script setup lang="ts">
import type { TeachingResource } from '../api/generated'
const props = defineProps<{ resource: TeachingResource; resourceViewMode: 'active' | 'trash' }>()
const emit = defineEmits<{ close: []; edit: [id: string]; download: [id: string] }>()
function formatFileSize(size: number) { return size < 1024 * 1024 ? `${Math.max(1, Math.round(size / 1024))} KB` : `${(size / (1024 * 1024)).toFixed(1)} MB` }
</script>
<template>
  <div class="resource-detail-drawer-backdrop" @click.self="emit('close')" @keydown.esc="emit('close')">
    <aside class="resource-detail-drawer" role="dialog" aria-modal="true" aria-labelledby="resource-detail-title">
      <header class="resource-detail-drawer-heading"><div><span class="share-dialog-eyebrow">TEACHING RESOURCE</span><h2 id="resource-detail-title">资源详情</h2></div><button type="button" aria-label="关闭详情" @click="emit('close')"><i class="bi bi-x-lg" aria-hidden="true"></i></button></header>
      <div class="resource-detail-drawer-body">
        <h3>{{ props.resource.title }}</h3>
        <dl class="resource-detail-fields">
          <div><dt>资源名称</dt><dd>{{ props.resource.title || '—' }}</dd></div>
          <div><dt>文件名称</dt><dd>{{ props.resource.filename || '未上传文件' }}</dd></div>
          <div><dt>资源类型</dt><dd>{{ props.resource.type || '其他' }}</dd></div>
          <div><dt>适用年级</dt><dd>{{ props.resource.grade || '不限定年级' }}</dd></div>
          <div><dt>公开状态</dt><dd>{{ props.resource.isPublic ? '公开展示' : '仅自己可见' }}</dd></div>
          <div><dt>文件大小</dt><dd>{{ props.resource.fileSize ? formatFileSize(props.resource.fileSize) : '—' }}</dd></div>
          <div><dt>MIME 类型</dt><dd>{{ props.resource.mimeType || '—' }}</dd></div>
          <div><dt>资源链接</dt><dd><a v-if="props.resource.url" :href="props.resource.url" target="_blank" rel="noopener noreferrer">{{ props.resource.url }}</a><span v-else>站内文件</span></dd></div>
          <div><dt>资源说明</dt><dd>{{ props.resource.description || '暂无说明' }}</dd></div>
          <div><dt>创建时间</dt><dd>{{ props.resource.createdAt ? new Date(props.resource.createdAt).toLocaleString('zh-CN') : '—' }}</dd></div>
          <div><dt>更新时间</dt><dd>{{ props.resource.updatedAt ? new Date(props.resource.updatedAt).toLocaleString('zh-CN') : '—' }}</dd></div>
          <div v-if="props.resource.deletedAt"><dt>移入回收站时间</dt><dd>{{ new Date(props.resource.deletedAt).toLocaleString('zh-CN') }}</dd></div>
          <div><dt>资源 ID</dt><dd class="resource-detail-id">{{ props.resource.id }}</dd></div>
        </dl>
        <div class="resource-detail-actions"><button v-if="props.resourceViewMode === 'active'" type="button" class="btn content-secondary-button" @click="emit('edit', props.resource.id)">编辑资源</button><button v-if="props.resourceViewMode === 'active'" type="button" class="btn content-primary-button" @click="emit('download', props.resource.id)">{{ props.resource.url ? '打开资源链接' : '下载文件' }}</button></div>
      </div>
    </aside>
  </div>
</template>
