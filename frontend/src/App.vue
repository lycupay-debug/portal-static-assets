<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { renderMarkdown } from './utils/markdown'

type PortalSection = {
  id: string
  number: string
  title: string
  subtitle: string
  description: string
  tags: string[]
}
type PortalUser = { id: string; email: string; displayName: string; createdAt: string }
type PortalContent = {
  id: string
  title: string
  slug: string
  contentType: 'article' | 'tool' | 'resource' | 'project'
  summary: string
  body: string
  visibility: 'public' | 'private'
  status: 'draft' | 'published' | 'unpublished'
  createdAt: string
  updatedAt: string
  publishedAt: string | null
}
type ContentVersion = { versionId: string; versionNumber: number; title: string; slug: string; status: PortalContent['status']; createdAt: string }
type ContentForm = {
  title: string
  slug: string
  contentType: PortalContent['contentType']
  summary: string
  body: string
  visibility: PortalContent['visibility']
}
type PublicContentCard = Pick<PortalContent, 'id' | 'title' | 'slug' | 'contentType' | 'summary' | 'publishedAt'>
type PublicContent = PublicContentCard & Pick<PortalContent, 'body' | 'createdAt' | 'updatedAt'>

const query = ref('')
const menuOpen = ref(false)
const route = ref(window.location.pathname || '/')
const user = ref<PortalUser | null>(null)
const authLoading = ref(false)
const authBusy = ref(false)
const authError = ref('')
const authNotice = ref('')
const authForm = ref({ displayName: '', email: '', password: '' })
const expandedAccountGroups = ref<string[]>(['personal', 'publishing', 'housing', 'education', 'software', 'workflows'])
const activeAccountSection = ref('overview')
const activeSidebarButton = ref('overview')
const accountNavQuery = ref('')
const accountGroups = [
  {
    id: 'personal', label: '个人中心', items: [
      { id: 'overview', label: '账户概览', description: '查看账户状态与基础信息。' },
      { id: 'profile', label: '个人资料', description: '查看当前账户的昵称和邮箱信息。' },
      { id: 'security', label: '登录与安全', description: '查看当前登录状态与账户安全信息。' },
    ],
  },
  {
    id: 'publishing', label: '发布中心', items: [
      { id: 'content-management', label: '内容管理', description: '管理发布内容、草稿与内容状态。' },
    ],
  },
  {
    id: 'housing', label: '房屋租赁中心', items: [
      { id: 'property-management', label: '房源管理', description: '集中管理房源信息与租赁资料。' },
      { id: 'room-status', label: '房态管理', description: '查看和维护房间空置、入住与出租状态。' },
    ],
  },
  {
    id: 'education', label: '教育业务中心', items: [
      { id: 'student-records', label: '学生档案', description: '管理学生基本资料与学习档案。' },
      { id: 'course-calendar', label: '课程日历', description: '查看和安排课程时间。' },
      { id: 'teaching-resources', label: '教学资源', description: '整理和管理教学资料。' },
    ],
  },
  {
    id: 'software', label: '软件工具', items: [
      { id: 'network-tools', label: '网络工具', description: '集中收纳网络检测、查询与诊断工具。' },
      { id: 'document-converters', label: '文档转换工具', description: '集中收纳常用文档格式转换工具。' },
      { id: 'add-tool', label: '添加工具', description: '添加并整理新的软件工具入口。' },
    ],
  },
  {
    id: 'workflows', label: '工作流管理', items: [
      { id: 'workflow-overview', label: '工作流总览', description: '集中查看和管理自动化工作流。' },
    ],
  },
]
const accountItems = accountGroups.flatMap((group) => group.items)
const filteredAccountGroups = computed(() => {
  const keyword = accountNavQuery.value.trim().toLocaleLowerCase()
  if (!keyword) return accountGroups
  return accountGroups
    .map((group) => ({ ...group, items: group.items.filter((item) => `${group.label} ${item.label} ${item.description}`.toLocaleLowerCase().includes(keyword)) }))
    .filter((group) => group.label.toLocaleLowerCase().includes(keyword) || group.items.length > 0)
})
const activeAccountItem = computed(() => accountItems.find((item) => item.id === activeAccountSection.value))
function toggleAccountGroup(groupId: string) {
  expandedAccountGroups.value = expandedAccountGroups.value.includes(groupId)
    ? expandedAccountGroups.value.filter((id) => id !== groupId)
    : [...expandedAccountGroups.value, groupId]
}
function accountGroupIcon(groupId: string): string {
  const icons: Record<string, string> = {
    personal: '♙', publishing: '▤', housing: '⌂',
    education: '▧', software: '⌘', workflows: '⑂',
  }
  return icons[groupId] || '◇'
}


type TableColumn = { key: string; label: string; width?: string }
type TableRecord = Record<string, string | number>
const tableQuery = ref('')
const tableScrollTop = ref(0)
const contentItems = ref<PortalContent[]>([])
const contentTrashItems = ref<PortalContent[]>([])
const contentViewMode = ref<'active' | 'trash'>('active')
const contentVersions = ref<ContentVersion[]>([])
const versionsContentId = ref<string | null>(null)
const versionsContentTitle = ref('')
const contentLoading = ref(false)
const contentBusy = ref(false)
const contentEditorOpen = ref(false)
const editingContentId = ref<string | null>(null)
const contentError = ref('')
const contentNotice = ref('')
const emptyContentForm = (): ContentForm => ({
  title: '', slug: '', contentType: 'article', summary: '', body: '', visibility: 'private',
})
const contentForm = ref<ContentForm>(emptyContentForm())
const renderedContentPreview = computed(() => renderMarkdown(contentForm.value.body))
const publicContents = ref<PublicContentCard[]>([])
const publicArticle = ref<PublicContent | null>(null)
const publicContentLoading = ref(false)
const publicContentError = ref('')
const virtualTableRowHeight = 44
const virtualTableViewportHeight = 440
const virtualTableOverscan = 6
const tableDefinitions: Record<string, { title: string; columns: TableColumn[]; note: string }> = {
  'content-management': { title: '内容列表', columns: [{ key: 'title', label: '内容标题', width: 'minmax(220px, 2fr)' }, { key: 'type', label: '类型' }, { key: 'status', label: '状态' }, { key: 'visibility', label: '可见范围', width: 'minmax(120px, 1fr)' }, { key: 'updatedAt', label: '更新时间' }, { key: 'actions', label: '操作', width: 'minmax(330px, 2.6fr)' }], note: '内容数据源尚未接入。' },
  'property-management': { title: '房源列表', columns: [{ key: 'property', label: '房源名称', width: 'minmax(180px, 1.6fr)' }, { key: 'address', label: '地址', width: 'minmax(180px, 2fr)' }, { key: 'roomCount', label: '房间数' }, { key: 'rent', label: '租金' }, { key: 'status', label: '房源状态' }, { key: 'updatedAt', label: '更新时间' }], note: '房源数据源尚未接入。' },
  'room-status': { title: '房态列表', columns: [{ key: 'room', label: '房间', width: 'minmax(140px, 1.3fr)' }, { key: 'property', label: '所属房源', width: 'minmax(180px, 1.7fr)' }, { key: 'status', label: '当前房态' }, { key: 'tenant', label: '租客' }, { key: 'checkIn', label: '入住日期' }, { key: 'checkOut', label: '退租日期' }], note: '房态数据源尚未接入。' },
  'student-records': { title: '学生档案', columns: [{ key: 'name', label: '学生姓名' }, { key: 'studentId', label: '学员编号' }, { key: 'course', label: '课程' }, { key: 'status', label: '学习状态' }, { key: 'updatedAt', label: '更新时间' }], note: '学生档案数据源尚未接入。' },
  'course-calendar': { title: '课程安排', columns: [{ key: 'course', label: '课程名称', width: 'minmax(180px, 1.6fr)' }, { key: 'class', label: '班级' }, { key: 'teacher', label: '教师' }, { key: 'startAt', label: '开始时间' }, { key: 'endAt', label: '结束时间' }, { key: 'status', label: '状态' }], note: '课程日历数据源尚未接入。' },
  'teaching-resources': { title: '教学资源', columns: [{ key: 'title', label: '资源名称', width: 'minmax(200px, 2fr)' }, { key: 'type', label: '资源类型' }, { key: 'course', label: '关联课程' }, { key: 'updatedAt', label: '更新时间' }, { key: 'actions', label: '操作' }], note: '教学资源数据源尚未接入。' },
  'network-tools': { title: '网络工具', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'description', label: '功能说明', width: 'minmax(220px, 2fr)' }, { key: 'url', label: '入口地址' }, { key: 'status', label: '状态' }], note: '网络工具目录尚未接入。' },
  'document-converters': { title: '文档转换工具', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'sourceFormat', label: '源格式' }, { key: 'targetFormat', label: '目标格式' }, { key: 'status', label: '状态' }, { key: 'actions', label: '操作' }], note: '文档转换工具目录尚未接入。' },
  'add-tool': { title: '工具清单', columns: [{ key: 'name', label: '工具名称', width: 'minmax(180px, 1.5fr)' }, { key: 'category', label: '分类' }, { key: 'url', label: '入口地址', width: 'minmax(220px, 2fr)' }, { key: 'status', label: '状态' }, { key: 'createdAt', label: '添加时间' }], note: '当前还没有接入工具管理数据源。' },
  'workflow-overview': { title: '工作流列表', columns: [{ key: 'name', label: '工作流名称', width: 'minmax(200px, 1.8fr)' }, { key: 'platform', label: '运行平台' }, { key: 'status', label: '运行状态' }, { key: 'lastRun', label: '最近运行' }, { key: 'result', label: '最近结果' }], note: '工作流数据源尚未接入。' },
}
const activeTableDefinition = computed(() => tableDefinitions[activeAccountSection.value] || null)
const contentTypeLabels: Record<PortalContent['contentType'], string> = {
  article: '知识文章', tool: '在线工具', resource: '资源资料', project: '项目展示',
}
const contentStatusLabels: Record<PortalContent['status'], string> = {
  draft: '草稿', published: '已发布', unpublished: '已下架',
}
const tableRecords = computed<TableRecord[]>(() => activeAccountSection.value === 'content-management'
  ? (contentViewMode.value === 'trash' ? contentTrashItems.value : contentItems.value).map((item) => ({
      id: item.id,
      title: item.title,
      type: contentTypeLabels[item.contentType],
      status: contentStatusLabels[item.status],
      visibility: item.visibility === 'public' ? '公开' : '仅自己可见',
      updatedAt: new Date(contentViewMode.value === 'trash' && 'deletedAt' in item ? String(item.deletedAt) : item.updatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' }),
      actions: contentViewMode.value === 'trash' ? '已删除' : item.status === 'published' ? '已发布' : '待发布',
    }))
  : [])
const filteredTableRecords = computed(() => {
  const keyword = tableQuery.value.trim().toLocaleLowerCase()
  if (!keyword) return tableRecords.value
  return tableRecords.value.filter((record) => Object.values(record).some((value) => String(value).toLocaleLowerCase().includes(keyword)))
})
const virtualTableStart = computed(() => Math.max(0, Math.floor(tableScrollTop.value / virtualTableRowHeight) - virtualTableOverscan))
const virtualTableEnd = computed(() => Math.min(filteredTableRecords.value.length, Math.ceil((tableScrollTop.value + virtualTableViewportHeight) / virtualTableRowHeight) + virtualTableOverscan))
const visibleTableRecords = computed(() => filteredTableRecords.value.slice(virtualTableStart.value, virtualTableEnd.value))
const virtualTableTopSpace = computed(() => virtualTableStart.value * virtualTableRowHeight)
const virtualTableBottomSpace = computed(() => Math.max(0, (filteredTableRecords.value.length - virtualTableEnd.value) * virtualTableRowHeight))
function onVirtualTableScroll(event: Event) {
  tableScrollTop.value = (event.target as HTMLElement).scrollTop
}
async function loadContentList() {
  contentLoading.value = true
  contentError.value = ''
  try {
    const isTrash = contentViewMode.value === 'trash'
    const response = await fetch(isTrash ? '/api/content/trash' : '/api/content', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: PortalContent[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取内容列表失败。')
    if (isTrash) contentTrashItems.value = result.items || []
    else contentItems.value = result.items || []
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '网络连接失败，无法读取内容。'
  } finally {
    contentLoading.value = false
  }
}
function switchContentView(mode: 'active' | 'trash') {
  contentViewMode.value = mode
  versionsContentId.value = null
  tableQuery.value = ''
  void loadContentList()
}
async function openContentVersions(id: string) {
  const item = contentItems.value.find((content) => content.id === id)
  if (!item) return
  contentBusy.value = true
  contentError.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}/versions`, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: ContentVersion[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取版本历史失败。')
    contentVersions.value = result.items || []
    versionsContentId.value = id
    versionsContentTitle.value = item.title
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '读取版本历史失败。'
  } finally { contentBusy.value = false }
}
async function restoreContentVersion(version: ContentVersion) {
  const id = versionsContentId.value
  if (!id || !window.confirm(`将“${versionsContentTitle.value}”恢复到版本 ${version.versionNumber}？当前内容会自动保存为一个历史版本。`)) return
  contentBusy.value = true
  contentError.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}/versions/${encodeURIComponent(version.versionId)}/restore`, { method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { error?: string }
    if (!response.ok) throw new Error(result.error || '恢复历史版本失败。')
    versionsContentId.value = null
    contentNotice.value = `已恢复到版本 ${version.versionNumber}。恢复前的内容已保存到版本历史。`
    await loadContentList()
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '恢复历史版本失败。'
  } finally { contentBusy.value = false }
}
async function restoreDeletedContent(id: string) {
  const item = contentTrashItems.value.find((content) => content.id === id)
  if (!item || !window.confirm(`将“${item.title}”从回收站恢复？`)) return
  contentBusy.value = true
  contentError.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}/restore`, { method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { error?: string }
    if (!response.ok) throw new Error(result.error || '恢复内容失败。')
    contentNotice.value = `“${item.title}”已恢复。`
    await loadContentList()
  } catch (error) { contentError.value = error instanceof Error ? error.message : '恢复内容失败。' }
  finally { contentBusy.value = false }
}
async function permanentlyDeleteContent(id: string) {
  const item = contentTrashItems.value.find((content) => content.id === id)
  if (!item || !window.confirm(`永久删除“${item.title}”？此操作不可撤销，相关版本历史也会一并删除。`)) return
  contentBusy.value = true
  contentError.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}/permanent`, { method: 'DELETE', credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { error?: string }
    if (!response.ok) throw new Error(result.error || '永久删除失败。')
    contentNotice.value = `“${item.title}”已永久删除。`
    await loadContentList()
  } catch (error) { contentError.value = error instanceof Error ? error.message : '永久删除失败。' }
  finally { contentBusy.value = false }
}
function selectAccountSection(id: string) {
  activeAccountSection.value = id
  activeSidebarButton.value = id
  tableQuery.value = ''
  contentError.value = ''
  contentNotice.value = ''
  contentEditorOpen.value = false
  versionsContentId.value = null
  contentViewMode.value = 'active'
  if (id === 'content-management') void loadContentList()
}
function openNewContent() {
  editingContentId.value = null
  contentForm.value = emptyContentForm()
  contentError.value = ''
  contentNotice.value = ''
  contentEditorOpen.value = true
}
async function editContent(id: string) {
  contentBusy.value = true
  contentError.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}`, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { item?: PortalContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '读取内容详情失败。')
    const item = result.item
    editingContentId.value = item.id
    contentForm.value = {
      title: item.title, slug: item.slug, contentType: item.contentType,
      summary: item.summary, body: item.body, visibility: item.visibility,
    }
    contentEditorOpen.value = true
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '读取内容详情失败。'
  } finally {
    contentBusy.value = false
  }
}
async function saveContent() {
  contentBusy.value = true
  contentError.value = ''
  contentNotice.value = ''
  try {
    const editing = editingContentId.value
    const response = await fetch(editing ? `/api/content/${encodeURIComponent(editing)}` : '/api/content', {
      method: editing ? 'PATCH' : 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(contentForm.value),
    })
    const result = await response.json() as { item?: PortalContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '保存内容失败。')
    contentEditorOpen.value = false
    contentNotice.value = editing ? '内容已保存。' : '草稿已创建。'
    await loadContentList()
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '保存内容失败。'
  } finally {
    contentBusy.value = false
  }
}
async function changeContentStatus(id: string, action: 'publish' | 'unpublish') {
  contentBusy.value = true
  contentError.value = ''
  contentNotice.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}/${action}`, {
      method: 'POST', credentials: 'same-origin', headers: { Accept: 'application/json' },
    })
    const result = await response.json() as { item?: PortalContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '更新内容状态失败。')
    contentNotice.value = action === 'publish' ? '内容状态已更新为已发布。' : '内容已下架。'
    await loadContentList()
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '更新内容状态失败。'
  } finally {
    contentBusy.value = false
  }
}
async function deleteContent(id: string) {
  const item = contentItems.value.find((content) => content.id === id)
  if (!item || !window.confirm(`确定将“${item.title}”移入删除状态吗？`)) return
  contentBusy.value = true
  contentError.value = ''
  contentNotice.value = ''
  try {
    const response = await fetch(`/api/content/${encodeURIComponent(id)}`, {
      method: 'DELETE', credentials: 'same-origin', headers: { Accept: 'application/json' },
    })
    const result = await response.json() as { error?: string }
    if (!response.ok) throw new Error(result.error || '删除内容失败。')
    contentNotice.value = '内容已移入删除状态。'
    await loadContentList()
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '删除内容失败。'
  } finally {
    contentBusy.value = false
  }
}

