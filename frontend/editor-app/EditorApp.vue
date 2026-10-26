<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import ContentDocumentEditor from '../src/components/ContentDocumentEditor.vue'
import { portalTaxonomy } from '../src/utils/taxonomy'

type PortalUser = { id: string; email: string; displayName: string; isAdmin: boolean }
type ContentItem = {
  id: string; title: string; slug: string; contentType: string; tags: string[]
  summary: string; body: string; visibility: 'public' | 'private'; status?: string
}
type ApiResult<T> = { user?: T; item?: T; error?: string }

const contentTypes = [
  { value: 'article', label: '文章' },
  { value: 'tool', label: '工具' },
  { value: 'resource', label: '资源' },
  { value: 'project', label: '项目' },
]
const tagOptions = [
  '教育', '学前教育', '小学教育', '中学教育', '高等教育', '职业教育', '家庭教育', '教学资源',
  '教育人力资源', '事业编', '国企', '省考', '国考', '旅游', '住宿', '交通', '景点攻略', '旅行规划', '美食体验',
  '网络', '网络安全', '网络维护', '网络基础知识', '网络诊断', '云与服务器', '系统', '操作系统', '系统配置',
  '系统维护', '部署运维', '故障排查', '编程', '前端开发', '后端开发', 'Python', 'JavaScript / TypeScript',
  '数据库', 'API 与集成', '代码实践', '软件工具', '效率工具', '开发工具', '文档工具', 'AI 工具', '自动化工具',
]
const removedGradeTags = new Set(['年级', '一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '初一', '初二', '初三', '高一', '高二', '高三', '本科', '硕士'])
const taxonomyParentNames = new Set<string>(portalTaxonomy.filter(group => group.value !== '年级').map(group => group.value))
const path = window.location.pathname.replace(/\/+$/, '') || '/editor'
const routeId = path.match(/^\/editor\/([0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})$/i)?.[1] || null
const isNewDocument = !routeId
const user = ref<PortalUser | null>(null)
const loading = ref(true)
const busy = ref(false)
const dirty = ref(false)
const error = ref('')
const notice = ref('')
const slugTouched = ref(false)
const form = ref<ContentItem>({
  id: '', title: '', slug: '', contentType: 'article', tags: [],
  summary: '', body: '', visibility: 'private', status: 'draft',
})
const title = computed(() => form.value.title.trim() || '未命名文档')
const tagGroups = computed(() => portalTaxonomy.map(group => ({
  ...group,
  children: group.children.filter(tag => tagOptions.includes(tag)),
})).filter(group => group.children.length))
const slugIsValid = computed(() => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(form.value.slug.trim()))
let initialSnapshot = ''

function slugify(value: string) {
  const slug = value.normalize('NFKD').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 100)
  return slug || `document-${Date.now().toString(36)}`
}
function normalizeTags(tags: string[]) {
  const children = [...new Set(tags.filter(tag => tagOptions.includes(tag) && !taxonomyParentNames.has(tag) && !removedGradeTags.has(tag)))]
  const parents = portalTaxonomy
    .filter(group => group.value !== '年级' && group.children.some(tag => children.includes(tag)))
    .map(group => group.value)
  return [...parents, ...children]
}
function snapshot() {
  return JSON.stringify({
    title: form.value.title, slug: form.value.slug, contentType: form.value.contentType,
    tags: [...form.value.tags].sort(), summary: form.value.summary, body: form.value.body,
    visibility: form.value.visibility,
  })
}
function markClean() {
  initialSnapshot = snapshot()
  dirty.value = false
}
watch(() => form.value.title, value => {
  if (!slugTouched.value) form.value.slug = slugify(value)
})
watch(form, () => {
  dirty.value = Boolean(initialSnapshot) && snapshot() !== initialSnapshot
}, { deep: true })

async function requestJSON<T>(url: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(url, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...(init.body ? { 'Content-Type': 'application/json' } : {}), ...init.headers },
    ...init,
  })
  const result = await response.json() as T & { error?: string }
  if (!response.ok) {
    if (response.status === 401) window.location.replace('/login')
    throw new Error(result.error || `请求失败（HTTP ${response.status}）`)
  }
  return result
}

