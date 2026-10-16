<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

type Option = { label: string; value: string }
const props = withDefaults(defineProps<{ modelValue: string | null; options: Option[]; label: string; placeholder?: string; disabled?: boolean }>(), { placeholder: '请选择', disabled: false })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const open = ref(false)
const query = ref('')
const scrollTop = ref(0)
const root = ref<HTMLElement | null>(null)
const rowHeight = 36
const viewportHeight = 216
const selected = computed(() => props.options.find(option => option.value === props.modelValue))
const filtered = computed(() => props.options.filter(option => option.label.toLocaleLowerCase().includes(query.value.trim().toLocaleLowerCase())))
const startIndex = computed(() => Math.max(0, Math.floor(scrollTop.value / rowHeight) - 2))
const endIndex = computed(() => Math.min(filtered.value.length, startIndex.value + Math.ceil(viewportHeight / rowHeight) + 4))
const visibleOptions = computed(() => filtered.value.slice(startIndex.value, endIndex.value))
function choose(value: string) { emit('update:modelValue', value); open.value = false; query.value = ''; scrollTop.value = 0 }
function onOutside(event: MouseEvent) { if (root.value && !root.value.contains(event.target as Node)) open.value = false }
function onKeys(event: KeyboardEvent) { if (event.key === 'Escape') open.value = false }
watch(open, value => { if (value) { query.value = ''; scrollTop.value = 0 } })
onMounted(() => { document.addEventListener('mousedown', onOutside); document.addEventListener('keydown', onKeys) })
onBeforeUnmount(() => { document.removeEventListener('mousedown', onOutside); document.removeEventListener('keydown', onKeys) })
</script>

<template>
  <div ref="root" class="virtualized-select" :class="{ 'is-open': open, 'is-disabled': disabled }">
    <button type="button" class="virtualized-select-trigger" :aria-label="label" :aria-expanded="open" :disabled="disabled" @click="open = !open">
      <span>{{ selected?.label || placeholder }}</span><span class="virtualized-select-chevron">⌄</span>
    </button>
    <div v-if="open" class="virtualized-select-popover" role="listbox" :aria-label="label">
      <input v-model="query" name="optionSearch" class="form-control virtualized-select-search" type="search" placeholder="搜索选项…" :aria-label="'搜索' + label" @keydown.stop />
      <div class="virtualized-select-viewport" :style="{ height: viewportHeight + 'px' }" @scroll="scrollTop = ($event.target as HTMLElement).scrollTop">
        <div :style="{ height: startIndex * rowHeight + 'px' }"></div>
        <button v-for="option in visibleOptions" :key="option.value" type="button" role="option" :aria-selected="option.value === modelValue" :class="{ selected: option.value === modelValue }" :style="{ height: rowHeight + 'px' }" @click="choose(option.value)"><span>{{ option.label }}</span><span v-if="option.value === modelValue">✓</span></button>
        <div :style="{ height: Math.max(0, (filtered.length - endIndex) * rowHeight) + 'px' }"></div>
        <p v-if="!filtered.length" class="virtualized-select-empty">没有匹配的选项</p>
      </div>
    </div>
  </div>
</template>