const sections: PortalSection[] = [
  { id: 'knowledge', number: '01', title: '知识文章', subtitle: 'KNOWLEDGE', description: '沉淀经过整理的思考、方法与实践记录，让知识可以被持续复用。', tags: ['思考与认知', '方法论', '实践复盘'] },
  { id: 'tools', number: '02', title: '在线工具', subtitle: 'TOOLS', description: '把重复步骤变成简单工具，减少操作成本，让精力回到真正重要的问题。', tags: ['效率工具', '文本处理', '数据辅助'] },
  { id: 'resources', number: '03', title: '资源资料', subtitle: 'RESOURCES', description: '按主题组织值得留存的资料、参考链接与学习入口。', tags: ['参考资料', '学习清单', '常用链接'] },
  { id: 'projects', number: '04', title: '项目展示', subtitle: 'PROJECTS', description: '记录正在构建的系统、已完成的实践，以及过程中的关键决策。', tags: ['项目记录', '技术实践', '构建日志'] },
]
const filteredSections = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  if (!keyword) return sections
  return sections.filter((section) => [section.title, section.subtitle, section.description, ...section.tags].join(' ').toLocaleLowerCase().includes(keyword))
})
const isRegister = computed(() => route.value === '/register')
const isAuthPage = computed(() => route.value === '/login' || route.value === '/register')
const isPublicContentPage = computed(() => /^\/content\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value))