async function loadDocument() {
  if (!routeId) {
    markClean()
    return
  }
  const result = await requestJSON<ApiResult<ContentItem>>('/api/content/' + encodeURIComponent(routeId))
  if (!result.item) throw new Error('接口没有返回文档内容。')
  const item = result.item
  form.value = {
    id: item.id, title: item.title || '', slug: item.slug || '', contentType: item.contentType || 'article',
    tags: (item.tags || []).filter(tag => !taxonomyParentNames.has(tag) && !removedGradeTags.has(tag)), summary: item.summary || '',
    body: item.body || '', visibility: item.visibility || 'private', status: item.status || 'draft',
  }
  slugTouched.value = true
  markClean()
}

function toggleTag(tag: string) {
  const tags = new Set(form.value.tags)
  if (tags.has(tag)) tags.delete(tag)
  else if (tags.size < 40) tags.add(tag)
  form.value.tags = [...tags]
}
function setGroup(group: string, checked: boolean) {
  const children = tagGroups.value.find(item => item.value === group)?.children || []
  const tags = new Set(form.value.tags)
  for (const tag of children) {
    if (checked) { if (tags.size < 40) tags.add(tag) }
    else tags.delete(tag)
  }
  form.value.tags = [...tags]
}
function groupState(group: string) {
  const children = tagGroups.value.find(item => item.value === group)?.children || []
  return children.length > 0 && children.every(tag => form.value.tags.includes(tag))
}
function clearError() { error.value = ''; notice.value = '' }

async function saveDocument() {
  clearError()
  if (!form.value.title.trim()) { error.value = '请填写文档标题。'; return }
  if (!slugIsValid.value) { error.value = 'URL 别名只能使用小写英文字母、数字和连字符。中文标题会自动生成别名，请检查是否为空。'; return }
  if (!form.value.body.trim()) { error.value = '正文不能为空。'; return }
  const unsupportedTags = form.value.tags.filter(tag => !tagOptions.includes(tag))
  if (unsupportedTags.length) { error.value = '发现已不在当前标签目录中的历史标签，请先移除：' + unsupportedTags.join('、'); return }
  if (!form.value.tags.length) { error.value = '请至少选择一个有效标签。'; return }
  busy.value = true
  try {
    const payload = {
      title: form.value.title.trim(), slug: form.value.slug.trim(), contentType: form.value.contentType,
      tags: normalizeTags(form.value.tags), parentCategory: null, childCategory: null,
      summary: form.value.summary.trim(), body: form.value.body, visibility: form.value.visibility,
    }
    const editingId = form.value.id || routeId
    const result = await requestJSON<ApiResult<ContentItem>>(editingId ? '/api/content/' + encodeURIComponent(editingId) : '/api/content', {
      method: editingId ? 'PATCH' : 'POST',
      body: JSON.stringify(payload),
    })
    if (!result.item) throw new Error('保存请求成功，但没有返回文档。')
    form.value.id = result.item.id
    form.value.status = result.item.status || 'draft'
    markClean()
    if (!routeId && result.item.id) history.replaceState({}, '', '/editor/' + encodeURIComponent(result.item.id))
    notice.value = editingId ? '文档已保存。' : '草稿已创建。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败，请稍后重试。'
  } finally {
    busy.value = false
  }
}

