<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import { createVditorOptions } from '../utils/vditorEditor'

const props = defineProps<{ modelValue: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const editorHost = ref<HTMLDivElement | null>(null)
let editor: Vditor | null = null

onMounted(() => {
  if (!editorHost.value) return
  editor = new Vditor(editorHost.value, createVditorOptions(props.modelValue || '', (markdown) => {
    emit('update:modelValue', markdown)
  }))
  if (props.disabled) editor.disabled()
})

watch(() => props.modelValue, (value) => {
  if (editor && editor.getValue() !== value) editor.setValue(value || '')
})

watch(() => props.disabled, (disabled) => {
  if (!editor) return
  if (disabled) editor.disabled()
  else editor.enable()
})

onBeforeUnmount(() => {
  editor?.destroy()
  editor = null
})
</script>

<template>
  <div class="notion-editor" :class="{ 'is-disabled': disabled }">
    <div ref="editorHost" class="vditor-editor-host" aria-label="文档正文编辑器" />
    <div class="notion-editor-footer">
      <span>Markdown 文档 · Vditor</span>
      <span>支持待办清单、表格、代码块与分屏预览</span>
    </div>
  </div>
</template>