function closeMenu() { menuOpen.value = false }
function setRouteFromLocation() { route.value = window.location.pathname || '/' }
async function loadPublicContents() {
  publicContentLoading.value = true
  publicContentError.value = ''
  try {
    const response = await fetch('/api/public/content', { headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: PublicContentCard[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取公开内容失败。')
    publicContents.value = result.items || []
  } catch (error) {
    publicContentError.value = error instanceof Error ? error.message : '读取公开内容失败。'
  } finally {
    publicContentLoading.value = false
  }
}
async function loadPublicArticle(slug: string) {
  publicContentLoading.value = true
  publicContentError.value = ''
  publicArticle.value = null
  try {
    const response = await fetch(`/api/public/content/${encodeURIComponent(slug)}`, { headers: { Accept: 'application/json' } })
    const result = await response.json() as { item?: PublicContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '内容不存在或尚未公开。')
    publicArticle.value = result.item
  } catch (error) {
    publicContentError.value = error instanceof Error ? error.message : '读取内容失败。'
  } finally {
    publicContentLoading.value = false
  }
}
async function navigate(target: string) {
  closeMenu()
  history.pushState({}, '', target)
  setRouteFromLocation()
  authError.value = ''
  authNotice.value = ''
  if (route.value === '/account') {
    const valid = await loadSession()
    if (!valid) {
      history.replaceState({}, '', '/login')
      route.value = '/login'
      authNotice.value = '请先登录，再进入个人中心。'
      return
    }
  }
  if (route.value === '/') void loadPublicContents()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (route.value === '/' && window.location.hash) {
    await nextTick()
    document.querySelector(window.location.hash)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  } else {
    window.scrollTo({ top: 0, behavior: 'auto' })
  }
}
async function loadSession(): Promise<boolean> {
  authLoading.value = true
  try {
    const response = await fetch('/api/auth/me', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    if (!response.ok) { user.value = null; return false }
    const result = await response.json() as { user: PortalUser }
    user.value = result.user
    return true
  } catch {
    user.value = null
    return false
  } finally { authLoading.value = false }
}
async function submitAuth() {
  authBusy.value = true
  authError.value = ''
  authNotice.value = ''
  try {
    const endpoint = isRegister.value ? '/api/auth/register' : '/api/auth/login'
    const payload = isRegister.value
      ? { displayName: authForm.value.displayName.trim(), email: authForm.value.email.trim(), password: authForm.value.password }
      : { email: authForm.value.email.trim(), password: authForm.value.password }
    const response = await fetch(endpoint, {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(payload),
    })
    const result = await response.json() as { user?: PortalUser; error?: string }
    if (!response.ok || !result.user) throw new Error(result.error || '操作失败，请稍后再试。')
    user.value = result.user
    authForm.value.password = ''
    await navigate('/account')
  } catch (error) {
    authError.value = error instanceof Error ? error.message : '网络连接失败，请稍后再试。'
  } finally { authBusy.value = false }
}
async function logout() {
  authBusy.value = true
  try {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' })
  } finally {
    user.value = null
    authBusy.value = false
    await navigate('/')
    authNotice.value = '已安全退出登录。'
  }
}
function onPopState() {
  setRouteFromLocation()
  closeMenu()
  authError.value = ''
  if (route.value === '/') void loadPublicContents()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (route.value === '/account') void loadSession().then((valid) => { if (!valid) void navigate('/login') })
}
onMounted(() => {
  window.addEventListener('popstate', onPopState)
  if (route.value === '/') void loadPublicContents()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  void loadSession().then((valid) => {
    if (route.value === '/account' && !valid) {
      history.replaceState({}, '', '/login')
      route.value = '/login'
      authNotice.value = '请先登录，再进入个人中心。'
    }
  })
})
onBeforeUnmount(() => window.removeEventListener('popstate', onPopState))
</script>

<template>
  <div class="site-shell">
    <header v-if="route !== '/account'" class="site-header">
      <a class="brand" href="/" aria-label="个人门户首页" @click.prevent="navigate('/')">
        <span class="brand-mark">P.</span>
        <span class="brand-copy"><strong>PERSONAL PORTAL</strong><small>知识 · 工具 · 实践</small></span>
      </a>
      <button class="menu-toggle" type="button" :aria-expanded="menuOpen" aria-label="切换导航菜单" @click="menuOpen = !menuOpen">
       <span></span>
      </button>
      <nav class="main-nav" :class="{ 'is-open': menuOpen }" aria-label="主导航">
        <a href="/#explore" @click.prevent="navigate('/#explore')">探索</a>
        <a href="/#about" @click.prevent="navigate('/#about')">关于本站</a>
        <a class="nav-contact" href="/#explore" @click.prevent="navigate('/#explore')">内容索引</a>
        <a v-if="user" class="nav-auth" href="/account" @click.prevent="navigate('/account')">个人中心</a>
        <a v-else class="nav-auth" href="/login" @click.prevent="navigate('/login')">登录 / 注册</a>
      </nav>
    </header>

    <main v-if="route === '/'" id="top">
      <section class="hero">
        <div class="hero-copy">
          <p class="eyebrow"><span class="status-dot"></span> A PERSONAL KNOWLEDGE SPACE</p>
          <h1>把想法整理好，<br /><span>让行动有迹可循。</span></h1>
          <p class="hero-description">一个持续生长的个人门户，用来整理知识、构建工具、归档资源，并记录从想法到实践的过程。</p>
          <div class="hero-actions"><a class="button button-primary" href="/#explore" @click.prevent="navigate('/#explore')">开始探索</a><a class="text-link" href="/#about" @click.prevent="navigate('/#about')">了解这个空间</a></div>
          <div class="hero-note"><span class="note-line"></span><span>持续整理 · 持续构建 · 持续迭代</span></div>
        </div>
        <div class="hero-art" aria-label="由层叠卡片构成的抽象知识结构插图" role="img">
          <div class="art-orbit orbit-one"></div><div class="art-orbit orbit-two"></div>
          <div class="art-card card-back"><span class="art-card-label">FIELD NOTES</span><span class="art-lines"><i></i><i></i><i></i></span></div>
          <div class="art-card card-front"><span class="art-card-label">THOUGHT  ACTION</span><div class="art-symbol">P<span>.</span></div><div class="art-card-footer"><span>COLLECT</span><span>CONNECT</span><span>CREATE</span></div></div>
          <span class="art-index">FIG. 001 — AN OPEN SYSTEM</span><span class="art-spark spark-one">✳</span><span class="art-spark spark-two">＋</span>
        </div>
        <div class="hero-bottom"><span>独立构建的个人空间</span><span>01 / 04 <i></i> PUBLIC SECTIONS</span></div>
      </section>
      <section id="explore" class="explore-section">
        <div class="section-heading"><div><p class="eyebrow">EXPLORE THE PORTAL <span class="heading-rule"></span></p><h2>从一个方向开始。</h2></div><p class="section-intro">四个入口，承载不同类型的内容。<br />结构先行，内容逐步积累。</p></div>
        <label class="search-box"><span class="search-icon" aria-hidden="true">⌕</span><input v-model="query" type="search" placeholder="搜索文章、工具、资源或项目分类…" aria-label="搜索门户分类" /><kbd>⌘ K</kbd></label>
        <div v-if="filteredSections.length" class="section-grid">
          <article v-for="section in filteredSections" :key="section.id" class="section-card">
            <div class="card-topline"><span>{{ section.number }}</span><span class="card-arrow">↗</span></div><p class="card-kicker">{{ section.subtitle }}</p><h3>{{ section.title }}</h3><p class="card-description">{{ section.description }}</p>
            <div class="tag-list"><span v-for="tag in section.tags" :key="tag">{{ tag }}</span></div><div class="card-bottom"><span>内容逐步整理中</span><span>↗</span></div>
          </article>
        </div>
        <div v-else class="empty-search"><strong>没有匹配的分类</strong><p>试试“知识”“工具”“资源”或“项目”。</p><button type="button" @click="query = ''">清除搜索</button></div>
      </section>
      <section v-if="publicContents.length || publicContentLoading" class="published-content-section">
        <div class="published-content-heading"><div><p class="eyebrow">LATEST PUBLICATIONS</p><h2>最新发布</h2></div><span>{{ publicContents.length }} 篇公开内容</span></div>
        <p v-if="publicContentError" class="published-content-empty">{{ publicContentError }}</p>
        <div v-else-if="publicContentLoading && !publicContents.length" class="published-content-empty">正在读取公开内容…</div>
        <div v-else class="published-content-grid">
          <a v-for="item in publicContents" :key="item.id" class="published-content-card" :href="`/content/${item.slug}`" @click.prevent="navigate(`/content/${item.slug}`)">
            <span class="published-content-type">{{ contentTypeLabels[item.contentType] }}</span>
            <h3>{{ item.title }}</h3>
            <p>{{ item.summary || '打开内容查看详情。' }}</p>
            <span class="published-content-link">阅读内容 ↗</span>
          </a>
        </div>
      </section>
      <section id="about" class="about-section"><div class="about-mark">P.</div><div class="about-copy"><p class="eyebrow">ABOUT THIS SPACE</p><h2>不是内容的终点，<br />而是持续实践的起点。</h2><p>这里将逐步收录公开的知识文章、在线工具、参考资料与项目记录。内容会在经过整理后发布；私人工作区与公开页面保持分离。</p></div><div class="about-meta"><span>DESIGNED TO GROW</span><span>Built with intention.</span></div></section>
    </main>

    <main v-else-if="isAuthPage" class="route-main">
      <section class="auth-panel glass-panel">
        <p class="eyebrow"><span class="status-dot"></span> PERSONAL PORTAL ACCOUNT</p>
        <h1>{{ isRegister ? '创建你的账户' : '欢迎回来' }}</h1>
        <p class="auth-intro">{{ isRegister ? '注册后即可进入个人中心。你的账户信息将安全保存。' : '登录后进入个人中心，管理你的账户信息。' }}</p>
        <p v-if="authNotice" class="form-notice">{{ authNotice }}</p>
        <form class="auth-form" @submit.prevent="submitAuth">
          <label v-if="isRegister" class="field-label">昵称 <input v-model="authForm.displayName" type="text" autocomplete="nickname" maxlength="60" placeholder="怎么称呼你" /></label>
          <label class="field-label">邮箱地址 <input v-model="authForm.email" type="email" autocomplete="email" maxlength="254" required placeholder="name@example.com" /></label>
          <label class="field-label">密码 <input v-model="authForm.password" type="password" :autocomplete="isRegister ? 'new-password' : 'current-password'" :minlength="isRegister ? 10 : 1" maxlength="72" required :placeholder="isRegister ? '至少 10 个字符' : '输入你的密码'" /></label>
          <p v-if="isRegister" class="field-help">密码长度为 10–72 个字符。请勿重复使用其他网站的密码。</p>
          <p v-if="authError" class="form-error" role="alert">{{ authError }}</p>
          <button class="button button-primary auth-submit" type="submit" :disabled="authBusy">{{ authBusy ? '正在处理…' : (isRegister ? '创建账户' : '登录') }}</button>
        </form>
        <p class="auth-switch">{{ isRegister ? '已经有账户？' : '还没有账户？' }} <a :href="isRegister ? '/login' : '/register'" @click.prevent="navigate(isRegister ? '/login' : '/register')">{{ isRegister ? '立即登录' : '创建账户' }}</a></p>
        <a class="back-home" href="/" @click.prevent="navigate('/')">← 返回首页</a>
      </section>
    </main>

    <main v-else-if="isPublicContentPage" class="public-content-page">
      <section v-if="publicContentLoading" class="public-content-state">正在读取内容…</section>
      <section v-else-if="publicArticle" class="public-article">
        <a class="public-article-back" href="/" @click.prevent="navigate('/')">← 返回首页</a>
        <p class="public-article-type">{{ contentTypeLabels[publicArticle.contentType] }} <span>·</span> {{ publicArticle.publishedAt ? new Date(publicArticle.publishedAt).toLocaleDateString('zh-CN') : '' }}</p>
        <h1>{{ publicArticle.title }}</h1>
        <p v-if="publicArticle.summary" class="public-article-summary">{{ publicArticle.summary }}</p>
        <div class="public-article-body markdown-rendered" v-html="renderMarkdown(publicArticle.body)"></div>
      </section>
      <section v-else class="public-content-state"><h1>内容暂不可用</h1><p>{{ publicContentError || '内容不存在或尚未公开。' }}</p><a class="button button-primary" href="/" @click.prevent="navigate('/')">返回首页</a></section>
    </main>

    <main v-else-if="route === '/account'" class="account-layout">

      <aside class="account-sidebar">
        <div v-for="group in filteredAccountGroups" :key="group.id" class="account-nav-group">
          <button
            type="button"
            class="account-sidebar-item account-group-toggle"
            :class="{ 'is-active': activeSidebarButton === group.id }"
            :aria-expanded="expandedAccountGroups.includes(group.id)"
            @click="activeSidebarButton = group.id; toggleAccountGroup(group.id)"
          >
            <span class="sidebar-group-icon" aria-hidden="true">{{ accountGroupIcon(group.id) }}</span>{{ group.label }}
          </button>
          <div v-show="accountNavQuery.trim() || expandedAccountGroups.includes(group.id)" class="account-sidebar-collapse">
            <button
              v-for="item in group.items"
              :key="item.id"
              type="button"
              class="account-sidebar-item account-sidebar-subitem"
              :class="{ 'is-active': activeSidebarButton === item.id }"
              @click="selectAccountSection(item.id)"
            >
              <span class="sidebar-item-dot"></span>{{ item.label }}
            </button>
          </div>
        </div>
        <p v-if="accountNavQuery.trim() && !filteredAccountGroups.length" class="account-search-empty">没有找到匹配的入口</p>
        <a class="account-sidebar-home" href="/" @click.prevent="navigate('/')">返回首页</a>
      </aside>

      <section class="account-content" :class="{ 'account-content-table-mode': !!activeTableDefinition }">
        <template v-if="activeAccountSection === 'overview'">
          <h1 class="account-page-title">账户概览</h1>
          <p class="auth-intro">查看你的账户状态与基础信息。</p>
          <div v-if="authLoading" class="account-loading">正在读取账户信息…</div>
          <div v-else-if="user" class="account-details">
            <div class="account-summary-card">
              <div class="account-avatar">{{ (user.displayName || user.email).slice(0, 1).toUpperCase() }}</div>
              <div><strong class="account-summary-name">{{ user.displayName || '未设置昵称' }}</strong><span class="account-summary-email">{{ user.email }}</span></div>
              <span class="account-status"><i></i> 已登录</span>
            </div>
            <div class="account-field"><span>账户昵称</span><strong>{{ user.displayName || '未设置' }}</strong></div>
            <div class="account-field"><span>注册时间</span><strong>{{ new Date(user.createdAt).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) }}</strong></div>
            <button class="button button-primary auth-submit" type="button" :disabled="authBusy" @click="logout">{{ authBusy ? '正在退出…' : '退出登录' }}</button>
          </div>
        </template>

        <template v-else-if="activeAccountSection === 'profile'">
          <h1 class="account-page-title">个人资料</h1>
          <p class="auth-intro">当前账户的基本资料。</p>
          <div v-if="authLoading" class="account-loading">正在读取账户信息…</div>
          <div v-else-if="user" class="account-details">
            <div class="account-summary-card">
              <div class="account-avatar">{{ (user.displayName || user.email).slice(0, 1).toUpperCase() }}</div>
              <div><strong class="account-summary-name">{{ user.displayName || '未设置昵称' }}</strong><span class="account-summary-email">{{ user.email }}</span></div>
            </div>
            <div class="account-field"><span>昵称</span><strong>{{ user.displayName || '未设置' }}</strong></div>
            <div class="account-field"><span>登录邮箱</span><strong>{{ user.email }}</strong></div>
            <div class="account-field"><span>注册时间</span><strong>{{ new Date(user.createdAt).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) }}</strong></div>
          </div>
        </template>

        <template v-else-if="activeAccountSection === 'security'">
          <h1 class="account-page-title">登录与安全</h1>
          <p class="auth-intro">账户登录状态和会话管理。</p>
          <div v-if="authLoading" class="account-loading">正在读取账户信息…</div>
          <div v-else-if="user" class="account-details">
            <div class="security-info-row"><span class="security-info-icon">✓</span><div><strong>账户已验证登录</strong><p>当前浏览器持有安全会话。</p></div></div>
            <div class="account-field"><span>登录邮箱</span><strong>{{ user.email }}</strong></div>
            <div class="account-field"><span>会话有效期</span><strong>系统自动管理</strong></div>
            <button class="button button-primary auth-submit" type="button" :disabled="authBusy" @click="logout">{{ authBusy ? '正在退出…' : '退出当前登录' }}</button>
          </div>
        </template>

        <template v-else>
          <h1 class="account-page-title">{{ activeAccountItem?.label || '业务中心' }}</h1>
          <p class="auth-intro">{{ activeAccountItem?.description || '管理你的业务内容与工作入口。' }}</p>
          <div v-if="activeAccountSection === 'content-management' && contentError" class="content-feedback content-feedback-error">{{ contentError }}</div>
          <div v-if="activeAccountSection === 'content-management' && contentNotice" class="content-feedback content-feedback-success">{{ contentNotice }}</div>
          <section v-if="activeAccountSection === 'content-management' && contentEditorOpen" class="content-editor-panel">
            <div class="content-editor-heading">
              <div><h2>{{ editingContentId ? '编辑内容' : '创建内容' }}</h2><p>{{ editingContentId ? '保存后会保留当前发布状态与可见范围。' : '内容以草稿形式保存；确认后可单独发布。' }}</p></div>
              <button type="button" class="content-secondary-button" :disabled="contentBusy" @click="contentEditorOpen = false">返回列表</button>
            </div>
            <form class="content-editor-form" @submit.prevent="saveContent">
              <label>内容标题<input v-model="contentForm.title" required maxlength="200" placeholder="输入标题" /></label>
              <label>URL 别名<input v-model="contentForm.slug" required maxlength="100" pattern="[a-z0-9]+(-[a-z0-9]+)*" placeholder="例如：go-language-notes" /><small>仅使用小写英文、数字和连字符。</small></label>
              <div class="content-form-row">
                <label>内容类型<select v-model="contentForm.contentType"><option value="article">知识文章</option><option value="tool">在线工具</option><option value="resource">资源资料</option><option value="project">项目展示</option></select></label>
                <label>可见范围<select v-model="contentForm.visibility"><option value="private">仅自己可见</option><option value="public">公开</option></select></label>
              </div>
              <label>内容摘要<textarea v-model="contentForm.summary" maxlength="500" rows="2" placeholder="简要说明内容（可选）"></textarea></label>
              <div class="content-markdown-workspace">
                <label class="content-markdown-source"><span>正文 Markdown</span><small>支持标题、列表、引用、链接、代码块和表格。</small><textarea v-model="contentForm.body" required maxlength="100000" rows="18" placeholder="# 文章标题\n\n在这里撰写 Markdown 正文…"></textarea></label>
                <section class="content-markdown-preview" aria-label="Markdown 实时预览"><div class="content-markdown-preview-heading">实时预览</div><div v-if="contentForm.body.trim()" class="markdown-rendered" v-html="renderedContentPreview"></div><p v-else class="content-markdown-preview-empty">输入正文后，这里会显示排版效果。</p></section>
              </div>
              <p v-if="contentError" class="content-feedback content-feedback-error">{{ contentError }}</p>
              <div class="content-editor-actions">
                <button type="button" class="content-secondary-button" :disabled="contentBusy" @click="contentEditorOpen = false">取消</button>
                <button type="submit" class="content-primary-button" :disabled="contentBusy">{{ contentBusy ? '正在保存…' : (editingContentId ? '保存修改' : '保存草稿') }}</button>
              </div>
            </form>
          </section>
          <section v-else-if="activeAccountSection === 'content-management' && versionsContentId" class="content-editor-panel content-versions-panel">
            <div class="content-editor-heading">
              <div><h2>版本历史</h2><p>“{{ versionsContentTitle }}” · 每次有效编辑前自动保存快照，最多显示最近 100 个版本；恢复只替换内容字段，保留当前发布状态。</p></div>
              <button type="button" class="content-secondary-button" :disabled="contentBusy" @click="versionsContentId = null">返回内容列表</button>
            </div>
            <div v-if="contentVersions.length" class="content-version-list">
              <div v-for="version in contentVersions" :key="version.versionId" class="content-version-row">
                <div><strong>版本 {{ version.versionNumber }} · {{ version.title }}</strong><p>{{ new Date(version.createdAt).toLocaleString('zh-CN') }} · {{ version.slug }} · {{ contentStatusLabels[version.status] || version.status }}</p></div>
                <button type="button" class="content-secondary-button" :disabled="contentBusy" @click="restoreContentVersion(version)">恢复此版本</button>
              </div>
            </div>
            <div v-else class="virtual-table-empty"><strong>暂无历史版本</strong><p>保存第一次有效修改后，这里会出现可恢复的历史版本。</p></div>
          </section>
          <section v-else-if="activeTableDefinition" class="virtual-table-panel">
            <div class="virtual-table-toolbar">
              <div class="virtual-table-heading"><h2>{{ activeTableDefinition.title }}</h2><span class="virtual-table-count">{{ contentLoading && activeAccountSection === 'content-management' ? '正在加载…' : filteredTableRecords.length + ' 条记录' }}</span></div>
              <div class="content-toolbar-actions">
                <input v-model="tableQuery" class="virtual-table-search" type="search" placeholder="搜索表格内容…" aria-label="搜索表格内容" @input="tableScrollTop = 0" />
                <template v-if="activeAccountSection === 'content-management'">
                  <button type="button" class="content-secondary-button" @click="switchContentView(contentViewMode === 'active' ? 'trash' : 'active')">{{ contentViewMode === 'active' ? '回收站' : '返回内容列表' }}</button>
                  <button v-if="contentViewMode === 'active'" type="button" class="content-primary-button" @click="openNewContent">新建内容</button>
                </template>
              </div>
            </div>
            <div class="virtual-table-scroll" :style="{ '--table-columns': activeTableDefinition.columns.map((column) => column.width || 'minmax(100px, 1fr)').join(' ') }" @scroll="onVirtualTableScroll">
              <div class="virtual-table-header" role="row">
                <span v-for="column in activeTableDefinition.columns" :key="column.key" role="columnheader">{{ column.label }}</span>
              </div>
              <div v-if="filteredTableRecords.length" class="virtual-table-body" role="rowgroup">
                <div class="virtual-table-spacer" :style="{ height: virtualTableTopSpace + 'px' }"></div>
                <div v-for="(record, index) in visibleTableRecords" :key="String(record.id ?? (virtualTableStart + index))" class="virtual-table-row" role="row">
                  <template v-for="column in activeTableDefinition.columns" :key="column.key">
                    <div v-if="column.key === 'actions' && activeAccountSection === 'content-management'" class="content-row-actions" role="cell">
                      <template v-if="contentViewMode === 'trash'">
                        <button type="button" :disabled="contentBusy" @click="restoreDeletedContent(String(record.id))">恢复</button>
                        <button type="button" class="content-danger-action" :disabled="contentBusy" @click="permanentlyDeleteContent(String(record.id))">永久删除</button>
                      </template>
                      <template v-else>
                        <button type="button" :disabled="contentBusy" @click="editContent(String(record.id))">编辑</button>
                        <button type="button" :disabled="contentBusy" @click="openContentVersions(String(record.id))">版本</button>
                        <button v-if="contentItems.find((item) => item.id === String(record.id))?.status !== 'published'" type="button" :disabled="contentBusy" @click="changeContentStatus(String(record.id), 'publish')">发布</button>
                        <button v-else type="button" :disabled="contentBusy" @click="changeContentStatus(String(record.id), 'unpublish')">下架</button>
                        <button type="button" class="content-danger-action" :disabled="contentBusy" @click="deleteContent(String(record.id))">删除</button>
                      </template>
                    </div>
                    <span v-else role="cell">{{ record[column.key] ?? '—' }}</span>
                  </template>
                </div>
                <div class="virtual-table-spacer" :style="{ height: virtualTableBottomSpace + 'px' }"></div>
              </div>
              <div v-else class="virtual-table-empty"><span class="virtual-table-empty-icon">▤</span><strong>{{ contentLoading && activeAccountSection === 'content-management' ? '正在读取内容' : tableQuery ? '没有匹配的记录' : '暂无数据' }}</strong><p>{{ contentLoading && activeAccountSection === 'content-management' ? '正在从数据库读取内容列表。' : tableQuery ? '尝试更换搜索关键词。' : activeAccountSection === 'content-management' ? (contentViewMode === 'trash' ? '回收站为空。已删除内容可以恢复，也可以永久删除。' : '创建第一篇内容，保存为草稿后再发布。') : activeTableDefinition.note }}</p></div>
            </div>
            <p class="virtual-table-footnote">虚拟滚动表格 · 仅渲染可视区域内的行</p>
          </section>
        </template>
      </section>
    </main>

    <main v-else class="route-main"><section class="auth-panel glass-panel"><p class="eyebrow">PAGE NOT FOUND</p><h1>页面不存在</h1><a class="button button-primary" href="/" @click.prevent="navigate('/')">返回首页</a></section></main>

    <footer v-if="route !== '/account'" class="site-footer"><a class="footer-brand" href="/" @click.prevent="navigate('/')">PERSONAL PORTAL <span>©</span></a><span>一个持续构建中的个人空间</span><a href="/#top" @click.prevent="navigate('/#top')">返回顶部 ↑</a></footer>
  </div>
</template>

<style>
:root {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans SC', 'Microsoft YaHei', sans-serif;
  color: #1d2825;
  background: #f7f8f5;
  font-synthesis: none;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  font-optical-sizing: auto;
  font-weight: 400;
  --ink: #1d2825;
  --muted: #69736e;
  --green: #276653;
  --line: #dfe4de;
  --paper: #f7f8f5;
  --mono: ui-monospace, SFMono-Regular, Consolas, monospace;
}

* { box-sizing: border-box; }
html { scroll-behavior: smooth; scroll-padding-top: 88px; }
body { margin: 0; min-width: 320px; }
a { color: inherit; text-decoration: none; }
button, input { font: inherit; }
button, a { -webkit-tap-highlight-color: transparent; }
button:focus-visible, a:focus-visible, input:focus-visible { outline: 2px solid #39856e; outline-offset: 4px; }
.site-shell { max-width: 1600px; margin: 0 auto; overflow: clip; }
.site-header { height: 82px; padding: 0 clamp(22px, 7.1vw, 112px); display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--line); }
.brand { display: inline-flex; align-items: center; gap: 12px; }
.brand-mark { display: grid; place-items: center; width: 40px; height: 40px; background: var(--green); color: #fff; font-size: 21px; font-weight: 700; letter-spacing: -2px; border-radius: 11px 11px 11px 3px; }
.brand-copy { display: flex; flex-direction: column; gap: 3px; }
.brand-copy strong { font: 500 11px var(--mono); letter-spacing: .12em; }
.brand-copy small { color: var(--muted); font-size: 11px; letter-spacing: .08em; }
.main-nav { display: flex; align-items: center; gap: clamp(22px, 3.4vw, 52px); font-size: 13px; color: #56615b; }
.main-nav a { transition: color .2s, transform .22s ease; }
.main-nav a:hover { transform: translateY(-1px) scale(1.04); }
.main-nav a:hover { color: var(--green); }
.main-nav .nav-contact { border: 1px solid #cfd8d0; padding: 10px 15px; border-radius: 5px; color: var(--ink); }
.nav-contact span { margin-left: 13px; color: var(--green); }
.menu-toggle { display: none; }

.hero { position: relative; min-height: 585px; padding: 94px clamp(22px, 11vw, 176px) 38px; display: grid; grid-template-columns: 1.05fr .95fr; align-items: center; column-gap: 6%; border-bottom: 1px solid var(--line); }
.hero-copy { position: relative; z-index: 1; padding-bottom: 32px; }
.eyebrow { display: flex; align-items: center; gap: 10px; margin: 0 0 27px; font: 500 10px var(--mono); letter-spacing: .13em; color: #69766f; }
.status-dot { width: 6px; height: 6px; border-radius: 50%; background: #5a9477; box-shadow: 0 0 0 4px #e5eee7; }
.hero h1 { margin: 0; font-size: clamp(37px, 4.3vw, 61px); line-height: 1.32; letter-spacing: -.055em; font-weight: 700; }
.hero h1 span { color: var(--green); }
.hero-description { max-width: 440px; margin: 22px 0 29px; color: #68726d; font-size: 14px; line-height: 2; }
.hero-actions { display: flex; align-items: center; gap: 25px; }
.button { display: inline-flex; align-items: center; justify-content: center; gap: 0; text-align: center; min-height: 46px; padding: 0 18px; border-radius: 4px; font-size: 13px; }
.button-primary { color: #fff; background: var(--green); transition: background .2s, transform .2s; }
.button-primary:hover { background: #1c5141; transform: translateY(-2px); }
.text-link { font-size: 12px; color: #58645d; }
.text-link span { margin-left: 8px; }
.hero-note { display: flex; align-items: center; gap: 12px; margin-top: 48px; color: #8a938e; font: 10px var(--mono); letter-spacing: .04em; }
.note-line { width: 28px; height: 1px; background: #a8b5aa; }
.hero-art { position: relative; min-height: 365px; display: grid; place-items: center; }
.art-orbit { position: absolute; border: 1px solid #dfe5dc; border-radius: 50%; transform: rotate(-26deg); }
.orbit-one { width: min(31vw, 390px); height: min(24vw, 300px); }
.orbit-two { width: min(25vw, 315px); height: min(34vw, 420px); transform: rotate(35deg); border-color: #e7eae3; }
.art-card { position: absolute; width: min(300px, 70%); height: 220px; border: 1px solid #d7dfd6; border-radius: 8px; box-shadow: 0 25px 55px #20382a12; }
.card-back { background: #e7ede5; transform: translate(26px, -22px) rotate(7deg); padding: 24px; }
.art-card-label { font: 10px var(--mono); letter-spacing: .12em; color: #77857b; }
.art-lines { display: flex; flex-direction: column; gap: 12px; margin-top: 43px; }
.art-lines i { height: 5px; border-radius: 4px; background: #d2dbd0; width: 85%; }
.art-lines i:nth-child(2) { width: 62%; }
.art-lines i:nth-child(3) { width: 74%; }
.card-front { background: #fdfefb; transform: translate(-13px, 17px) rotate(-5deg); padding: 23px; display: flex; flex-direction: column; }
.art-symbol { flex: 1; display: flex; align-items: center; justify-content: center; font-size: 98px; font-weight: 700; line-height: 1; letter-spacing: -.12em; color: #275d4b; }
.art-symbol span { color: #c0cfb6; }
.art-card-footer { display: flex; justify-content: space-between; border-top: 1px solid #e5e9e2; padding-top: 12px; color: #7a877d; font: 8px var(--mono); letter-spacing: .06em; }
.art-index { position: absolute; bottom: 5px; right: 0; font: 9px var(--mono); color: #939d95; letter-spacing: .1em; }
.art-spark { position: absolute; color: #73927a; }
.spark-one { top: 32px; right: 20px; font-size: 28px; }
.spark-two { bottom: 60px; left: 5px; font-size: 24px; font-weight: 300; }
.hero-bottom { grid-column: 1 / -1; display: flex; align-items: center; justify-content: space-between; margin-top: 34px; padding-top: 22px; border-top: 1px solid var(--line); color: #8a948d; font-size: 10px; letter-spacing: .04em; }
.hero-bottom span:last-child { font: 9px var(--mono); letter-spacing: .08em; }
.hero-bottom i { display: inline-block; width: 40px; height: 1px; margin: 0 10px 3px; background: #b7c2b8; }

.explore-section { padding: 93px clamp(22px, 11vw, 176px) 104px; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; }
.section-heading .eyebrow { margin-bottom: 18px; }
.heading-rule { display: inline-block; width: 35px; height: 1px; background: #b3c0b4; }
.section-heading h2 { margin: 0; font-size: clamp(27px, 3vw, 39px); letter-spacing: -.04em; }
.section-intro { margin: 0 0 3px; color: #7b857e; font-size: 12px; line-height: 1.9; }
.search-box { display: flex; align-items: center; gap: 13px; height: 52px; margin: 38px 0 20px; padding: 0 16px; background: #fff; border: 1px solid #e0e5de; border-radius: 5px; }
.search-icon { font-size: 26px; line-height: 1; color: #73847a; transform: rotate(-20deg); }
.search-box input { width: 100%; min-width: 0; border: 0; outline: 0; color: var(--ink); background: transparent; font-size: 12px; }
.search-box input::placeholder { color: #a1aaa2; }
.search-box kbd { white-space: nowrap; padding: 4px 6px; border: 1px solid #e2e6df; border-radius: 3px; color: #a0a8a0; font: 9px var(--mono); }
.section-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
.section-card { display: flex; flex-direction: column; min-height: 295px; padding: 22px 19px 0; border: 1px solid #e0e5dd; background: #fbfcf9; border-radius: 5px; transition: transform .2s, border-color .2s, box-shadow .2s; }
.section-card:hover { transform: translateY(-4px); border-color: #b8cabe; box-shadow: 0 14px 30px #263b2c0b; }
.card-topline { display: flex; align-items: center; justify-content: space-between; color: #9aa49b; font: 10px var(--mono); }
.card-arrow { color: #8aa092; font-size: 16px; transition: transform .2s; }
.section-card:hover .card-arrow { transform: translate(2px, -2px); }
.card-kicker { margin: 26px 0 8px; color: #688372; font: 9px var(--mono); letter-spacing: .12em; }
.section-card h3 { margin: 0; font-size: 21px; letter-spacing: -.03em; }
.card-description { min-height: 68px; margin: 12px 0 15px; color: #7a847c; font-size: 11px; line-height: 1.85; }
.tag-list { display: flex; flex-wrap: wrap; gap: 5px; margin-bottom: 22px; }
.tag-list span { padding: 5px 7px; border: 1px solid #e5e9e2; border-radius: 3px; color: #8b958c; font-size: 9px; }
.card-bottom { display: flex; justify-content: space-between; align-items: center; margin-top: auto; padding: 13px 0; border-top: 1px solid #e8ebe5; color: #6f7c72; font-size: 10px; }
.card-bottom span:last-child { color: var(--green); font-size: 14px; }
.empty-search { padding: 45px 20px; text-align: center; border: 1px dashed #d7dfd5; color: var(--muted); }
.empty-search strong { color: var(--ink); }
.empty-search p { font-size: 12px; }
.empty-search button { text-align: center; padding: 8px 12px; border: 1px solid #d2ddd2; border-radius: 4px; background: transparent; color: var(--green); cursor: pointer; }

.about-section { position: relative; display: grid; grid-template-columns: .35fr 1.3fr .7fr; align-items: center; gap: 40px; padding: 64px clamp(22px, 11vw, 176px); background: #eaf0e8; border-top: 1px solid #dce5db; border-bottom: 1px solid #dce5db; }
.about-mark { display: grid; place-items: center; width: 86px; height: 86px; border: 1px solid #bdcdbd; border-radius: 50%; color: #31634e; font-size: 39px; font-weight: 700; letter-spacing: -.12em; }
.about-copy .eyebrow { margin-bottom: 15px; color: #64816c; }
.about-copy h2 { margin: 0; font-size: clamp(25px, 2.8vw, 35px); line-height: 1.5; letter-spacing: -.04em; }
.about-copy > p:last-child { max-width: 500px; margin: 14px 0 0; color: #68766b; font-size: 12px; line-height: 2; }
.about-meta { align-self: end; display: flex; flex-direction: column; gap: 9px; padding-bottom: 5px; color: #788a7b; font: 9px var(--mono); letter-spacing: .08em; }
.about-meta span:last-child { color: #8c9a8d; font: 11px -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; letter-spacing: 0; }

.site-footer { min-height: 78px; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px clamp(22px, 7.1vw, 112px); color: #89928b; font-size: 10px; }
.footer-brand { color: #5c6e60; font: 10px var(--mono); letter-spacing: .08em; }
.footer-brand span { margin-left: 5px; }
.site-footer > a:last-child { color: #526d5b; }

@media (min-width: 1400px) {
  .hero { min-height: 650px; }
  .section-card { min-height: 315px; padding-inline: 23px; }
}
@media (max-width: 1050px) {
  .hero { padding-inline: 7vw; column-gap: 2%; }
  .explore-section, .about-section { padding-inline: 7vw; }
  .section-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .section-card { min-height: 270px; }
  .card-description { min-height: 48px; }
  .about-section { grid-template-columns: .25fr 1.4fr; }
  .about-meta { grid-column: 2; flex-direction: row; gap: 20px; }
}
@media (max-width: 700px) {
  .site-header { height: 70px; padding-inline: 20px; }
  .brand-mark { width: 36px; height: 36px; }
  .brand-copy strong { font-size: 9px; }
  .brand-copy small { font-size: 10px; }
  .menu-toggle { display: flex; flex-direction: column; gap: 5px; padding: 10px 4px; border: 0; background: transparent; }
  .menu-toggle span { display: block; width: 23px; height: 1px; background: var(--ink); }
  .main-nav { position: absolute; z-index: 5; top: 69px; left: 0; right: 0; display: none; align-items: stretch; gap: 0; padding: 8px 20px 16px; background: var(--paper); border-bottom: 1px solid var(--line); }
  .main-nav.is-open { display: flex; flex-direction: column; }
  .main-nav a { padding: 13px 0; }
  .main-nav .nav-contact { border: 0; padding: 13px 0; }
  .hero { grid-template-columns: 1fr; padding: 67px 22px 25px; }
  .hero-copy { padding-bottom: 0; }
  .hero .eyebrow { margin-bottom: 22px; font-size: 9px; }
  .hero h1 { font-size: clamp(34px, 9vw, 49px); }
  .hero-description { margin-top: 17px; font-size: 13px; }
  .hero-note { margin-top: 30px; }
  .hero-art { min-height: 310px; margin-top: 10px; }
  .art-card { width: min(270px, 72%); height: 200px; }
  .orbit-one { width: 78vw; height: 55vw; }
  .orbit-two { width: 60vw; height: 85vw; }
  .art-index { right: 4px; bottom: 0; font-size: 8px; }
  .hero-bottom { margin-top: 26px; padding-top: 17px; font-size: 9px; }
  .hero-bottom span:last-child { font-size: 8px; }
  .hero-bottom i { width: 18px; margin-inline: 5px; }
  .explore-section { padding: 65px 20px 70px; }
  .section-heading { align-items: flex-start; flex-direction: column; gap: 15px; }
  .section-heading h2 { font-size: 29px; }
  .section-intro br { display: none; }
  .search-box { margin-top: 25px; }
  .search-box kbd { display: none; }
  .section-grid { grid-template-columns: 1fr; gap: 10px; }
  .section-card { min-height: 250px; padding-inline: 20px; }
  .card-kicker { margin-top: 20px; }
  .card-description { min-height: 0; max-width: 360px; }
  .about-section { grid-template-columns: 1fr; gap: 22px; padding: 43px 22px; }
  .about-mark { width: 60px; height: 60px; font-size: 28px; }
  .about-copy h2 { font-size: 27px; }
  .about-meta { grid-column: auto; flex-direction: column; gap: 8px; }
  .site-footer { align-items: flex-start; flex-direction: column; padding: 23px 20px; gap: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; animation-duration: .01ms !important; }
}

/* macOS-inspired glassmorphism layer */
:root {
  color: #17283c;
  background: #dce8f5;
  --ink: #17283c;
  --muted: #61738a;
  --green: #245ca8;
  --line: rgba(255, 255, 255, .48);
  --paper: rgba(244, 248, 255, .58);
}
body {
  background:
    radial-gradient(ellipse at 12% 8%, rgba(255, 226, 209, .95), transparent 32%),
    radial-gradient(ellipse at 83% 18%, rgba(181, 213, 247, .9), transparent 35%),
    radial-gradient(ellipse at 60% 72%, rgba(192, 222, 235, .95), transparent 38%),
    linear-gradient(135deg, #eaf0f8 0%, #cbdced 48%, #e8e0dc 100%);
  background-attachment: fixed;
}
.site-shell { background: transparent; }
.site-header {
  position: sticky;
  top: 0;
  z-index: 20;
  background: rgba(245, 249, 255, .58);
  border-bottom-color: rgba(255, 255, 255, .6);
  -webkit-backdrop-filter: blur(24px) saturate(155%);
  backdrop-filter: blur(24px) saturate(155%);
}
.brand-mark {
  background: linear-gradient(145deg, #4f91dc, #3157a4);
  border: 1px solid rgba(255, 255, 255, .5);
  box-shadow: 0 5px 18px rgba(49, 87, 164, .2);
}
.main-nav { color: #455c78; }
.main-nav .nav-contact {
  background: rgba(255, 255, 255, .36);
  border-color: rgba(255, 255, 255, .72);
  border-radius: 999px;
  box-shadow: inset 0 1px rgba(255,255,255,.55);
}
.hero {
  border-bottom-color: rgba(255, 255, 255, .45);
}
.hero h1 span { color: #326bb0; }
.status-dot { background: #4c9e8c; box-shadow: 0 0 0 4px rgba(225, 248, 243, .8); }
.button { border-radius: 999px; }
.button-primary {
  background: linear-gradient(135deg, #4c91dc, #315bb1);
  box-shadow: 0 8px 22px rgba(47, 94, 171, .2), inset 0 1px rgba(255,255,255,.3);
}
.button-primary:hover { background: linear-gradient(135deg, #3e82cc, #284d9d); transform: translateY(-2px) scale(1.07); }
.art-orbit { border-color: rgba(255,255,255,.58); }
.orbit-two { border-color: rgba(255,255,255,.38); }
.art-card {
  border: 1px solid rgba(255, 255, 255, .78);
  border-radius: 22px;
  box-shadow: 0 24px 65px rgba(45, 71, 107, .13), inset 0 1px rgba(255,255,255,.8);
  -webkit-backdrop-filter: blur(22px) saturate(145%);
  backdrop-filter: blur(22px) saturate(145%);
}
.card-back { background: rgba(216, 231, 246, .56); }
.art-lines i { background: rgba(110, 143, 177, .23); }
.card-front { background: linear-gradient(145deg, rgba(255,255,255,.8), rgba(231,241,252,.55)); }
.art-symbol { color: #315fa2; }
.art-symbol span { color: #90b5e1; }
.art-card-footer { border-top-color: rgba(255,255,255,.72); }
.hero-bottom { border-top-color: rgba(255,255,255,.48); }
.explore-section { position: relative; }
.search-box {
  background: rgba(255, 255, 255, .43);
  border-color: rgba(255,255,255,.8);
  border-radius: 17px;
  box-shadow: 0 8px 28px rgba(47, 73, 106, .045), inset 0 1px rgba(255,255,255,.7);
  -webkit-backdrop-filter: blur(24px) saturate(150%);
  backdrop-filter: blur(24px) saturate(150%);
}
.search-box kbd { border-color: rgba(255,255,255,.75); background: rgba(255,255,255,.36); }
.section-card {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(255,255,255,.8);
  border-radius: 22px;
  background: linear-gradient(145deg, rgba(255,255,255,.61), rgba(243,248,255,.3));
  box-shadow: 0 12px 34px rgba(49, 75, 108, .06), inset 0 1px rgba(255,255,255,.8);
  -webkit-backdrop-filter: blur(22px) saturate(145%);
  backdrop-filter: blur(22px) saturate(145%);
}
.section-card::before {
  content: '';
  position: absolute;
  width: 180px;
  height: 180px;
  top: -115px;
  right: -90px;
  border-radius: 50%;
  background: rgba(255,255,255,.24);
  filter: blur(4px);
  pointer-events: none;
}
.section-card:hover {
  z-index: 2;
  transform: translateY(-7px) scale(1.045);
  border-color: rgba(255,255,255,.98);
  box-shadow: 0 20px 42px rgba(49, 75, 108, .12), inset 0 1px rgba(255,255,255,.9);
}
.card-kicker { color: #527caf; }
.tag-list span {
  border-color: rgba(255,255,255,.8);
  background: rgba(255,255,255,.28);
  border-radius: 999px;
}
.card-bottom { border-top-color: rgba(255,255,255,.72); }
.about-section {
  background: linear-gradient(125deg, rgba(230,241,251,.68), rgba(255,246,241,.46));
  border-top-color: rgba(255,255,255,.65);
  border-bottom-color: rgba(255,255,255,.65);
  -webkit-backdrop-filter: blur(26px) saturate(145%);
  backdrop-filter: blur(26px) saturate(145%);
}
.about-mark {
  border-color: rgba(255,255,255,.9);
  background: rgba(255,255,255,.3);
  color: #315fa2;
  box-shadow: inset 0 1px rgba(255,255,255,.75), 0 8px 24px rgba(49,75,108,.06);
}
.about-copy .eyebrow { color: #527caf; }
.site-footer { color: #62768c; }
.footer-brand, .site-footer > a:last-child { color: #315f9e; }
@media (max-width: 700px) {
  .main-nav { background: rgba(240,247,255,.88); -webkit-backdrop-filter: blur(24px); backdrop-filter: blur(24px); }
  .section-card { border-radius: 19px; }
}
@media (prefers-reduced-transparency: reduce) {
  .site-header, .art-card, .search-box, .section-card, .about-section {
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
  }
  .site-header { background: #eef4fb; }
  .search-box, .section-card { background: #f2f6fb; }
}

/* Glass refinement v2: translucent surfaces, no bright outlines, restrained motion */
:root {
  --glass-blur: 26px;
  --glass-border: rgba(51, 76, 105, 0.075);
  --glass-surface: rgba(245, 249, 255, 0.42);
  --glass-surface-strong: rgba(248, 251, 255, 0.62);
  --glass-shadow: 0 10px 34px rgba(34, 55, 83, 0.075);
  --glass-shadow-hover: 0 16px 38px rgba(34, 55, 83, 0.13);
  --glass-motion: 220ms cubic-bezier(.2, .7, .2, 1);
}
body {
  background:
    radial-gradient(ellipse at 10% 8%, rgba(255, 224, 208, .78), transparent 34%),
    radial-gradient(ellipse at 88% 14%, rgba(160, 199, 242, .76), transparent 36%),
    radial-gradient(ellipse at 58% 68%, rgba(188, 220, 232, .78), transparent 40%),
    linear-gradient(135deg, #e9eff7 0%, #cbdced 48%, #e8e1df 100%);
  background-attachment: fixed;
}
.site-header {
  background: rgba(231, 239, 249, .48);
  border-bottom: 1px solid rgba(42, 66, 96, .055);
  box-shadow: 0 4px 24px rgba(36, 56, 84, .025);
  -webkit-backdrop-filter: blur(var(--glass-blur)) saturate(155%);
  backdrop-filter: blur(var(--glass-blur)) saturate(155%);
}
.main-nav a,
.text-link,
.site-footer a,
.footer-brand {
  transition: color var(--glass-motion), transform var(--glass-motion);
}
.main-nav .nav-contact {
  background: rgba(237, 244, 252, .34);
  border: 1px solid rgba(48, 71, 100, .07);
  box-shadow: 0 4px 16px rgba(34, 55, 83, .035);
}
.hero,
.hero-bottom {
  border-color: rgba(42, 66, 96, .07);
}
.button-primary {
  border: 0;
  background: linear-gradient(135deg, rgba(65, 126, 190, .96), rgba(43, 83, 153, .96));
  box-shadow: 0 8px 22px rgba(47, 94, 171, .17);
  transition: transform var(--glass-motion), box-shadow var(--glass-motion), background var(--glass-motion);
}
.button-primary:hover {
  transform: translateY(-2px) scale(1.015);
  box-shadow: 0 12px 26px rgba(47, 94, 171, .22);
}
.art-card {
  border: 0;
  box-shadow: 0 24px 60px rgba(38, 62, 94, .12), inset 0 1px rgba(255,255,255,.22);
  -webkit-backdrop-filter: blur(26px) saturate(150%);
  backdrop-filter: blur(26px) saturate(150%);
}
.card-back {
  background: rgba(174, 202, 231, .38);
}
.card-front {
  background: linear-gradient(145deg, rgba(250, 252, 255, .58), rgba(220, 234, 249, .34));
}
.art-card-footer {
  border-top-color: rgba(58, 80, 109, .08);
}
.search-box {
  background: rgba(239, 246, 254, .38);
  border: 1px solid rgba(47, 70, 99, .07);
  box-shadow: 0 8px 26px rgba(43, 65, 94, .045), inset 0 1px rgba(255,255,255,.22);
  -webkit-backdrop-filter: blur(var(--glass-blur)) saturate(150%);
  backdrop-filter: blur(var(--glass-blur)) saturate(150%);
  transition: background var(--glass-motion), box-shadow var(--glass-motion), transform var(--glass-motion);
}
.search-box:focus-within {
  background: rgba(248, 251, 255, .58);
  box-shadow: 0 10px 28px rgba(43, 65, 94, .075), 0 0 0 2px rgba(68, 126, 190, .17);
}
.search-box kbd {
  border: 0;
  background: rgba(65, 91, 123, .07);
}
.section-card {
  border: 1px solid var(--glass-border);
  background: linear-gradient(145deg, rgba(247, 250, 255, .47), rgba(231, 240, 250, .28));
  box-shadow: var(--glass-shadow), inset 0 1px rgba(255,255,255,.18);
  -webkit-backdrop-filter: blur(var(--glass-blur)) saturate(148%);
  backdrop-filter: blur(var(--glass-blur)) saturate(148%);
  transition: transform var(--glass-motion), box-shadow var(--glass-motion), background var(--glass-motion);
}
.section-card::before {
  background: rgba(255,255,255,.12);
  filter: blur(12px);
}
.section-card:hover {
  transform: translateY(-3px) scale(1.015);
  border-color: rgba(51, 76, 105, .09);
  background: linear-gradient(145deg, rgba(250, 252, 255, .58), rgba(235, 243, 252, .36));
  box-shadow: var(--glass-shadow-hover), inset 0 1px rgba(255,255,255,.2);
}
.tag-list span {
  border: 0;
  background: rgba(61, 91, 126, .065);
  color: #5e7189;
}
.card-bottom {
  border-top-color: rgba(42, 66, 96, .075);
}
.about-section {
  background: linear-gradient(120deg, rgba(194, 216, 237, .36), rgba(247, 229, 219, .3));
  border-top: 1px solid rgba(42, 66, 96, .065);
  border-bottom: 1px solid rgba(42, 66, 96, .065);
  box-shadow: inset 0 1px rgba(255,255,255,.14);
  -webkit-backdrop-filter: blur(30px) saturate(145%);
  backdrop-filter: blur(30px) saturate(145%);
}
.about-mark {
  border: 0;
  background: rgba(237, 245, 253, .34);
  box-shadow: 0 10px 28px rgba(43, 65, 94, .07), inset 0 1px rgba(255,255,255,.18);
}
.empty-search {
  border: 1px dashed rgba(47, 70, 99, .18);
  background: rgba(239, 246, 254, .25);
  -webkit-backdrop-filter: blur(18px);
  backdrop-filter: blur(18px);
}
.empty-search button {
  border: 0;
  background: rgba(65, 126, 190, .1);
  transition: transform var(--glass-motion), background var(--glass-motion);
}
.empty-search button:hover {
  transform: translateY(-2px) scale(1.015);
  background: rgba(65, 126, 190, .16);
}
.site-footer {
  background: rgba(230, 238, 248, .16);
}
@media (hover: hover) and (pointer: fine) {
  .section-card:hover,
  .button-primary:hover,
  .empty-search button:hover {
    will-change: transform;
  }
}
@media (max-width: 700px) {
  .main-nav {
    background: rgba(226, 237, 249, .82);
    border-bottom: 1px solid rgba(42, 66, 96, .07);
    box-shadow: 0 14px 28px rgba(34, 55, 83, .07);
    -webkit-backdrop-filter: blur(28px) saturate(150%);
    backdrop-filter: blur(28px) saturate(150%);
  }
  .section-card:hover {
    transform: translateY(-2px) scale(1.008);
  }
}
@media (prefers-reduced-transparency: reduce) {
  .site-header, .art-card, .search-box, .section-card, .about-section, .main-nav {
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
  }
  .site-header, .main-nav { background: #e8f0f8; }
  .art-card, .search-box, .section-card { background: #edf3fa; }
  .about-section { background: #e3edf5; }
}

/* Typography scale: Cloudflare-inspired, clearer hierarchy and larger readable text */
.brand-copy strong { font-size: 12px; }
.brand-copy small { font-size: 13px; }
.main-nav { font-size: 15px; }
.main-nav .nav-contact { padding: 12px 17px; }
.hero .eyebrow,
.section-heading .eyebrow,
.about-copy .eyebrow { font-size: 12px; }
.hero h1 { font-size: clamp(44px, 5vw, 72px); line-height: 1.24; }
.hero-description { max-width: 510px; font-size: 17px; line-height: 1.9; }
.button { min-height: 52px; padding-inline: 22px; font-size: 15px; }
.hero-actions { gap: 28px; }
.text-link { font-size: 15px; }
.hero-note { font-size: 12px; }
.hero-bottom { font-size: 12px; }
.hero-bottom span:last-child { font-size: 11px; }
.art-card-label { font-size: 11px; }
.art-card-footer { font-size: 9px; }
.art-index { font-size: 10px; }
.explore-section { padding-top: 100px; padding-bottom: 112px; }
.section-heading h2 { font-size: clamp(34px, 3.4vw, 48px); }
.section-intro { font-size: 15px; line-height: 1.85; }
.search-box { min-height: 60px; }
.search-box input { font-size: 15px; }
.search-box kbd { font-size: 11px; padding: 5px 7px; }
.section-grid { gap: 18px; }
.section-card { min-height: 340px; padding: 25px 22px 0; }
.card-topline { font-size: 12px; }
.card-kicker { font-size: 11px; margin-top: 28px; }
.section-card h3 { font-size: 25px; }
.card-description { min-height: 84px; font-size: 14px; line-height: 1.9; }
.tag-list { gap: 6px; }
.tag-list span { padding: 6px 9px; font-size: 12px; }
.card-bottom { padding: 15px 0; font-size: 12px; }
.card-bottom span:last-child { font-size: 16px; }
.empty-search strong { font-size: 18px; }
.empty-search p { font-size: 15px; }
.empty-search button { padding: 10px 15px; font-size: 14px; }
.about-section { padding-top: 72px; padding-bottom: 72px; }
.about-copy h2 { font-size: clamp(30px, 3.2vw, 43px); line-height: 1.4; }
.about-copy > p:last-child { max-width: 560px; font-size: 15px; line-height: 1.9; }
.about-meta { font-size: 11px; }
.about-meta span:last-child { font-size: 13px; }
.site-footer { font-size: 12px; }
.footer-brand { font-size: 11px; }
@media (min-width: 1400px) {
  .section-card { min-height: 355px; padding-inline: 25px; }
}
@media (max-width: 1050px) {
  .hero h1 { font-size: clamp(42px, 5.3vw, 60px); }
  .hero-description { font-size: 16px; }
  .section-card { min-height: 310px; }
  .card-description { min-height: 66px; }
  .about-copy h2 { font-size: clamp(30px, 3.6vw, 40px); }
}
@media (max-width: 700px) {
  .brand-copy strong { font-size: 11px; }
  .brand-copy small { font-size: 12px; }
  .main-nav { font-size: 16px; }
  .main-nav a, .main-nav .nav-contact { padding-top: 15px; padding-bottom: 15px; }
  .hero { padding-top: 62px; }
  .hero .eyebrow { font-size: 10px; line-height: 1.6; letter-spacing: .09em; }
  .hero h1 { font-size: clamp(36px, 8.8vw, 50px); line-height: 1.3; }
  .hero-description { font-size: 15px; line-height: 1.9; }
  .button { min-height: 49px; padding-inline: 18px; font-size: 14px; }
  .hero-actions { gap: 18px; flex-wrap: wrap; }
  .text-link { font-size: 14px; }
  .hero-note { margin-top: 27px; font-size: 11px; }
  .hero-bottom { font-size: 11px; }
  .hero-bottom span:last-child { font-size: 9px; }
  .art-card-label { font-size: 10px; }
  .art-card-footer { font-size: 8px; }
  .art-index { font-size: 9px; }
  .explore-section { padding-top: 70px; padding-bottom: 76px; }
  .section-heading .eyebrow { font-size: 10px; }
  .section-heading h2 { font-size: clamp(30px, 7.4vw, 36px); }
  .section-intro { font-size: 14px; line-height: 1.85; }
  .search-box { min-height: 56px; margin-top: 25px; gap: 10px; padding-inline: 14px; }
  .search-box input { font-size: 14px; }
  .section-grid { gap: 13px; }
  .section-card { min-height: 0; padding: 22px 20px 0; }
  .card-topline { font-size: 11px; }
  .card-kicker { margin-top: 21px; font-size: 10px; }
  .section-card h3 { font-size: 24px; }
  .card-description { min-height: 0; margin-top: 11px; margin-bottom: 16px; font-size: 14px; line-height: 1.85; }
  .tag-list { margin-bottom: 19px; gap: 6px; }
  .tag-list span { padding: 6px 9px; font-size: 11px; }
  .card-bottom { padding: 13px 0; font-size: 11px; }
  .about-section { gap: 21px; padding-top: 45px; padding-bottom: 45px; }
  .about-copy .eyebrow { font-size: 10px; }
  .about-copy h2 { font-size: clamp(28px, 7vw, 34px); line-height: 1.45; }
  .about-copy > p:last-child { font-size: 14px; line-height: 1.9; }
  .about-meta { font-size: 10px; }
  .about-meta span:last-child { font-size: 12px; }
  .site-footer { font-size: 12px; }
  .footer-brand { font-size: 11px; }
}


/* Routed account pages; keep the established Liquid Glass palette and type scale. */
.route-main { min-height: min(78vh, 900px); display: grid; place-items: center; padding: clamp(34px, 7vw, 82px) 24px; }
.glass-panel { width: min(100%, 560px); padding: clamp(28px, 5vw, 52px); border: 0; border-radius: 28px; background: linear-gradient(145deg, rgba(255,255,255,.78), rgba(237,244,252,.62)); box-shadow: 0 24px 70px rgba(35,57,82,.10); -webkit-backdrop-filter: blur(26px) saturate(145%); backdrop-filter: blur(26px) saturate(145%); }
.auth-panel h1, .account-panel h1 { margin: 14px 0 12px; font-size: clamp(36px, 4.5vw, 52px); line-height: 1.12; letter-spacing: -.045em; }
.auth-intro { color: var(--muted); line-height: 1.8; font-size: 16px; margin: 0 0 28px; }
.auth-form { display: grid; gap: 18px; }
.field-label { display: grid; gap: 9px; color: var(--ink); font-weight: 600; font-size: 15px; }
.field-label input { width: 100%; min-height: 52px; border: 1px solid rgba(133,151,169,.35); border-radius: 14px; padding: 0 16px; outline: none; color: var(--ink); background: rgba(255,255,255,.78); transition: border-color .18s, box-shadow .18s; }
.field-label input:focus { border-color: #7296c3; box-shadow: 0 0 0 4px rgba(91,139,196,.13); }
.field-help { margin: -8px 0 0; font-size: 13px; line-height: 1.7; color: var(--muted); }
.form-error, .form-notice { padding: 12px 14px; border-radius: 12px; margin: 0; font-size: 14px; line-height: 1.65; }
.form-error { color: #8b302c; background: rgba(255,232,229,.85); border: 1px solid rgba(185,91,83,.2); }
.form-notice { color: #315f4f; background: rgba(225,243,235,.78); border: 1px solid rgba(76,140,112,.2); margin-bottom: 18px; }
.auth-submit { width: 100%; min-height: 52px; justify-content: center; border: 0; margin-top: 2px; }
.auth-submit:disabled { opacity: .65; cursor: wait; }
.auth-switch { margin: 24px 0 0; text-align: center; color: var(--muted); font-size: 14px; }
.auth-switch a { color: var(--green); font-weight: 650; text-decoration: underline; text-underline-offset: 3px; }
.back-home { display: inline-block; margin-top: 26px; color: var(--muted); font-size: 14px; }
.account-panel { width: min(100%, 650px); }
.account-details { display: grid; gap: 0; }
.account-avatar { display: grid; place-items: center; width: 68px; height: 68px; border-radius: 22px; margin: 0 0 20px; color: #fff; font-size: 30px; font-weight: 650; background: linear-gradient(145deg, #74a9db, #6276b8); }
.account-field { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 18px 0; border-bottom: 1px solid rgba(128,145,164,.2); }
.account-field span { color: var(--muted); font-size: 14px; }
.account-field strong { color: var(--ink); font-size: 15px; font-weight: 600; text-align: right; overflow-wrap: anywhere; }
.account-details .auth-submit { margin-top: 28px; }
.account-loading { padding: 22px 0; color: var(--muted); }
.main-nav .nav-auth { padding: 11px 19px; border: 1px solid rgba(103, 139, 178, .32); border-radius: 999px; background: rgba(255, 255, 255, .72); color: #294f78; font-weight: 650; box-shadow: 0 4px 14px rgba(48, 78, 112, .06); }
.main-nav .nav-auth:hover { background: rgba(255, 255, 255, .92); color: #1e4168; }
/* No hover zoom: interactive elements use the pointer cursor only. */
a, button, [role="button"] { cursor: pointer; }
a:hover, button:hover, .section-card:hover, .section-card:hover .card-arrow, .main-nav a:hover, .button-primary:hover, .empty-search button:hover { transform: none !important; }
.section-card:hover { will-change: auto !important; }
@media (max-width: 700px) {
  .route-main { min-height: 72vh; padding: 28px 16px 48px; }
  .glass-panel { border-radius: 22px; padding: 26px 21px; }
  .account-field { align-items: flex-start; flex-direction: column; gap: 7px; }
  .account-field strong { text-align: left; }
}

/* Account center: left collapsible navigation and right-side content workspace. */
.account-layout {
  width: 100%;
  height: 100dvh;
  min-height: 0;
  max-height: 100dvh;
  display: grid;
  grid-template-columns: clamp(190px, 15vw, 250px) minmax(0, 1fr);
  align-items: stretch;
  gap: 0;
  padding: 0;
  overflow: hidden;
}
.account-sidebar {
  position: relative;
  align-self: stretch;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
  -ms-overflow-style: none;
  padding: 22px 12px 18px;
  border: 0;
  border-right: 1px solid rgba(51, 76, 105, .09);
  border-radius: 0;
  background: rgba(245, 249, 255, .26);
  box-shadow: none;
  -webkit-backdrop-filter: blur(20px) saturate(130%);
  backdrop-filter: blur(20px) saturate(130%);
}
.account-sidebar::-webkit-scrollbar { display: none; width: 0; height: 0; }
.account-sidebar-heading { display: none; }
.account-nav-group + .account-nav-group { margin-top: 5px; }
.account-group-toggle { font-weight: 600; }
.account-sidebar-collapse { padding: 4px 8px 8px 14px; }
.account-sidebar-subitem { width: calc(100% - 6px); min-height: 42px; margin-left: 6px; padding-left: 11px; font-size: 15px; }
.account-sidebar-item {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  text-align: left;
  gap: 10px;
  width: 100%;
  min-height: 47px;
  padding: 0 12px;
  border: 0;
  border-radius: 10px;
  color: #586d84;
  text-align: left;
  font-size: 16px;
  background: transparent;
  transition: color .18s ease, background .18s ease;
}
.account-sidebar-item:hover { color: #294f78; background: rgba(255,255,255,.32); }
.account-sidebar-item.is-active { color: #174f9b; background: #dceaff; box-shadow: inset 3px 0 0 #3478d4; font-weight: 650; }
.account-sidebar-subitem { width: calc(100% - 6px); min-height: 42px; margin-left: 6px; padding: 0 10px; font-size: 15px; }
.virtual-table-panel { display: flex; flex-direction: column; min-width: 0; min-height: 0; height: calc(100% - 94px); margin: 0; overflow: hidden; border: 0; border-radius: 0; background: transparent; box-shadow: none; }
.virtual-table-toolbar { display: flex; flex: 0 0 auto; align-items: center; justify-content: space-between; gap: 18px; padding: 12px 18px; border-bottom: 1px solid rgba(133,157,181,.16); }
.virtual-table-heading { display: flex; align-items: baseline; flex-wrap: wrap; gap: 12px; min-width: 0; }
.virtual-table-heading h2 { margin: 0; color: #304760; font-size: 18px; font-weight: 650; letter-spacing: -.02em; }
.virtual-table-count { color: #8795a6; font-size: 12px; white-space: nowrap; }
.virtual-table-search { width: min(260px, 42%); min-width: 140px; height: 38px; padding: 0 12px; border: 1px solid rgba(126,150,176,.2); border-radius: 10px; color: #344b63; background: rgba(255,255,255,.56); font-size: 13px; outline: none; }
.virtual-table-search:focus { border-color: rgba(66,124,190,.5); box-shadow: 0 0 0 3px rgba(66,124,190,.08); }
.virtual-table-scroll { flex: 1 1 auto; min-width: 0; min-height: 0; max-height: none; overflow: auto; overscroll-behavior: contain; }
.virtual-table-header, .virtual-table-row { display: grid; grid-template-columns: var(--table-columns); min-width: max-content; align-items: center; }
.virtual-table-header { position: sticky; top: 0; z-index: 1; min-height: 44px; color: #71849a; background: rgba(242,247,251,.96); border-bottom: 1px solid rgba(133,157,181,.16); font-size: 12px; font-weight: 600; }
.virtual-table-header span, .virtual-table-row span { overflow: hidden; padding: 0 16px; text-overflow: ellipsis; white-space: nowrap; }
.virtual-table-row { min-height: 44px; color: #40566e; border-bottom: 1px solid rgba(133,157,181,.11); font-size: 13px; }
.virtual-table-row:hover { background: rgba(255,255,255,.5); }
.virtual-table-empty { display: flex; min-height: 220px; flex-direction: column; align-items: center; justify-content: center; padding: 30px 18px; text-align: center; }
.virtual-table-empty-icon { display: grid; place-items: center; width: 38px; height: 38px; margin-bottom: 12px; border: 1px solid rgba(133,157,181,.18); border-radius: 12px; color: #6685a7; background: rgba(255,255,255,.52); font-size: 18px; }
.virtual-table-empty strong { color: #52677e; font-size: 14px; font-weight: 600; }
.virtual-table-empty p { margin: 7px 0 0; color: #8a98a8; font-size: 12px; }
.virtual-table-footnote { margin: 0; padding: 10px 16px; border-top: 1px solid rgba(133,157,181,.12); color: #9aa6b4; font-size: 11px; }
@media (max-width: 680px) { .virtual-table-toolbar { align-items: stretch; flex-direction: column; gap: 12px; padding: 12px; } .virtual-table-search { width: 100%; } .account-content-table-mode h1.account-page-title { margin: 12px 12px 4px; } .account-content-table-mode > .auth-intro { margin: 0 12px 8px; } .virtual-table-panel { height: calc(100% - 86px); } }
.workspace-placeholder { margin-top: 28px; padding: 34px; border-radius: 22px; text-align: center; background: rgba(255,255,255,.28); }
.workspace-placeholder-icon { display: grid; place-items: center; width: 54px; height: 54px; margin: 0 auto 14px; border-radius: 18px; color: #315f9e; background: rgba(255,255,255,.55); font-size: 28px; }
.workspace-placeholder h2 { margin: 0 0 8px; color: #304760; font-size: 20px; }
.workspace-placeholder p { margin: 0; color: #75869a; font-size: 14px; }
.sidebar-item-dot { width: 6px; height: 6px; flex: 0 0 6px; border-radius: 50%; background: rgba(94,125,158,.4); }
.account-sidebar-item.is-active .sidebar-item-dot { background: #2868bd; box-shadow: 0 0 0 4px rgba(52,120,212,.16); }
.account-sidebar-home { display: block; margin: 4px 20px 20px; padding-top: 16px; border-top: 1px solid rgba(51,76,105,.07); color: #718197; font-size: 13px; }
.account-sidebar-home:hover { color: #315f9e; }
.account-content {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  max-height: 100%;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
  -ms-overflow-style: none;
  padding: clamp(30px, 4vw, 64px);
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  -webkit-backdrop-filter: none;
  backdrop-filter: none;
}
.account-content.account-content-table-mode { display: flex; flex-direction: column; overflow: hidden; padding: 0; }
.account-content-table-mode h1.account-page-title { flex: 0 0 auto; margin: 16px 18px 4px; font-size: clamp(26px, 2.4vw, 34px); }
.account-content-table-mode > .auth-intro { flex: 0 0 auto; margin: 0 18px 10px; font-size: 13px; }
.account-search-empty { padding: 10px 8px; color: #7a8ca2; font-size: 12px; text-align: center; }
.account-content::-webkit-scrollbar { display: none; width: 0; height: 0; }
.account-content h1 { margin: 14px 0 12px; font-size: clamp(34px, 3.4vw, 48px); line-height: 1.16; letter-spacing: -.045em; }
.account-content h1.account-page-title { margin-top: 0; }
.sidebar-group-icon { display: inline-grid; place-items: center; flex: 0 0 20px; width: 20px; height: 20px; border-radius: 6px; color: #315f9e; background: rgba(255,255,255,.52); font-size: 14px; font-weight: 500; line-height: 1; }
.account-summary-card {
  display: flex;
  align-items: center;
  gap: 17px;
  padding: 20px;
  margin: 4px 0 14px;
  border: 1px solid rgba(51,76,105,.065);
  border-radius: 20px;
  background: rgba(255,255,255,.25);
}
.account-summary-card .account-avatar { flex: 0 0 60px; width: 60px; height: 60px; margin: 0; border-radius: 19px; }
.account-summary-card > div:nth-child(2) { display: grid; min-width: 0; gap: 6px; }
.account-summary-name { color: #2b415a; font-size: 18px; overflow-wrap: anywhere; }
.account-summary-email { color: #72839a; font-size: 14px; overflow-wrap: anywhere; }
.account-status { display: inline-flex; align-items: center; gap: 7px; margin-left: auto; white-space: nowrap; color: #4d806d; font-size: 12px; }
.account-status i { width: 6px; height: 6px; border-radius: 50%; background: #58a17d; }
.security-info-row { display: flex; align-items: center; gap: 14px; padding: 20px; border: 1px solid rgba(72,143,111,.12); border-radius: 18px; background: rgba(228,246,237,.35); }
.security-info-icon { display: grid; place-items: center; flex: 0 0 36px; height: 36px; border-radius: 50%; color: #39775b; background: rgba(255,255,255,.6); font-size: 18px; }
.security-info-row strong { color: #315f4f; font-size: 15px; }
.security-info-row p { margin: 5px 0 0; color: #72867b; font-size: 13px; }
@media (max-width: 760px) {
  .account-layout { height: 100dvh; min-height: 0; max-height: 100dvh; grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); align-content: stretch; gap: 0; padding: 0; overflow: hidden; }
  .account-sidebar { position: relative; height: auto; max-height: 34dvh; border-radius: 0; border-right: 0; border-bottom: 1px solid rgba(51,76,105,.09); padding: 10px 12px; }
  .account-content { height: 100%; max-height: 100%; overflow: auto; padding: 26px 21px; }
  .account-sidebar-heading { min-height: 72px; padding: 14px 16px; }
  .account-sidebar-collapse { padding: 4px 8px 8px; }
  .account-sidebar-collapse .account-sidebar-item { min-height: 40px; }
  .account-sidebar-home { margin: 4px 12px 10px; }
  .account-content { min-height: 0; padding: 26px 21px; }
  .account-content.account-content-table-mode { padding: 0; }
  .account-content h1 { font-size: clamp(30px, 8vw, 38px); }
  .account-summary-card { align-items: flex-start; flex-wrap: wrap; padding: 16px; gap: 12px; }
  .account-summary-card .account-avatar { flex-basis: 52px; width: 52px; height: 52px; }
  .account-status { margin-left: 0; }
}
@media (prefers-reduced-transparency: reduce) {
  .account-sidebar, .account-content { -webkit-backdrop-filter: none; backdrop-filter: none; }
  .account-sidebar { background: #edf3fa; }
  .account-content { background: transparent; }
}


/* Content management editor and row actions */
.content-toolbar-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; min-width: 0; }
.content-editor-panel { width: 100%; min-width: 0; padding: 18px clamp(12px, 2vw, 28px) 28px; overflow: auto; }
.content-editor-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-bottom: 22px; }
.content-editor-heading h2 { margin: 0; color: #304760; font-size: 21px; font-weight: 650; }
.content-editor-heading p { margin: 7px 0 0; color: #71849a; font-size: 13px; line-height: 1.6; }
.content-editor-form { display: grid; gap: 17px; max-width: 1000px; }
.content-editor-form label { display: grid; gap: 8px; color: #40566e; font-size: 13px; font-weight: 650; }
.content-editor-form input, .content-editor-form select, .content-editor-form textarea { width: 100%; min-width: 0; border: 1px solid rgba(126,150,176,.24); border-radius: 11px; padding: 11px 13px; color: #344b63; background: rgba(255,255,255,.65); font: inherit; font-weight: 400; outline: none; }
.content-editor-form input, .content-editor-form select { min-height: 42px; }
.content-editor-form textarea { resize: vertical; line-height: 1.75; }
.content-editor-form input:focus, .content-editor-form select:focus, .content-editor-form textarea:focus { border-color: rgba(66,124,190,.56); box-shadow: 0 0 0 3px rgba(66,124,190,.09); }
.content-editor-form small { color: #8190a1; font-size: 11px; font-weight: 400; }
.content-form-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.content-editor-actions { display: flex; justify-content: flex-end; gap: 10px; padding-top: 4px; }
.content-primary-button, .content-secondary-button { flex: 0 0 auto; min-height: 38px; padding: 0 15px; border: 1px solid transparent; border-radius: 10px; font-size: 12px; font-weight: 650; cursor: pointer; transition: transform .18s, background .18s, box-shadow .18s; }
.content-primary-button { color: white; background: linear-gradient(135deg, #4c91dc, #315bb1); box-shadow: 0 5px 14px rgba(47,94,171,.16); }
.content-secondary-button { color: #52677e; background: rgba(255,255,255,.52); border-color: rgba(126,150,176,.2); }
.content-primary-button:hover, .content-secondary-button:hover { transform: translateY(-1px); }
.content-primary-button:disabled, .content-secondary-button:disabled, .content-row-actions button:disabled { opacity: .55; cursor: wait; }
.content-feedback { margin: 0 0 12px; padding: 10px 13px; border-radius: 10px; font-size: 12px; line-height: 1.65; }
.content-feedback-error { color: #8b302c; background: rgba(255,232,229,.8); }
.content-feedback-success { color: #315f4f; background: rgba(225,243,235,.78); }
.content-row-actions { display: flex; align-items: center; gap: 7px; padding: 0 10px; white-space: nowrap; }
.content-row-actions button { padding: 5px 8px; border: 1px solid rgba(126,150,176,.18); border-radius: 7px; color: #315f9e; background: rgba(255,255,255,.48); font-size: 11px; cursor: pointer; }
.content-row-actions .content-danger-action { color: #a34b46; }
.content-version-list { display: grid; gap: 0; border-top: 1px solid rgba(126,150,176,.2); }
.content-version-row { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 16px 4px; border-bottom: 1px solid rgba(126,150,176,.16); }
.content-version-row strong { color: #344b63; font-size: 13px; }
.content-version-row p { margin: 6px 0 0; color: #8190a1; font-size: 11px; }
@media (max-width: 680px) { .content-version-row { align-items: flex-start; flex-direction: column; gap: 10px; } }
@media (max-width: 680px) {
  .content-toolbar-actions { width: 100%; align-items: stretch; flex-direction: column; }
  .content-toolbar-actions .virtual-table-search { width: 100%; }
  .content-form-row { grid-template-columns: 1fr; }
  .content-editor-heading { align-items: flex-start; }
  .content-row-actions { padding: 0 8px; }
}


/* Public content listing and article view */
.published-content-section { padding: 10px clamp(22px, 11vw, 176px) 96px; }
.published-content-heading { display: flex; align-items: end; justify-content: space-between; gap: 18px; margin-bottom: 20px; }
.published-content-heading h2 { margin: 8px 0 0; color: #304760; font-size: clamp(26px, 3vw, 38px); letter-spacing: -.035em; }
.published-content-heading > span { color: #71849a; font-size: 12px; }
.published-content-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.published-content-card { display: flex; min-width: 0; flex-direction: column; padding: 22px; border: 1px solid rgba(51,76,105,.075); border-radius: 18px; color: inherit; text-decoration: none; background: rgba(245,249,255,.42); box-shadow: 0 10px 34px rgba(34,55,83,.06); -webkit-backdrop-filter: blur(22px) saturate(145%); backdrop-filter: blur(22px) saturate(145%); transition: transform .2s, box-shadow .2s, background .2s; }
.published-content-card:hover { transform: translateY(-3px); background: rgba(250,252,255,.65); box-shadow: 0 16px 38px rgba(34,55,83,.11); }
.published-content-type { color: #527caf; font-size: 11px; font-weight: 650; }
.published-content-card h3 { margin: 12px 0 8px; color: #304760; font-size: 18px; line-height: 1.5; overflow-wrap: anywhere; }
.published-content-card p { flex: 1; margin: 0 0 20px; color: #71849a; font-size: 13px; line-height: 1.8; overflow-wrap: anywhere; }
.published-content-link { color: #315f9e; font-size: 12px; font-weight: 650; }
.published-content-empty { padding: 22px; border-radius: 14px; color: #71849a; background: rgba(245,249,255,.35); font-size: 13px; }
.public-content-page { min-height: 76vh; padding: clamp(32px, 7vw, 82px) 24px; }
.public-article { width: min(100%, 900px); margin: 0 auto; }
.public-article-back { display: inline-block; margin-bottom: 36px; color: #527caf; font-size: 13px; text-decoration: none; }
.public-article-type { margin: 0 0 15px; color: #527caf; font-size: 12px; font-weight: 650; }
.public-article-type span { padding: 0 6px; color: #8a98a8; }
.public-article h1 { margin: 0; color: #263e58; font-size: clamp(34px, 5vw, 58px); line-height: 1.25; letter-spacing: -.04em; overflow-wrap: anywhere; }
.public-article-summary { margin: 22px 0 0; color: #62768c; font-size: 17px; line-height: 1.9; }
.public-article-body { margin-top: 38px; padding-top: 30px; border-top: 1px solid rgba(42,66,96,.09); color: #40566e; font-size: 16px; line-height: 1.85; overflow-wrap: anywhere; }
.content-markdown-workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: stretch; gap: 16px; }
.content-markdown-source { min-width: 0; }
.content-markdown-source textarea { min-height: 390px; resize: vertical; font-family: var(--mono); font-size: 13px; tab-size: 2; }
.content-markdown-preview { min-width: 0; min-height: 390px; overflow: auto; border: 1px solid rgba(126,150,176,.2); border-radius: 11px; background: rgba(255,255,255,.5); }
.content-markdown-preview-heading { position: sticky; top: 0; z-index: 1; padding: 12px 14px; border-bottom: 1px solid rgba(126,150,176,.16); color: #71849a; background: rgba(244,248,252,.96); font-size: 12px; font-weight: 650; }
.content-markdown-preview-empty { padding: 18px; color: #8a98a8; font-size: 12px; }
.markdown-rendered { color: #40566e; font-size: 15px; line-height: 1.85; overflow-wrap: anywhere; }
.markdown-rendered > :first-child { margin-top: 0; }
.markdown-rendered > :last-child { margin-bottom: 0; }
.markdown-rendered h1, .markdown-rendered h2, .markdown-rendered h3, .markdown-rendered h4 { margin: 1.45em 0 .65em; color: #2e465f; line-height: 1.4; letter-spacing: -.025em; }
.markdown-rendered h1 { font-size: 1.8em; }
.markdown-rendered h2 { font-size: 1.5em; }
.markdown-rendered h3 { font-size: 1.22em; }
.markdown-rendered p { margin: .8em 0; }
.markdown-rendered ul, .markdown-rendered ol { padding-left: 1.6em; margin: .8em 0; }
.markdown-rendered li + li { margin-top: .25em; }
.markdown-rendered blockquote { margin: 1em 0; padding: .15em 1em; border-left: 3px solid #9ab7d5; color: #687e94; background: rgba(228,238,248,.35); }
.markdown-rendered a { color: #2869ad; text-decoration: underline; text-underline-offset: 3px; }
.markdown-rendered hr { border: 0; border-top: 1px solid rgba(126,150,176,.24); margin: 1.6em 0; }
.markdown-rendered code { padding: .12em .35em; border-radius: 5px; color: #8b4e35; background: rgba(229,235,242,.75); font-family: var(--mono); font-size: .88em; }
.markdown-rendered pre { max-width: 100%; overflow: auto; padding: 15px; border-radius: 9px; color: #e7edf5; background: #243448; line-height: 1.65; }
.markdown-rendered pre code { padding: 0; color: inherit; background: transparent; font-size: .88em; }
.markdown-rendered table { width: 100%; border-collapse: collapse; margin: 1.2em 0; font-size: .9em; }
.markdown-rendered th, .markdown-rendered td { padding: 8px 10px; border: 1px solid rgba(126,150,176,.24); text-align: left; }
.markdown-rendered th { background: rgba(226,236,247,.6); font-weight: 650; }
.markdown-rendered img { max-width: 100%; height: auto; border-radius: 8px; }
.public-content-state { display: flex; width: min(100%, 620px); min-height: 320px; flex-direction: column; align-items: flex-start; justify-content: center; gap: 12px; margin: 0 auto; color: #52677e; }
.public-content-state h1 { margin: 0; color: #304760; font-size: 32px; }
.public-content-state p { margin: 0 0 10px; line-height: 1.8; }
@media (max-width: 900px) { .published-content-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) {
  .published-content-section { padding: 0 20px 60px; }
  .published-content-grid { grid-template-columns: 1fr; gap: 10px; }
  .published-content-heading { align-items: flex-start; flex-direction: column; gap: 8px; }
  .public-content-page { padding: 30px 20px 56px; }
  .public-article-back { margin-bottom: 28px; }
  .public-article-summary { font-size: 15px; }
  .public-article-body { margin-top: 28px; font-size: 15px; }
  .content-markdown-workspace { grid-template-columns: minmax(0, 1fr); }
  .content-markdown-source textarea { min-height: 260px; }
  .content-markdown-preview { min-height: 220px; }
}

</style>