function leaveEditor() {
  if (dirty.value && !window.confirm('文档存在未保存的修改，仍要离开吗？')) return
  window.location.assign('/content-management')
}
function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!dirty.value || busy.value) return
  event.preventDefault()
  event.returnValue = ''
}
function onShortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    if (!busy.value) void saveDocument()
  }
}
onMounted(async () => {
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('keydown', onShortcut)
  try {
    const session = await requestJSON<ApiResult<PortalUser>>('/api/auth/me')
    if (!session.user) { window.location.replace('/login'); return }
    user.value = session.user
    await loadDocument()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '无法初始化编辑器。'
  } finally {
    loading.value = false
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  window.removeEventListener('keydown', onShortcut)
})
</script>

<template>
  <main class="editor-app-shell">
    <header class="editor-app-topbar">
      <a class="editor-app-brand" href="/content-management" @click.prevent="leaveEditor">
        <span class="editor-app-mark">R.</span>
        <span><strong>文档工作台</strong><small>MARKDOWN STUDIO</small></span>
      </a>
      <div class="editor-app-title">
        <span class="editor-state-dot" :class="{ 'is-dirty': dirty, 'is-saved': !dirty && !loading }"></span>
        <span class="editor-app-title-text">{{ title }}</span>
        <span class="editor-state-label">{{ loading ? '加载中' : dirty ? '未保存' : '已同步' }}</span>
      </div>
      <div class="editor-app-actions">
        <span class="editor-app-user">{{ user?.displayName || user?.email || '' }}</span>
        <button type="button" class="editor-button editor-button-quiet" :disabled="busy" @click="leaveEditor">返回内容管理</button>
        <button type="button" class="editor-button editor-button-primary" :disabled="busy || loading" @click="saveDocument">
          <span class="bi bi-cloud-check" aria-hidden="true"></span>{{ busy ? '正在保存…' : '保存草稿' }}
        </button>
      </div>
    </header>

    <div v-if="error || notice" class="editor-feedback" :class="{ 'is-error': error, 'is-success': notice && !error }" role="status" aria-live="polite">
      <span class="bi" :class="error ? 'bi-exclamation-circle' : 'bi-check-circle'" aria-hidden="true"></span>
      <span>{{ error || notice }}</span>
      <button type="button" aria-label="关闭提示" @click="clearError">×</button>
    </div>

    <section v-if="loading" class="editor-loading"><span class="editor-loading-spinner"></span><strong>正在准备编辑空间</strong><small>验证会话并加载文档</small></section>
    <section v-else class="editor-app-layout">
      <section class="editor-app-document">
        <div class="editor-document-toolbar">
          <span class="editor-breadcrumb">工作区 <span>/</span> 文档编辑</span>
          <span class="editor-document-badge"><span class="bi bi-file-earmark-text" aria-hidden="true"></span>{{ isNewDocument && !form.id ? '新建草稿' : '编辑文档' }}</span>
        </div>
        <div class="editor-document-paper">
          <div class="editor-paper-heading"><span>DOCUMENT EDITOR</span><span>Markdown · Tiptap</span></div>
          <ContentDocumentEditor v-model="form.body" workspace :disabled="busy || loading">
            <template #workspace-actions>
              <div class="editor-properties-heading"><span class="bi bi-sliders2" aria-hidden="true"></span><strong>文档属性</strong></div>
              <section class="editor-properties">
                <label><span>文档标题 <i>*</i></span><input v-model="form.title" maxlength="200" autocomplete="off" placeholder="为这篇文档命名" /></label>
                <label><span>URL 别名 <i>*</i></span><input v-model="form.slug" maxlength="100" autocomplete="off" spellcheck="false" placeholder="document-slug" @input="slugTouched = true" /><small :class="{ 'is-invalid': form.slug && !slugIsValid }">仅支持小写英文、数字与连字符</small></label>
                <label><span>内容类型</span><select v-model="form.contentType"><option v-for="item in contentTypes" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
                <label><span>可见范围</span><select v-model="form.visibility"><option value="private">私有 · 仅自己可见</option><option value="public">公开 · 可供访问</option></select></label>
                <label><span>摘要 <small>可选</small></span><textarea v-model="form.summary" rows="3" maxlength="500" placeholder="用一两句话介绍文档内容"></textarea><small class="editor-character-count">{{ form.summary.length }}/500</small></label>
              </section>

              <section class="editor-tag-section">
                <div class="editor-tag-heading"><strong>知识标签</strong><span>{{ form.tags.length }}/40</span></div>
                <p class="editor-tag-help">至少选择一个标签。点击分类可展开子标签。</p>
                <details v-for="group in tagGroups" :key="group.value" class="editor-tag-group">
                  <summary><span>{{ group.value }}</span><span class="editor-tag-count">{{ group.children.filter(tag => form.tags.includes(tag)).length }}/{{ group.children.length }}</span><input type="checkbox" :checked="groupState(group.value)" :aria-label="'选择全部' + group.value" @click.stop @change="setGroup(group.value, ($event.target as HTMLInputElement).checked)" /></summary>
                  <div class="editor-tag-options">
                    <label v-for="tag in group.children" :key="tag" :class="{ 'is-selected': form.tags.includes(tag) }">
                      <input type="checkbox" :checked="form.tags.includes(tag)" @change="toggleTag(tag)" />
                      <span>{{ tag }}</span>
                    </label>
                  </div>
                </details>
                <div v-if="form.tags.length" class="editor-selected-tags"><span v-for="tag in form.tags" :key="tag">{{ tag }}<button type="button" :aria-label="'移除标签 ' + tag" @click="toggleTag(tag)">×</button></span></div>
              </section>
              <div class="editor-private-note"><span class="bi bi-shield-lock" aria-hidden="true"></span><span>内容通过同源 Go API 保存。新建文档默认私有，不会自动发布。</span></div>
            </template>
          </ContentDocumentEditor>
        </div>
      </section>
    </section>
  </main>
</template>
