<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, onUpdated, ref, watch } from 'vue'
import VirtualizedSelect from './components/VirtualizedSelect.vue'
import NotificationHost from './components/NotificationHost.vue'
const ComplianceCenter = defineAsyncComponent(() => import('./components/ComplianceCenter.vue'))
const SystemOpsCenter = defineAsyncComponent(() => import('./components/SystemOpsCenter.vue'))
import { portalNotify } from './utils/notification'
import { portalTaxonomy, taxonomyChildOptions } from './utils/taxonomy'
import { matchesKnowledgeTaxonomy } from './utils/knowledgeFilters'
import { resourceFileIcon } from './utils/resourceIcons'
import type { Content as APIContent, ContentPatch, ContentVersion as APIContentVersion, ContentWrite, LoginRequest, RegisterRequest, ResourceWrite, TeachingResource as APITeachingResource, User as APIUser } from './api/generated'
const removedGradeTags = new Set(['年级', '一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '初一', '初二', '初三', '高一', '高二', '高三', '本科', '硕士'])
const knowledgeTagOptions = portalTaxonomy
  .filter(category => category.value !== '年级')
  .map(category => ({ ...category, children: category.children.filter(tag => !removedGradeTags.has(tag)) }))
const taxonomyParentNames = new Set<string>(knowledgeTagOptions.map(category => category.value))
const normalizeKnowledgeTags = (tags: string[]) => {
  const selectedChildren = [...new Set(tags.filter(tag => tag && !removedGradeTags.has(tag) && !taxonomyParentNames.has(tag)))]
  const selectedParents = knowledgeTagOptions
    .filter(category => category.children.some(child => selectedChildren.includes(child)))
    .map(category => category.value)
  return [...selectedParents, ...selectedChildren]
}
const mergeLegacyTaxonomyIntoTags = (tags: string[], parentCategory?: string | null, childCategory?: string | null) =>
  normalizeKnowledgeTags([...tags, parentCategory || '', childCategory || ''])
const visibilityOptions = [{ label: '仅自己可见', value: 'private' }, { label: '公开', value: 'public' }]
const ContentDocumentEditor = defineAsyncComponent(() => import('./components/ContentDocumentEditor.vue'))

type PortalSection = {
  id: string
  number: string
  title: string
  subtitle: string
  description: string
  tags: string[]
}
type PortalUser = APIUser & { isAdmin?: boolean }
type PortalContent = APIContent
type ContentVersion = APIContentVersion
type TeachingResource = APITeachingResource
type PublicTeachingResource = Pick<TeachingResource, 'id' | 'title' | 'description' | 'type' | 'grade' | 'url' | 'filename' | 'fileSize' | 'createdAt' | 'updatedAt'> & { downloadUrl?: string | null }
const selectedEducationResource = ref<PublicTeachingResource | null>(null)
let educationResourcePreviousBodyOverflow = ''
watch(selectedEducationResource, (resource, previous) => {
  if (resource && !previous) {
    educationResourcePreviousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
  } else if (!resource && previous) {
    document.body.style.overflow = educationResourcePreviousBodyOverflow
    educationResourcePreviousBodyOverflow = ''
  }
}, { flush: 'sync' })
function formatResourceFileSize(size?: number | null) {
  if (size == null || !Number.isFinite(size) || size < 0) return ''
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}
function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' })
  document.querySelectorAll<HTMLElement>('.account-content, .account-sidebar, .virtual-table-scroll, .content-workspace-page, .content-outline-sidebar, .content-tools-sidebar, .content-editor-document').forEach((element) => {
    element.scrollTo({ top: 0, behavior: 'smooth' })
  })
}
type ContentForm = {
  title: string
  slug: string
  contentType: PortalContent['contentType']
  tags: string[]
  summary: string
  body: string
  visibility: PortalContent['visibility']
}
type PublicContentCard = Pick<PortalContent, 'id' | 'title' | 'slug' | 'contentType' | 'tags' | 'parentCategory' | 'childCategory' | 'summary' | 'publishedAt'>
type PublicContent = PublicContentCard & Pick<PortalContent, 'body' | 'createdAt' | 'updatedAt'> & { authorName: string; authorJoinedAt: string; likeCount: number; favoriteCount: number; viewerLiked: boolean; viewerFavorited: boolean }

const query = ref('')
const menuOpen = ref(false)
const route = ref(window.location.pathname || '/')
const user = ref<PortalUser | null>(null)
const authLoading = ref(false)
const authBusy = ref(false)
const authError = ref('')
const authNotice = ref('')
const authForm = ref({ displayName: '', email: '', password: '' })
const expandedAccountGroups = ref<string[]>(['personal', 'publishing', 'housing', 'education', 'software', 'workflows', 'compliance', 'system-ops'])
const activeAccountSection = ref('overview')
const sectionRoutes: Record<string, string> = {
  overview: '/account', profile: '/profile', security: '/security',
  'content-management': '/content-management', 'property-management': '/housing/property-management',
  'room-status': '/housing/room-status', 'student-records': '/education/student-records',
  'course-calendar': '/education/course-calendar', 'teaching-resources': '/education/teaching-resources',
  'network-tools': '/tools/network', 'document-converters': '/tools/document-converters',
  'add-tool': '/tools/add', 'workflow-overview': '/workflows', 'copyright-compliance': '/account/compliance',
  'system-warnings': '/account/system-ops/warnings', 'system-logs': '/account/system-ops/logs',
}
const routeSections = Object.fromEntries(Object.entries(sectionRoutes).map(([id, path]) => [path, id])) as Record<string, string>
const routeAliases: Record<string, string> = {
  '/首页': '/', '/个人中心': '/account', '/个人资料': '/profile', '/登录与安全': '/security',
  '/登录': '/login', '/注册': '/register', '/内容管理': '/content-management',
  '/房源管理': '/housing/property-management', '/房态管理': '/housing/room-status',
  '/学生档案': '/education/student-records', '/课程日历': '/education/course-calendar',
  '/教学资源': '/education/teaching-resources', '/网络工具': '/tools/network',
  '/文档转换': '/tools/document-converters', '/添加工具': '/tools/add', '/工作流管理': '/workflows',
}
const contentEditorRouteId = computed(() => route.value.match(/^\/content-management\/([0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})$/i)?.[1] || null)
const isContentEditorPage = computed(() => route.value === '/content-management/new' || contentEditorRouteId.value !== null)
const isProtectedPage = computed(() => isContentEditorPage.value || route.value === '/account' || Object.prototype.hasOwnProperty.call(routeSections, route.value))
const isAccountPage = computed(() => !isContentEditorPage.value && (route.value === '/account' || Object.prototype.hasOwnProperty.call(routeSections, route.value)))
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
  {
    id: 'compliance', label: '治理与合规', items: [
      { id: 'copyright-compliance', label: '门户开发资源合规中心', description: '管理门户自身的软件依赖许可证、静态资源和内部审核记录。' },
    ],
  },
  {
    id: 'system-ops', label: '系统运维', items: [
      { id: 'system-warnings', label: '系统警告', description: '查看服务器异常、失败任务和需要人工处理的运维警告。' },
      { id: 'system-logs', label: '系统日志', description: '实时查看服务器运行日志和 GPT 连接 SSH 的操作审计日志。' },
    ],
  },
]
const accountItems = accountGroups.flatMap((group) => group.items)
const filteredAccountGroups = computed(() => {
  const visibleGroups = accountGroups.filter(group => !['compliance', 'system-ops'].includes(group.id) || user.value?.isAdmin === true)
  const keyword = accountNavQuery.value.trim().toLocaleLowerCase()
  if (!keyword) return visibleGroups
  return visibleGroups
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
    personal: 'bi-person', publishing: 'bi-journal-text', housing: 'bi-house-door',
    education: 'bi-mortarboard', software: 'bi-tools', workflows: 'bi-diagram-3', compliance: 'bi-shield-check', 'system-ops': 'bi-activity',
  }
  return icons[groupId] || 'bi-grid'
}


type TableColumn = { key: string; label: string; width?: string }
type TableRecord = Record<string, string | number>
const tableQuery = ref('')
const debouncedTableQuery = ref('')
let tableSearchDebounceTimer: ReturnType<typeof setTimeout> | undefined
watch(tableQuery, (query) => {
  if (tableSearchDebounceTimer) clearTimeout(tableSearchDebounceTimer)
  tableSearchDebounceTimer = setTimeout(() => { debouncedTableQuery.value = query }, 150)
})
const tableScrollTop = ref(0)
const virtualTableScrollElement = ref<HTMLElement | null>(null)
const virtualTableViewportWidth = ref(0)
let tableResizeObserver: ResizeObserver | null = null
const selectedTableCell = ref<{ label: string; value: string } | null>(null)
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
const teachingResources = ref<TeachingResource[]>([])
const teachingResourceTrash = ref<TeachingResource[]>([])
const resourceViewMode = ref<'active' | 'trash'>('active')
const resourceDialogOpen = ref(false)
const editingResourceId = ref<string | null>(null)
const resourceBusy = ref(false)
const resourceError = ref('')
const resourceTitle = ref('')
const resourceDescription = ref('')
const resourceFile = ref<File | null>(null)
const resourceType = ref('听力材料')
const resourceGrade = ref('')
const resourceIsPublic = ref(false)
const resourceUrl = ref('')
const resourceUploadProgress = ref(0)
const resourceUploadDone = ref(false)
const resourceUploadResult = ref('')
const resourceDialogLocked = computed(() => resourceBusy.value || (resourceUploadDone.value && resourceDialogOpen.value))
const resourceTypeOptions = ['听力材料', '课件', '教学视频', '电子书', '练习题', '教案', '其他']
const resourceGradeOptions = ['不限定年级', ...taxonomyChildOptions('年级').map(option => option.value)]
watch(authError, (message) => { if (message) portalNotify.error(message, '登录或注册失败') }, { flush: 'sync' })
watch(authNotice, (message) => { if (message) portalNotify.info(message) }, { flush: 'sync' })
watch(contentError, (message) => { if (message) portalNotify.error(message, '内容操作失败') }, { flush: 'sync' })
watch(contentNotice, (message) => { if (message) portalNotify.success(message) }, { flush: 'sync' })
const emptyContentForm = (): ContentForm => ({
  title: '', slug: '', contentType: 'article', tags: [], summary: '', body: '', visibility: 'private',
})
const contentForm = ref<ContentForm>(emptyContentForm())
const knowledgeTagDialogOpen = ref(false)
const knowledgeTagTrigger = ref<HTMLButtonElement | null>(null)
const knowledgeTagDialog = ref<HTMLElement | null>(null)
const knowledgeTagDraft = ref<string[]>([])
const expandedKnowledgeTagGroups = ref<string[]>(['教育'])
const selectedKnowledgeTagCount = computed(() => contentForm.value.tags.filter(tag => !taxonomyParentNames.has(tag)).length)
const knowledgeTagDraftCount = computed(() => knowledgeTagDraft.value.filter(tag => !taxonomyParentNames.has(tag)).length)
const isKnowledgeTagSelected = (tag: string) => knowledgeTagDraft.value.includes(tag)
const openKnowledgeTagDialog = async () => {
  if (contentBusy.value) return
  knowledgeTagDraft.value = normalizeKnowledgeTags(contentForm.value.tags)
  knowledgeTagDialogOpen.value = true
  await nextTick()
  knowledgeTagDialog.value?.focus()
}
const closeKnowledgeTagDialog = (apply = false) => {
  if (apply) contentForm.value.tags = normalizeKnowledgeTags(knowledgeTagDraft.value)
  knowledgeTagDialogOpen.value = false
  nextTick(() => knowledgeTagTrigger.value?.focus())
}
const toggleKnowledgeTagGroup = (category: string) => {
  expandedKnowledgeTagGroups.value = expandedKnowledgeTagGroups.value.includes(category)
    ? expandedKnowledgeTagGroups.value.filter(item => item !== category)
    : [...expandedKnowledgeTagGroups.value, category]
}
const toggleKnowledgeTag = (tag: string, checked: boolean) => {
  const selected = knowledgeTagDraft.value.filter(item => !taxonomyParentNames.has(item))
  const next = checked ? [...selected, tag] : selected.filter(item => item !== tag)
  knowledgeTagDraft.value = normalizeKnowledgeTags(next)
}
const clearKnowledgeTags = () => { knowledgeTagDraft.value = [] }
const removeSelectedKnowledgeTag = (tag: string) => {
  contentForm.value.tags = normalizeKnowledgeTags(contentForm.value.tags.filter(item => item !== tag))
}
const clearSelectedKnowledgeTags = () => { contentForm.value.tags = [] }
const publicContents = ref<PublicContentCard[]>([])
const publicTeachingResources = ref<PublicTeachingResource[]>([])
const educationResourceLoading = ref(false)
const educationResourceError = ref('')
const educationResourceCategory = ref('全部资源')
const educationResourceGrade = ref('全部年级')
const educationResourceSearch = ref('')
const educationResourceTypes = ['全部资源', ...resourceTypeOptions]
const educationResourceGrades = ['全部年级', ...taxonomyChildOptions('年级').map(option => option.value)]
const filteredEducationResources = computed(() => publicTeachingResources.value.filter(item => {
  if (educationResourceCategory.value !== '全部资源' && item.type !== educationResourceCategory.value) return false
  if (educationResourceGrade.value !== '全部年级' && item.grade !== educationResourceGrade.value) return false
  const term = educationResourceSearch.value.trim()
  return !term || knowledgeFuzzyMatch(`${item.title} ${item.description || ''} ${item.type} ${item.grade || ''} ${item.filename || ''}`, term)
}).sort((a, b) => new Date(b.updatedAt || b.createdAt).getTime() - new Date(a.updatedAt || a.createdAt).getTime()))
const visibleEducationResources = computed(() => filteredEducationResources.value.slice(0, knowledgeVisibleCount.value))
const knowledgeVisibleCount = ref(10)
const knowledgePageSize = 10
const knowledgeCategory = ref('recent')
const knowledgeSubcategory = ref('')
const knowledgeSearchQuery = ref('')
const highlightedKnowledgeSuggestion = ref(-1)
const knowledgeCategoryMenus = [
  { value: 'recent', label: '最近更新', children: [] },
  { value: 'education', label: '教育', children: ['学前教育', '小学教育', '中学教育', '高等教育', '职业教育', '家庭教育', '教学资源'] },
  { value: 'education-jobs', label: '教育人力资源', children: ['事业编', '国企', '省考', '国考'] },
  { value: 'travel', label: '旅游', children: ['住宿', '交通', '景点攻略', '旅行规划', '美食体验'] },
  { value: 'network', label: '网络', children: ['网络安全', '网络维护', '网络基础知识', '网络诊断', '云与服务器'] },
  { value: 'system', label: '系统', children: ['操作系统', '系统配置', '系统维护', '部署运维', '故障排查'] },
  { value: 'programming', label: '编程', children: ['前端开发', '后端开发', 'Python', 'JavaScript / TypeScript', '数据库', 'API 与集成', '代码实践'] },
  { value: 'software', label: '软件工具', children: ['效率工具', '开发工具', '文档工具', 'AI 工具', '自动化工具'] },
]
function knowledgeEditDistance(left: string, right: string) {
  const previous = Array.from({ length: right.length + 1 }, (_, index) => index)
  for (let i = 1; i <= left.length; i++) {
    const current = [i]
    for (let j = 1; j <= right.length; j++) current[j] = Math.min((current[j - 1] ?? i) + 1, (previous[j] ?? j) + 1, (previous[j - 1] ?? i + j) + ((left[i - 1] ?? '') === (right[j - 1] ?? '') ? 0 : 1))
    for (let j = 0; j < current.length; j++) previous[j] = current[j] ?? j
  }
  return previous[right.length] ?? right.length
}
function knowledgeFuzzyMatch(source: string, query: string) {
  const text = source.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  const term = query.normalize('NFKC').toLocaleLowerCase().replace(/\s+/g, '')
  if (!term || text.includes(term)) return true
  // Character-order matching helps Chinese partial phrases with omitted characters.
  let position = 0
  for (const character of term) {
    position = text.indexOf(character, position)
    if (position < 0) break
    position++
  }
  if (position > 0) {
    let check = 0
    let matched = true
    for (const character of term) { check = text.indexOf(character, check); if (check < 0) { matched = false; break } check++ }
    if (matched) return true
  }
  // Allow small spelling mistakes in longer queries using a bounded-length window.
  const chars = Array.from(text)
  const termChars = Array.from(term)
  if (termChars.length >= 4) {
    const tolerance = termChars.length >= 7 ? 2 : 1
    for (let start = 0; start < chars.length; start++) {
      for (let size = Math.max(1, termChars.length - tolerance); size <= termChars.length + tolerance && start + size <= chars.length; size++) {
        if (knowledgeEditDistance(termChars.join(''), chars.slice(start, start + size).join('')) <= tolerance) return true
      }
    }
  }
  return false
}

const knowledgeFilteredItems = computed(() => {
  const items = [...directoryContentItems.value]
  let filtered = items
  if (knowledgeCategory.value !== 'recent') {
    const selected = knowledgeSubcategory.value || knowledgeCategoryMenus.find(item => item.value === knowledgeCategory.value)?.label || ''
    const terms: Record<string, string[]> = {
      education: ['教育', '学习', '课程', '教学', '学生', '学校'], travel: ['旅游', '旅行', '住宿', '交通', '景点'],
      network: ['网络', '安全', '维护', 'DNS', 'TCP', 'HTTP'], system: ['系统', '部署', '运维', 'Linux', 'FreeBSD', 'Windows'],
      programming: ['编程', '代码', 'Python', 'JavaScript', 'TypeScript', 'API', '数据库'], software: ['软件', '工具', '效率', '自动化', 'AI'],
    }
    const keywords = (knowledgeSubcategory.value ? [knowledgeSubcategory.value] : [selected, ...(terms[knowledgeCategory.value] || [])]).filter(Boolean).map(value => value.toLocaleLowerCase())
    filtered = filtered.filter(item => {
      const category = knowledgeCategoryMenus.find(category => category.value === knowledgeCategory.value)
      const categoryLabel = category?.label || ''
      if (matchesKnowledgeTaxonomy(item, categoryLabel, category?.children || [], knowledgeSubcategory.value)) return true
      if (item.tags?.length || item.parentCategory || item.childCategory) return false
      return keywords.some(keyword => knowledgeFuzzyMatch(`${item.title} ${item.summary || ''}`, keyword))
    })
  }
  const searchTerm = knowledgeSearchQuery.value.trim().toLocaleLowerCase()
  if (searchTerm) filtered = filtered.filter(item => knowledgeFuzzyMatch(`${item.title} ${item.summary || ''}`, searchTerm))
  return filtered.sort((a, b) => new Date(b.publishedAt || 0).getTime() - new Date(a.publishedAt || 0).getTime())
})
const knowledgeSearchSuggestions = computed(() => {
  const term = knowledgeSearchQuery.value.trim().toLocaleLowerCase()
  if (!term) return []
  const themes = knowledgeCategoryMenus.flatMap(category => [
    { label: category.label, kind: 'theme' as const },
    ...category.children.map(child => ({ label: child, kind: 'theme' as const })),
  ])
  const articles = directoryContentItems.value.map(item => ({ label: item.title, kind: 'article' as const, slug: item.slug, summary: item.summary || '' }))
  const matches = [...themes, ...articles].filter(item => knowledgeFuzzyMatch(`${item.label} ${'summary' in item ? item.summary : ''}`, term))
  return matches.filter((item, index) => matches.findIndex(other => other.label === item.label) === index).slice(0, 8)
})
function chooseKnowledgeSuggestion(suggestion: (typeof knowledgeSearchSuggestions.value)[number]) {
  if (suggestion.kind === 'article' && 'slug' in suggestion) {
    navigate(`/content/${suggestion.slug}`)
    return
  }
  const childCategory = knowledgeCategoryMenus.find(category => category.children.includes(suggestion.label))
  const parentCategory = knowledgeCategoryMenus.find(category => category.label === suggestion.label)
  if (childCategory) selectKnowledgeCategory(childCategory.value, suggestion.label)
  else if (parentCategory) selectKnowledgeCategory(parentCategory.value)
  knowledgeSearchQuery.value = ''
  knowledgeVisibleCount.value = knowledgePageSize
}
function queryKnowledgeSearch(queryString: string, callback: (results: Array<Record<string, unknown>>) => void) {
  const results = knowledgeSearchSuggestions.value
    .map(suggestion => ({ ...suggestion, value: suggestion.label }))
    .filter(suggestion => !queryString.trim() || knowledgeFuzzyMatch(`${suggestion.value} ${'summary' in suggestion ? suggestion.summary : ''}`, queryString))
  callback(results)
}
function handleKnowledgeAutocompleteSelect(item: { value: string; kind: 'theme' | 'article' }) {
  const suggestion = knowledgeSearchSuggestions.value.find(candidate => candidate.label === item.value && candidate.kind === item.kind)
  if (suggestion) chooseKnowledgeSuggestion(suggestion)
  highlightedKnowledgeSuggestion.value = -1
}
function onKnowledgeSearchKeydown(event: KeyboardEvent) {
  const suggestions = knowledgeSearchSuggestions.value
  if (event.key === 'ArrowDown' && suggestions.length) {
    event.preventDefault()
    highlightedKnowledgeSuggestion.value = (highlightedKnowledgeSuggestion.value + 1) % suggestions.length
  } else if (event.key === 'ArrowUp' && suggestions.length) {
    event.preventDefault()
    highlightedKnowledgeSuggestion.value = highlightedKnowledgeSuggestion.value <= 0 ? suggestions.length - 1 : highlightedKnowledgeSuggestion.value - 1
  } else if (event.key === 'Enter' && highlightedKnowledgeSuggestion.value >= 0 && suggestions[highlightedKnowledgeSuggestion.value]) {
    event.preventDefault()
    chooseKnowledgeSuggestion(suggestions[highlightedKnowledgeSuggestion.value]!)
    highlightedKnowledgeSuggestion.value = -1
  } else if (event.key === 'Escape') {
    highlightedKnowledgeSuggestion.value = -1
  }
}
function submitKnowledgeSearch() {
  knowledgeVisibleCount.value = knowledgePageSize
}

const visibleDirectoryContentItems = computed(() => knowledgeFilteredItems.value.slice(0, knowledgeVisibleCount.value))
function syncKnowledgeFilterToUrl() {
  if (route.value !== '/knowledge') return
  const url = new URL(window.location.href)
  if (knowledgeCategory.value === 'recent') url.searchParams.delete('category')
  else url.searchParams.set('category', knowledgeCategory.value)
  if (knowledgeSubcategory.value) url.searchParams.set('subcategory', knowledgeSubcategory.value)
  else url.searchParams.delete('subcategory')
  history.replaceState(history.state, '', `${url.pathname}${url.search}${url.hash}`)
}
function selectKnowledgeCategory(category: string, subcategory = '') {
  knowledgeCategory.value = category
  knowledgeSubcategory.value = subcategory
  knowledgeVisibleCount.value = knowledgePageSize
  syncKnowledgeFilterToUrl()
}
const selectedKnowledgeCategoryLabel = computed(() => {
  if (knowledgeSubcategory.value) return knowledgeSubcategory.value
  return knowledgeCategoryMenus.find(category => category.value === knowledgeCategory.value)?.label || '最近更新'
})
function chooseKnowledgeCategory(categoryValue: string, subcategory = '') {
  selectKnowledgeCategory(categoryValue, subcategory)
}
function scrollKnowledgeCategories(direction: number) {
  const carousel = document.querySelector('.directory-tile-carousel') as HTMLElement | null
  if (carousel) carousel.scrollBy({ left: direction * Math.max(260, carousel.clientWidth * 0.78), behavior: 'smooth' })
}
function loadMoreKnowledge() {
  if (route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length) {
    knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, knowledgeFilteredItems.value.length)
  } else if (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length) {
    knowledgeVisibleCount.value = Math.min(knowledgeVisibleCount.value + knowledgePageSize, filteredEducationResources.value.length)
  }
}
function isKnowledgeListNearViewportEnd() {
  if (route.value !== '/knowledge' && route.value !== '/education-resources') return false
  const list = document.querySelector<HTMLElement>('.knowledge-infinite-list')
  if (!list) return false
  return list.getBoundingClientRect().bottom <= window.innerHeight + 400
}
function onKnowledgePageScroll() {
  if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length) || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length)) && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
}
function ensureKnowledgeListFilled() {
  // Keep the first screen populated without creating a nested scroll container.
  if (((route.value === '/knowledge' && knowledgeVisibleCount.value < knowledgeFilteredItems.value.length) || (route.value === '/education-resources' && knowledgeVisibleCount.value < filteredEducationResources.value.length)) && isKnowledgeListNearViewportEnd()) loadMoreKnowledge()
}
onUpdated(ensureKnowledgeListFilled)
const publicArticle = ref<PublicContent | null>(null)
const publicArticleHtml = ref('')
let publicArticleRequestSequence = 0
const publicContentLoading = ref(false)
const publicContentError = ref('')
const publicInteractionBusy = ref(false)
const shareDialogOpen = ref(false)
const shareUrl = ref('')
watch(publicContentError, (message) => { if (message) portalNotify.error(message, '内容读取失败') }, { flush: 'sync' })
const virtualTableRowHeight = 44
const virtualTableViewportHeight = 440
const virtualTableOverscan = 6
const tableDefinitions: Record<string, { title: string; columns: TableColumn[]; note: string }> = {
  'content-management': { title: '内容列表', columns: [{ key: 'title', label: '内容标题', width: 'minmax(220px, 2fr)' }, { key: 'type', label: '类型' }, { key: 'status', label: '状态' }, { key: 'visibility', label: '可见范围', width: 'minmax(120px, 1fr)' }, { key: 'updatedAt', label: '更新时间' }, { key: 'likeCount', label: '点赞' }, { key: 'favoriteCount', label: '收藏' }, { key: 'actions', label: '操作', width: 'minmax(330px, 2.6fr)' }], note: '内容数据源尚未接入。' },
  'property-management': { title: '房源列表', columns: [{ key: 'property', label: '房源名称', width: 'minmax(180px, 1.6fr)' }, { key: 'address', label: '地址', width: 'minmax(180px, 2fr)' }, { key: 'roomCount', label: '房间数' }, { key: 'rent', label: '租金' }, { key: 'status', label: '房源状态' }, { key: 'updatedAt', label: '更新时间' }], note: '房源数据源尚未接入。' },
  'room-status': { title: '房态列表', columns: [{ key: 'room', label: '房间', width: 'minmax(140px, 1.3fr)' }, { key: 'property', label: '所属房源', width: 'minmax(180px, 1.7fr)' }, { key: 'status', label: '当前房态' }, { key: 'tenant', label: '租客' }, { key: 'checkIn', label: '入住日期' }, { key: 'checkOut', label: '退租日期' }], note: '房态数据源尚未接入。' },
  'student-records': { title: '学生档案', columns: [{ key: 'name', label: '学生姓名' }, { key: 'studentId', label: '学员编号' }, { key: 'course', label: '课程' }, { key: 'status', label: '学习状态' }, { key: 'updatedAt', label: '更新时间' }], note: '学生档案数据源尚未接入。' },
  'course-calendar': { title: '课程安排', columns: [{ key: 'course', label: '课程名称', width: 'minmax(180px, 1.6fr)' }, { key: 'class', label: '班级' }, { key: 'teacher', label: '教师' }, { key: 'startAt', label: '开始时间' }, { key: 'endAt', label: '结束时间' }, { key: 'status', label: '状态' }], note: '课程日历数据源尚未接入。' },
  'teaching-resources': { title: '教学资源', columns: [{ key: 'title', label: '资源名称', width: 'minmax(200px, 2fr)' }, { key: 'filename', label: '文件名称', width: 'minmax(180px, 1.5fr)' }, { key: 'type', label: '类型' }, { key: 'grade', label: '年级' }, { key: 'visibility', label: '公开状态' }, { key: 'description', label: '资源说明', width: 'minmax(220px, 2fr)' }, { key: 'updatedAt', label: '更新时间' }, { key: 'actions', label: '操作' }], note: '教学资源已接入数据库。' },
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
      type: item.tags?.length ? item.tags.join(' · ') : contentTypeLabels[item.contentType],
      status: contentStatusLabels[item.status],
      visibility: item.visibility === 'public' ? '公开' : '仅自己可见',
      updatedAt: new Date(contentViewMode.value === 'trash' && 'deletedAt' in item ? String(item.deletedAt) : item.updatedAt).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' }),
      likeCount: item.likeCount ?? 0,
      favoriteCount: item.favoriteCount ?? 0,
      actions: contentViewMode.value === 'trash' ? '已删除' : item.status === 'published' ? '已发布' : '待发布',
    }))
  : activeAccountSection.value === 'teaching-resources'
    ? (resourceViewMode.value === 'trash' ? teachingResourceTrash.value : teachingResources.value).map((item) => ({ id: item.id, title: item.title, filename: item.filename || '—', type: item.type || '其他', grade: item.grade || '不限定年级', visibility: item.isPublic ? '公开' : '仅自己可见', description: item.description || '—', updatedAt: new Date(resourceViewMode.value === 'trash' ? (item.deletedAt || item.createdAt) : (item.updatedAt || item.createdAt)).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' }), actions: resourceViewMode.value === 'trash' ? '已删除' : '可用' }))
    : [])
// Build each record's normalized search text once per data change, rather than
// converting every cell to lowercase on every keystroke. Action/status helper fields
// are excluded because they are UI-only and do not add useful search content.
const searchableTableRecords = computed<TableRecord[]>(() => tableRecords.value.map((record): TableRecord => ({
  ...record,
  _searchText: Object.entries(record)
    .filter(([key, value]) => key !== 'actions' && (typeof value === 'string' || typeof value === 'number'))
    .map(([, value]) => String(value))
    .join(' ')
    .toLocaleLowerCase(),
})))
const filteredTableRecords = computed(() => {
  const keyword = debouncedTableQuery.value.trim().toLocaleLowerCase()
  if (!keyword) return searchableTableRecords.value
  return searchableTableRecords.value.filter((record) => String(record['_searchText'] || '').includes(keyword))
})
const virtualTableStart = computed(() => Math.max(0, Math.floor(tableScrollTop.value / virtualTableRowHeight) - virtualTableOverscan))
const virtualTableEnd = computed(() => Math.min(filteredTableRecords.value.length, Math.ceil((tableScrollTop.value + virtualTableViewportHeight) / virtualTableRowHeight) + virtualTableOverscan))
const visibleTableRecords = computed(() => filteredTableRecords.value.slice(virtualTableStart.value, virtualTableEnd.value))
function tableCellValue(record: TableRecord, columnKey: string): string {
  return String(record[columnKey] || (columnKey === 'title' ? '未命名内容' : '—'))
}
function tableCellPreview(value: string): string {
  const chars = Array.from(value)
  return chars.length > 10 ? chars.slice(0, 10).join('') + '…' : value
}
function openTableCell(label: string, value: string) {
  selectedTableCell.value = { label, value }
}
const tableColumnTemplate = computed(() => {
  const columns = activeTableDefinition.value?.columns || []
  if (!columns.length) return '100%'
  // Virtualization limits rendered rows, but column sizing must also avoid scanning
  // the entire dataset. A bounded sample keeps widths useful without doing O(rows × columns)
  // string processing every time the table data or search results change.
  const records = filteredTableRecords.value.slice(0, 20)
  const available = Math.max(320, virtualTableViewportWidth.value || window.innerWidth * 0.75)
  const actionIndex = columns.findIndex((column) => column.key === 'actions')
  const widths = columns.map((column) => {
    if (column.key === 'actions') return 330
    let maxChars = Math.max(1, Array.from(column.label).length)
    for (const record of records) {
      const preview = tableCellPreview(tableCellValue(record, column.key))
      maxChars = Math.max(maxChars, Array.from(preview).length)
    }
    return Math.max(72, maxChars * 14 + 32)
  })
  const naturalTotal = widths.reduce((sum, width) => sum + width, 0)
  if (naturalTotal > available) {
    const cap = Math.max(72, Math.floor(available * 0.15))
    return widths.map((width, index) => index === actionIndex ? 'minmax(330px, max-content)' : `${Math.min(width, cap)}px`).join(' ')
  }
  const flexibleWidth = Math.max(0, available - (actionIndex >= 0 ? (widths[actionIndex] ?? 0) : 0))
  const naturalFlexibleTotal = widths.reduce((sum, width, index) => sum + (index === actionIndex ? 0 : width), 0)
  return widths.map((width, index) => {
    if (index === actionIndex) return 'minmax(330px, max-content)'
    const proportional = naturalFlexibleTotal ? flexibleWidth * width / naturalFlexibleTotal : flexibleWidth / columns.length
    return `${Math.max(72, Math.floor(proportional))}px`
  }).join(' ')
})
function observeVirtualTableWidth() {
  tableResizeObserver?.disconnect()
  const element = virtualTableScrollElement.value
  if (!element) { virtualTableViewportWidth.value = 0; return }
  virtualTableViewportWidth.value = element.clientWidth
  tableResizeObserver = new ResizeObserver((entries) => {
    const entry = entries[0]
    if (entry) virtualTableViewportWidth.value = Math.floor(entry.contentRect.width)
  })
  tableResizeObserver.observe(element)
}
watch(activeTableDefinition, async () => {
  await nextTick()
  observeVirtualTableWidth()
})
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
async function loadTeachingResources() {
  resourceBusy.value = true; resourceError.value = ''
  try {
    const trash = resourceViewMode.value === 'trash'
    const response = await fetch(trash ? '/api/resources/trash' : '/api/resources', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: TeachingResource[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取教学资源失败。')
    if (trash) teachingResourceTrash.value = result.items || []; else teachingResources.value = result.items || []
  } catch (error) { resourceError.value = error instanceof Error ? error.message : '读取教学资源失败。'; portalNotify.error(resourceError.value, '教学资源') }
  finally { resourceBusy.value = false }
}
function resetResourceUpload() { resourceUploadProgress.value = 0; resourceUploadDone.value = false; resourceUploadResult.value = '' }
function openResourceDialog() { editingResourceId.value = null; resourceTitle.value = ''; resourceDescription.value = ''; resourceType.value = '听力材料'; resourceGrade.value = ''; resourceIsPublic.value = false; resourceUrl.value = ''; resourceFile.value = null; resourceError.value = ''; resetResourceUpload(); resourceDialogOpen.value = true }
function openResourceEdit(item: TeachingResource) { editingResourceId.value = item.id; resourceTitle.value = item.title; resourceDescription.value = item.description || ''; resourceType.value = item.type || '其他'; resourceGrade.value = item.grade || ''; resourceIsPublic.value = item.isPublic === true; resourceUrl.value = item.url || ''; resourceFile.value = null; resourceError.value = ''; resetResourceUpload(); resourceDialogOpen.value = true }
function openResourceEditById(id: string) { const item = teachingResources.value.find((resource) => resource.id === id); if (item) openResourceEdit(item) }
function requestWithUploadProgress(url: string, method: string, body: ResourceWrite): Promise<{ response: number; result: { item?: TeachingResource; error?: string } }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest(); xhr.open(method, url); xhr.withCredentials = true; xhr.setRequestHeader('Content-Type', 'application/json'); xhr.setRequestHeader('Accept', 'application/json')
    xhr.upload.onprogress = (event) => { if (event.lengthComputable) resourceUploadProgress.value = Math.min(99, Math.round(event.loaded / event.total * 100)) }
    xhr.onerror = () => reject(new Error('网络连接失败，资源未能保存。'))
    xhr.onload = () => { let result: { item?: TeachingResource; error?: string } = {}; try { result = JSON.parse(xhr.responseText) } catch {} if (xhr.status < 200 || xhr.status >= 300) { reject(new Error(result.error || '保存资源失败。')); return } resourceUploadProgress.value = 100; resolve({ response: xhr.status, result }) }
    xhr.send(JSON.stringify(body))
  })
}
async function createTeachingResource() {
  if (!resourceTitle.value.trim()) { resourceError.value = '请填写资源名称。'; return }
  if (!resourceFile.value && !resourceUrl.value.trim() && !editingResourceId.value) { resourceError.value = '请上传文件或粘贴资源 URL 链接。'; return }
  if (resourceUrl.value.trim() && !/^https?:\/\//i.test(resourceUrl.value.trim())) { resourceError.value = '资源 URL 必须以 http:// 或 https:// 开头。'; return }
  if (resourceFile.value && resourceFile.value.size > 25 * 1024 * 1024) { resourceError.value = '单个文件大小不能超过 25 MB。'; return }
  resourceBusy.value = true; resourceError.value = ''; resetResourceUpload()
  try {
    const file = resourceFile.value
    let data = ''
    if (file) data = await new Promise<string>((resolve, reject) => { const reader = new FileReader(); reader.onerror = () => reject(new Error('读取文件失败。')); reader.onload = () => resolve(String(reader.result).split(',')[1] || ''); reader.readAsDataURL(file) })
    const payload: ResourceWrite = { title: resourceTitle.value.trim(), description: resourceDescription.value.trim(), type: resourceType.value, grade: (resourceGrade.value || null) as ResourceWrite['grade'], isPublic: resourceIsPublic.value, url: resourceUrl.value.trim() || null, ...(file ? { filename: file.name, data } : {}) }
    const endpoint = editingResourceId.value ? `/api/resources/${encodeURIComponent(editingResourceId.value)}` : '/api/resources'
    const { result } = await requestWithUploadProgress(endpoint, editingResourceId.value ? 'PATCH' : 'POST', payload)
    if (!result.item) throw new Error('服务器未返回已保存的资源。')
    if (editingResourceId.value) teachingResources.value = teachingResources.value.map((item) => item.id === result.item!.id ? result.item! : item)
    else teachingResources.value = [result.item, ...teachingResources.value]
    resourceUploadDone.value = true; resourceUploadResult.value = editingResourceId.value ? '资源已更新。请点击下方“完成”关闭窗口。' : '资源已保存。请点击下方“完成”关闭窗口。'
  } catch (error) { resourceError.value = error instanceof Error ? error.message : '保存资源失败。'; resourceUploadProgress.value = 0 }
  finally { resourceBusy.value = false }
}
function finishResourceUpload() { if (!resourceUploadDone.value) return; resourceDialogOpen.value = false; editingResourceId.value = null; resetResourceUpload(); portalNotify.success('教学资源已保存。') }
async function changeTeachingResource(id: string, action: 'trash' | 'restore' | 'permanent') {
  const message = action === 'permanent' ? '永久删除后将同时删除服务器文件，无法恢复。确定继续？' : action === 'trash' ? '将该资源移入回收站？' : ''
  if (message && !window.confirm(message)) return
  resourceBusy.value = true
  try {
    const suffix = action === 'restore' ? '/restore' : action === 'permanent' ? '/permanent' : ''
    const response = await fetch(`/api/resources/${encodeURIComponent(id)}${suffix}`, { method: action === 'restore' ? 'POST' : 'DELETE', credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { error?: string }
    if (!response.ok) throw new Error(result.error || '资源操作失败。')
    await loadTeachingResources(); portalNotify.success(action === 'trash' ? '已移入回收站。' : action === 'restore' ? '资源已恢复。' : '资源及文件已永久删除。')
  } catch (error) { portalNotify.error(error instanceof Error ? error.message : '资源操作失败。', '教学资源') }
  finally { resourceBusy.value = false }
}
function switchResourceView() { resourceViewMode.value = resourceViewMode.value === 'active' ? 'trash' : 'active'; tableQuery.value = ''; void loadTeachingResources() }
function downloadTeachingResource(id: string) { const item = teachingResources.value.find((resource) => resource.id === id); if (item?.url) window.open(item.url, '_blank', 'noopener,noreferrer'); else window.location.href = `/api/resources/${encodeURIComponent(id)}/download` }
function formatFileSize(size: number) { return size < 1024 * 1024 ? `${Math.max(1, Math.round(size / 1024))} KB` : `${(size / (1024 * 1024)).toFixed(1)} MB` }
const resourceFileExtensions = new Set(['ppt', 'pptx', 'mp3', 'pdf', 'doc', 'docx', 'txt'])
function handleResourcePaste(event: ClipboardEvent) {
  if (resourceBusy.value || resourceUploadDone.value) return
  const clipboard = event.clipboardData
  if (!clipboard) return
  const files = Array.from(clipboard.files || [])
  if (!files.length) {
    for (const item of Array.from(clipboard.items || [])) {
      if (item.kind !== 'file') continue
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }
  if (!files.length) return
  event.preventDefault()
  const file = files[0]
  if (!file) return
  const extension = file.name.split('.').pop()?.toLowerCase() || ''
  if (!resourceFileExtensions.has(extension)) {
    resourceError.value = '粘贴的文件格式不受支持。请使用 PPT、PPTX、MP3、PDF、DOC、DOCX 或 TXT。'
    return
  }
  if (file.size > 25 * 1024 * 1024) {
    resourceError.value = '单个文件大小不能超过 25 MB。'
    return
  }
  resourceFile.value = file
  resourceError.value = ''
}

async function initializeContentEditorRoute() {
  const id = contentEditorRouteId.value
  contentError.value = ''
  contentEditorOpen.value = false
  if (!id) {
    editingContentId.value = null
    contentForm.value = emptyContentForm()
    return
  }
  if (editingContentId.value === id && contentForm.value.title) return
  contentBusy.value = true
  try {
    const response = await fetch('/api/content/' + encodeURIComponent(id), { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { item?: PortalContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '读取内容详情失败。')
    const item = result.item
    editingContentId.value = item.id
    contentForm.value = { title: item.title, slug: item.slug, contentType: item.contentType, tags: mergeLegacyTaxonomyIntoTags(item.tags || [], item.parentCategory, item.childCategory), summary: item.summary, body: item.body, visibility: item.visibility }
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '读取内容详情失败。'
  } finally { contentBusy.value = false }
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
  const target = sectionRoutes[id] || '/account'
  if (route.value !== target) void navigate(target)
  else if (id === 'content-management') void loadContentList()
  else if (id === 'teaching-resources') void loadTeachingResources()
}
function openNewContent() {
  editingContentId.value = null
  contentForm.value = emptyContentForm()
  contentError.value = ''
  contentNotice.value = ''
  contentEditorOpen.value = false
  void navigate('/content-management/new')
}
function returnToContentList() {
  contentEditorOpen.value = false
  editingContentId.value = null
  contentError.value = ''
  void navigate('/content-management')
}
function cancelContentEditor() {
  if (contentBusy.value) return
  if ((contentForm.value.title.trim() || contentForm.value.body.trim() || contentForm.value.summary.trim()) && !window.confirm('当前内容尚未保存，确定取消并返回内容列表吗？')) return
  contentEditorOpen.value = false
  contentError.value = ''
  void navigate('/content-management')
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
      title: item.title, slug: item.slug, contentType: item.contentType, tags: mergeLegacyTaxonomyIntoTags(item.tags || [], item.parentCategory, item.childCategory),
      summary: item.summary, body: item.body, visibility: item.visibility,
    }
    contentEditorOpen.value = false
    await navigate('/content-management/' + encodeURIComponent(item.id))
  } catch (error) {
    contentError.value = error instanceof Error ? error.message : '读取内容详情失败。'
  } finally {
    contentBusy.value = false
  }
}
async function saveContent() {
  if (!contentForm.value.title.trim()) { contentError.value = '请填写内容标题。'; return }
  if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(contentForm.value.slug.trim())) { contentError.value = '请填写有效的 URL 别名，仅允许小写英文字母、数字和连字符。'; return }
  if (!contentForm.value.body.trim()) { contentError.value = '正文不能为空。'; return }
  if (!contentForm.value.tags.length) { contentError.value = '请至少选择一个知识广场标签。'; return }
  contentBusy.value = true
  contentError.value = ''
  contentNotice.value = ''
  try {
    const editing = editingContentId.value
    const payload: ContentWrite | ContentPatch = {
      title: contentForm.value.title.trim(), slug: contentForm.value.slug.trim(), contentType: contentForm.value.contentType,
      tags: mergeLegacyTaxonomyIntoTags(contentForm.value.tags), parentCategory: null, childCategory: null, summary: contentForm.value.summary, body: contentForm.value.body, visibility: contentForm.value.visibility,
    }
    const response = await fetch(editing ? `/api/content/${encodeURIComponent(editing)}` : '/api/content', {
      method: editing ? 'PATCH' : 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(payload),
    })
    const result = await response.json() as { item?: PortalContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '保存内容失败。')
    if (!editing) editingContentId.value = result.item.id
    contentEditorOpen.value = false
    contentNotice.value = editing ? '内容已保存。' : '草稿已创建。'
    await navigate('/content-management')
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
const isLegalPage = computed(() => /^\/legal\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value))
const legalSlug = computed(() => isLegalPage.value ? route.value.slice('/legal/'.length) : '')
const publishedLegalDocuments = ref<Array<{ slug: string; title: string; documentType: string }>>([])
const legalDocument = ref<{ slug: string; title: string; bodyMarkdown: string; effectiveAt?: string | null } | null>(null)
const legalDocumentHtml = ref('')
const legalDocumentLoading = ref(false)
const legalDocumentError = ref('')
let legalDocumentRequestSequence = 0
const directoryPages: Record<string, { eyebrow: string; title: string; subtitle: string; description: string }> = {
  '/knowledge': { eyebrow: 'KNOWLEDGE & IDEAS', title: '知识广场', subtitle: '技术、经验、分析与教学', description: '汇集技术实践、经验分享、行业分析和教学文章，把值得留下的知识整理成可以持续查阅的内容。' },
  '/education-resources': { eyebrow: 'LEARNING LIBRARY', title: '教育资源', subtitle: '让学习资料更容易找到', description: '集中整理课件讲义、英语听力、网课视频链接和其他学习资料。' },
  '/homestays': { eyebrow: 'STAYS & HOMES', title: '民宿房源', subtitle: '发现值得分享的住宿空间', description: '用于展示民宿与短租房源，后续可逐步补充房源图片、位置、价格、入住说明和预订方式。' },
  '/software-tools': { eyebrow: 'SOFTWARE & TOOLS', title: '软件工具', subtitle: '把实用工具集中起来', description: '整理在线工具、桌面软件、开发运维工具、文档处理服务和自动化资源。' },
}
const directoryTiles: Record<string, Array<{ number: string; title: string; description: string; tags: string[] }>> = {
  '/knowledge': [
    { number: '01', title: '技术实践', description: '开发笔记、系统架构、部署经验与问题排查。', tags: ['编程开发', '系统运维', '实践复盘'] },
    { number: '02', title: '经验分享', description: '工具用法、工作方法与值得复用的经验。', tags: ['经验总结', '效率方法', '资源推荐'] },
    { number: '03', title: '行业分析', description: '围绕行业变化、商业逻辑和具体问题展开分析。', tags: ['行业观察', '案例分析', '商业思考'] },
    { number: '04', title: '教学文章', description: '教程、操作指南、学习路径与知识讲解。', tags: ['入门教程', '操作指南', '学习笔记'] },
  ],
  '/education-resources': [
    { number: '01', title: '课件与讲义', description: '课堂课件、教学讲义、练习题和复习材料。', tags: ['PPT', '讲义', '练习题'] },
    { number: '02', title: '听力材料', description: '英语及其他语言的听力音频、文本和训练素材。', tags: ['英语听力', '音频', '听力文本'] },
    { number: '03', title: '网课视频', description: '整理公开课程、系列教学视频和在线学习入口。', tags: ['课程链接', '系列课程', '视频学习'] },
    { number: '04', title: '学习资料', description: '电子书、参考资料、学习清单和辅助工具。', tags: ['电子资料', '学习清单', '参考链接'] },
  ],
  '/homestays': [
    { number: '01', title: '城市民宿', description: '适合城市旅行、短期出差和周末出行的住宿空间。', tags: ['城市出行', '短期住宿', '交通便利'] },
    { number: '02', title: '乡村与度假', description: '适合休闲度假、亲近自然和慢节奏旅行的房源。', tags: ['乡村体验', '度假住宿', '周边游'] },
    { number: '03', title: '短租房源', description: '按入住周期整理的短租与灵活住宿信息。', tags: ['短租', '入住周期', '配套设施'] },
    { number: '04', title: '房源信息说明', description: '后续房源详情可展示位置、价格、设施、规则和联系预订方式。', tags: ['房源图片', '价格说明', '预订方式'] },
  ],
  '/software-tools': [
    { number: '01', title: '在线效率工具', description: '无需复杂安装即可使用的在线服务与小工具。', tags: ['在线工具', '效率提升', '实用网站'] },
    { number: '02', title: '开发与运维', description: '面向开发、调试、部署和系统管理的工具。', tags: ['开发工具', '服务器', '网络诊断'] },
    { number: '03', title: '文档与格式转换', description: '处理 PDF、图片、文本、表格和常见文件格式。', tags: ['文档处理', '格式转换', '文件管理'] },
    { number: '04', title: 'AI 与自动化', description: '人工智能服务、工作流平台和重复任务自动化工具。', tags: ['AI 服务', '自动化', '工作流'] },
  ],
}
const isPublicDirectoryPage = computed(() => Object.prototype.hasOwnProperty.call(directoryPages, route.value))
const currentDirectoryPage = computed(() => directoryPages[route.value] || directoryPages['/knowledge']!)
const currentDirectoryTiles = computed(() => directoryTiles[route.value] || [])
const directoryContentItems = computed(() => {
  const contentType = route.value === '/knowledge' ? 'article' : route.value === '/education-resources' ? 'resource' : route.value === '/software-tools' ? 'tool' : null
  return contentType ? publicContents.value.filter((item) => item.contentType === contentType) : []
})
const directoryContentHeading = computed(() => route.value === '/knowledge' ? '最新文章' : route.value === '/education-resources' ? '已发布的学习资料' : '已发布的工具')

function closeMenu() { menuOpen.value = false }
function setRouteFromLocation() {
  const rawPath = decodeURI(window.location.pathname || '/')
  const pathname = rawPath.length > 1 ? rawPath.replace(/\/+$/, '') : rawPath
  route.value = routeAliases[pathname] || pathname
  if (route.value === '/knowledge') {
    const params = new URLSearchParams(window.location.search)
    const requestedCategory = params.get('category') || 'recent'
    const validCategory = knowledgeCategoryMenus.some(category => category.value === requestedCategory)
    const category = validCategory ? requestedCategory : 'recent'
    const requestedSubcategory = params.get('subcategory') || ''
    const categoryDefinition = knowledgeCategoryMenus.find(item => item.value === category)
    knowledgeCategory.value = category
    knowledgeSubcategory.value = categoryDefinition?.children.includes(requestedSubcategory) ? requestedSubcategory : ''
    knowledgeVisibleCount.value = knowledgePageSize
  }
  if (isAccountPage.value) {
    const section = routeSections[route.value] || 'overview'
    activeAccountSection.value = section
    activeSidebarButton.value = section
  }
}
const scrollStorageKey = (path = route.value) => `portal:scroll:${path}`
function saveScrollPosition() {
  try {
    const read = (selector: string) => (document.querySelector(selector) as HTMLElement | null)?.scrollTop || 0
    sessionStorage.setItem(scrollStorageKey(), JSON.stringify({ windowY: window.scrollY, accountContent: read('.account-content'), accountSidebar: read('.account-sidebar'), table: read('.virtual-table-scroll') }))
  } catch { /* Storage may be unavailable. */ }
}
async function restoreScrollPosition() {
  await nextTick()
  try {
    const saved = sessionStorage.getItem(scrollStorageKey())
    if (!saved) return
    const state = JSON.parse(saved) as { windowY?: number; accountContent?: number; accountSidebar?: number; table?: number }
    window.scrollTo({ top: state.windowY || 0, behavior: 'auto' })
    const write = (selector: string, value?: number) => { const el = document.querySelector(selector) as HTMLElement | null; if (el && typeof value === 'number') el.scrollTop = value }
    write('.account-content', state.accountContent); write('.account-sidebar', state.accountSidebar); write('.virtual-table-scroll', state.table)
  } catch { /* Ignore malformed state. */ }
}
function onScrollCapture() { saveScrollPosition(); onKnowledgePageScroll() }
async function loadPublicContents() {
  publicArticleRequestSequence += 1
  publicContentLoading.value = true
  publicContentError.value = ''
  try {
    const response = await fetch('/api/public/content', { headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: PublicContentCard[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取公开内容失败。')
    publicContents.value = result.items || []
    knowledgeVisibleCount.value = knowledgePageSize
  } catch (error) {
    publicContentError.value = error instanceof Error ? error.message : '读取公开内容失败。'
  } finally {
    publicContentLoading.value = false
  }
}
async function loadPublicTeachingResources() {
  educationResourceLoading.value = true
  educationResourceError.value = ''
  try {
    const response = await fetch('/api/public/resources', { headers: { Accept: 'application/json' } })
    const result = await response.json() as { items?: PublicTeachingResource[]; error?: string }
    if (!response.ok) throw new Error(result.error || '读取公开教育资源失败。')
    publicTeachingResources.value = result.items || []
    knowledgeVisibleCount.value = knowledgePageSize
  } catch (error) {
    educationResourceError.value = error instanceof Error ? error.message : '读取公开教育资源失败。'
  } finally { educationResourceLoading.value = false }
}
async function loadPublicArticle(slug: string) {
  const requestId = ++publicArticleRequestSequence
  const requestedRoute = route.value
  const apiPath = `/api/public/content/${encodeURIComponent(slug)}`
  publicContentLoading.value = true
  publicContentError.value = ''
  publicArticle.value = null
  publicArticleHtml.value = ''
  try {
    const response = await fetch(apiPath, { credentials: 'same-origin', headers: { Accept: 'application/json' } })
    const result = await response.json() as { item?: PublicContent; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '内容不存在或尚未公开。')
    if (requestId !== publicArticleRequestSequence || route.value !== requestedRoute) return
    const { renderMarkdown } = await import('./utils/markdown')
    if (requestId !== publicArticleRequestSequence || route.value !== requestedRoute) return
    publicArticleHtml.value = renderMarkdown(result.item.body)
    publicArticle.value = result.item
  } catch (error) {
    if (requestId === publicArticleRequestSequence) publicContentError.value = error instanceof Error ? error.message : '读取内容失败。'
  } finally {
    if (requestId === publicArticleRequestSequence) publicContentLoading.value = false
  }
}
async function loadPublishedLegalDocuments() {
  try {
    const response = await fetch('/api/public/legal', { headers: { Accept: 'application/json' } })
    if (!response.ok) return
    const result = await response.json() as { items?: Array<{ slug: string; title: string; documentType: string }> }
    publishedLegalDocuments.value = result.items || []
  } catch { /* Footer policy links stay hidden until published documents are available. */ }
}
async function loadLegalDocument(slug: string) {
  const requestId = ++legalDocumentRequestSequence
  legalDocumentLoading.value = true
  legalDocumentError.value = ''
  legalDocument.value = null
  legalDocumentHtml.value = ''
  try {
    const response = await fetch(`/api/public/legal/${encodeURIComponent(slug)}`, { headers: { Accept: 'application/json' } })
    const result = await response.json() as { item?: { slug: string; title: string; bodyMarkdown: string; effectiveAt?: string | null }; error?: string }
    if (!response.ok || !result.item) throw new Error(result.error || '政策文件不存在或尚未发布。')
    if (requestId !== legalDocumentRequestSequence || route.value !== `/legal/${slug}`) return
    const { renderMarkdown } = await import('./utils/markdown')
    legalDocumentHtml.value = renderMarkdown(result.item.bodyMarkdown)
    legalDocument.value = result.item
  } catch (e) {
    if (requestId === legalDocumentRequestSequence) legalDocumentError.value = e instanceof Error ? e.message : '读取政策文件失败。'
  } finally {
    if (requestId === legalDocumentRequestSequence) legalDocumentLoading.value = false
  }
}

async function togglePublicInteraction(type: 'like' | 'favorite') {
  if (!publicArticle.value || publicInteractionBusy.value) return
  publicInteractionBusy.value = true
  try {
    const response = await fetch(`/api/public/content/${encodeURIComponent(publicArticle.value.slug)}/interactions`, {
      method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ type }),
    })
    const result = await response.json() as { active?: boolean; likeCount?: number; favoriteCount?: number; error?: string }
    if (response.status === 401) {
      authNotice.value = '请先登录，再进行点赞或收藏。'
      await navigate('/login')
      return
    }
    if (!response.ok) throw new Error(result.error || '操作失败，请稍后重试。')
    publicArticle.value = { ...publicArticle.value,
      likeCount: result.likeCount ?? publicArticle.value.likeCount,
      favoriteCount: result.favoriteCount ?? publicArticle.value.favoriteCount,
      ...(type === 'like' ? { viewerLiked: Boolean(result.active) } : { viewerFavorited: Boolean(result.active) }),
    }
    portalNotify.success(type === 'like' ? (result.active ? '已点赞' : '已取消点赞') : (result.active ? '已收藏' : '已取消收藏'))
  } catch (error) {
    portalNotify.error(error instanceof Error ? error.message : '操作失败。')
  } finally { publicInteractionBusy.value = false }
}
function sharePublicArticle() {
  if (!publicArticle.value) return
  shareUrl.value = window.location.href
  shareDialogOpen.value = true
}
async function copyShareUrl() {
  if (!shareUrl.value) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(shareUrl.value)
    } else {
      const input = document.querySelector<HTMLInputElement>('.share-dialog-url')
      if (!input) throw new Error('无法读取分享链接。')
      input.focus()
      input.select()
      if (!document.execCommand('copy')) throw new Error('浏览器未能复制链接，请手动选择并复制。')
    }
    portalNotify.success('分享链接已复制')
    shareDialogOpen.value = false
  } catch (error) {
    portalNotify.error(error instanceof Error ? error.message : '复制失败，请手动复制链接。')
  }
}
async function navigate(target: string) {
  saveScrollPosition()
  closeMenu()
  history.pushState({}, '', target)
  setRouteFromLocation()
  void loadPublishedLegalDocuments()
  authError.value = ''
  authNotice.value = ''
  if (isProtectedPage.value) {
    const valid = await loadSession()
    if (!valid) {
      history.replaceState({}, '', '/login')
      route.value = '/login'
      authNotice.value = '请先登录，再进入工作空间。'
      return
    }
  }
  if (route.value === '/' || (isPublicDirectoryPage.value && route.value !== '/education-resources')) void loadPublicContents()
  if (route.value === '/education-resources') void loadPublicTeachingResources()
  if (route.value === '/content-management') await loadContentList()
  if (route.value === '/education/teaching-resources') await loadTeachingResources()
  if (isContentEditorPage.value) void initializeContentEditorRoute()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (isLegalPage.value) void loadLegalDocument(legalSlug.value)
  if (route.value === '/' && window.location.hash) {
    await nextTick()
    document.querySelector(window.location.hash)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  } else {
    window.scrollTo({ top: 0, behavior: 'auto' })
    if (isAccountPage.value) {
      await nextTick()
      const content = document.querySelector('.account-content') as HTMLElement | null
      const table = document.querySelector('.virtual-table-scroll') as HTMLElement | null
      if (content) content.scrollTop = 0
      if (table) table.scrollTop = 0
    }
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
    const payload: RegisterRequest | LoginRequest = isRegister.value
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
    portalNotify.success(isRegister.value ? '账户创建成功。' : '登录成功。', isRegister.value ? '注册成功' : '登录成功')
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
    portalNotify.success('已安全退出登录。', '退出登录成功')
  }
}
function onPopState() {
  setRouteFromLocation()
  closeMenu()
  authError.value = ''
  if (route.value === '/' || (isPublicDirectoryPage.value && route.value !== '/education-resources')) void loadPublicContents()
  if (route.value === '/education-resources') void loadPublicTeachingResources()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (isLegalPage.value) void loadLegalDocument(legalSlug.value)
  if (route.value === '/content-management') void loadContentList()
  if (route.value === '/education/teaching-resources') void loadTeachingResources()
  if (isProtectedPage.value) void loadSession().then((valid) => { if (!valid) void navigate('/login') })
  if (isContentEditorPage.value) void initializeContentEditorRoute()
  void restoreScrollPosition()
}
onMounted(() => {
  setRouteFromLocation()
  void nextTick().then(observeVirtualTableWidth)
  window.addEventListener('popstate', onPopState)
  document.addEventListener('scroll', onScrollCapture, true)
  window.addEventListener('beforeunload', saveScrollPosition)
  if (route.value === '/' || (isPublicDirectoryPage.value && route.value !== '/education-resources')) void loadPublicContents()
  if (route.value === '/education-resources') void loadPublicTeachingResources()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (isLegalPage.value) void loadLegalDocument(legalSlug.value)
  void loadPublishedLegalDocuments()
  void loadSession().then((valid) => {
    if (isProtectedPage.value && !valid) {
      history.replaceState({}, '', '/login')
      route.value = '/login'
      authNotice.value = '请先登录，再进入工作空间。'
    }
  })
  if (isAccountPage.value && activeAccountSection.value === 'content-management') void loadContentList()
  if (isAccountPage.value && activeAccountSection.value === 'teaching-resources') void loadTeachingResources()
  if (isContentEditorPage.value) void initializeContentEditorRoute()
  void restoreScrollPosition()
})
onBeforeUnmount(() => {
  if (selectedEducationResource.value) document.body.style.overflow = educationResourcePreviousBodyOverflow
  educationResourcePreviousBodyOverflow = ''
  saveScrollPosition()
  if (tableSearchDebounceTimer) clearTimeout(tableSearchDebounceTimer)
  tableResizeObserver?.disconnect()
  window.removeEventListener('popstate', onPopState)
  document.removeEventListener('scroll', onScrollCapture, true)
  window.removeEventListener('beforeunload', saveScrollPosition)
})
</script>

<template>
  <div class="site-shell">
    <header v-if="!isAccountPage && !isContentEditorPage" class="site-header">
      <a class="brand" href="/" aria-label="资源与服务平台首页" @click.prevent="navigate('/')">
        <span class="brand-mark">R.</span>
        <span class="brand-copy"><strong>资源平台</strong><small>资源 · 信息 · 行业服务</small></span>
      </a>
      <button class="menu-toggle" type="button" :aria-expanded="menuOpen" aria-label="切换导航菜单" @click="menuOpen = !menuOpen">
       <span></span>
      </button>
      <nav class="main-nav" :class="{ 'is-open': menuOpen }" aria-label="主导航">
        <a href="/knowledge" @click.prevent="navigate('/knowledge')">知识广场</a>
        <a href="/education-resources" @click.prevent="navigate('/education-resources')">教育资源</a>
        <a href="/homestays" @click.prevent="navigate('/homestays')">民宿房源</a>
        <a href="/software-tools" @click.prevent="navigate('/software-tools')">软件工具</a>
        <a href="/#explore" @click.prevent="navigate('/#explore')">资源目录</a>
        <a href="/#about" @click.prevent="navigate('/#about')">关于</a>
        <a v-if="user" class="btn btn-outline-secondary nav-auth" href="/account" @click.prevent="navigate('/account')">个人中心</a>
        <a v-else class="btn btn-outline-secondary nav-auth" href="/login" @click.prevent="navigate('/login')">登录 / 注册</a>
      </nav>
    </header>

    <main v-if="isContentEditorPage" class="content-workspace-page">
      <header class="content-workspace-topbar">
        <a class="workspace-brand" href="/content-management" @click.prevent="returnToContentList"><span class="workspace-brand-mark">R.</span><span><strong>内容工作台</strong><small></small></span></a>
        <div class="workspace-document-name"><span class="workspace-status-dot"></span><span>{{ contentForm.title.trim() || '未命名内容' }}</span><em>草稿</em></div>
        <button type="button" class="btn content-secondary-button workspace-return" :disabled="contentBusy" @click="cancelContentEditor"><i class="bi bi-arrow-left" aria-hidden="true"></i> 返回列表</button>
      </header>
      <ContentDocumentEditor v-model="contentForm.body" workspace :disabled="contentBusy">
        <template #workspace-actions>
          <details class="workspace-fold workspace-settings-fold" open>
            <summary><span><strong>创建内容</strong></span><span class="workspace-fold-chevron bi bi-chevron-down" aria-hidden="true"></span></summary>
            <section class="workspace-properties">
              <label><span>内容标题</span><input class="form-control" v-model="contentForm.title" name="contentTitle" required maxlength="200" placeholder="输入内容标题" /></label>
              <fieldset class="workspace-tag-field"><legend>知识广场标签 <small>可多选</small></legend><button type="button" ref="knowledgeTagTrigger" class="workspace-tag-picker-trigger" :disabled="contentBusy" :aria-expanded="knowledgeTagDialogOpen" aria-haspopup="dialog" @click.stop="openKnowledgeTagDialog"><span class="bi bi-diagram-3" aria-hidden="true"></span><span>{{ selectedKnowledgeTagCount ? `已选择 ${selectedKnowledgeTagCount} 个标签` : '点击选择标签' }}</span><span class="bi bi-chevron-down" aria-hidden="true"></span></button><div v-if="selectedKnowledgeTagCount" class="workspace-tag-selected-preview"><span v-for="tag in contentForm.tags.filter(item => !taxonomyParentNames.has(item))" :key="tag" class="workspace-tag-selected-chip">{{ tag }}<button type="button" :disabled="contentBusy" :aria-label="`移除标签 ${tag}`" @click="removeSelectedKnowledgeTag(tag)">×</button></span><button type="button" class="workspace-tag-clear" :disabled="contentBusy" @click="clearSelectedKnowledgeTags">清空</button></div><p v-else class="workspace-tag-empty-hint">尚未选择标签</p></fieldset>
              <label><span>可见范围</span><VirtualizedSelect v-model="contentForm.visibility" label="可见范围" :options="visibilityOptions" /></label>
              <label><span>URL 别名</span><input class="form-control" v-model="contentForm.slug" name="contentSlug" required maxlength="100" pattern="[a-z0-9]+(-[a-z0-9]+)*" placeholder="例如：go-language-notes" /></label>
              <label><span>摘要</span><textarea class="form-control" v-model="contentForm.summary" name="contentSummary" maxlength="500" rows="3" placeholder="简要介绍这篇内容…"></textarea></label>

              <p class="workspace-privacy-note"><span>◈</span> 内容默认保存为草稿；保存后可在内容列表单独发布。</p>
            </section>
          </details>
          <div class="workspace-create-actions">
            <button type="button" class="btn content-secondary-button" :disabled="contentBusy" @click="cancelContentEditor">取消</button>
            <button type="button" class="btn btn-dark workspace-save-draft" :disabled="contentBusy" @click="saveContent()">{{ contentBusy ? '正在保存…' : '保存草稿' }}</button>
          </div>
        </template>
      </ContentDocumentEditor>
      <Teleport to="body">
        <div v-if="knowledgeTagDialogOpen" class="knowledge-tag-dialog-backdrop" @click.self="closeKnowledgeTagDialog(false)" @keydown.esc.stop.prevent="closeKnowledgeTagDialog(false)">
          <section ref="knowledgeTagDialog" class="knowledge-tag-dialog" role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="knowledge-tag-dialog-title" @click.stop>
            <header class="knowledge-tag-dialog-header"><div><span class="knowledge-tag-dialog-eyebrow">CONTENT TAXONOMY</span><h2 id="knowledge-tag-dialog-title">选择知识广场标签</h2><p>展开分类后选择具体标签；一级分类会随子标签自动关联。</p></div><button type="button" class="knowledge-tag-dialog-close" aria-label="关闭标签选择器" @click="closeKnowledgeTagDialog(false)">×</button></header>
            <div class="knowledge-tag-dialog-body">
              <section v-for="category in knowledgeTagOptions" :key="category.value" class="knowledge-tag-tree-group" :class="{ 'is-expanded': expandedKnowledgeTagGroups.includes(category.value) }">
                <button type="button" class="knowledge-tag-tree-heading" :aria-expanded="expandedKnowledgeTagGroups.includes(category.value)" @click.stop.prevent="toggleKnowledgeTagGroup(category.value)"><span class="bi" :class="expandedKnowledgeTagGroups.includes(category.value) ? 'bi-chevron-down' : 'bi-chevron-right'" aria-hidden="true"></span><span class="knowledge-tag-tree-title">{{ category.value }}</span><span class="knowledge-tag-parent-note">一级分类 · 自动关联</span><span class="knowledge-tag-tree-count">{{ category.children.filter(child => isKnowledgeTagSelected(child)).length }}/{{ category.children.length }}</span></button>
                <div v-if="expandedKnowledgeTagGroups.includes(category.value)" class="knowledge-tag-tree-children"><label v-for="child in category.children" :key="child" class="knowledge-tag-tree-option" :class="{ 'is-selected': isKnowledgeTagSelected(child) }"><input type="checkbox" :checked="isKnowledgeTagSelected(child)" :disabled="contentBusy" @change="toggleKnowledgeTag(child, ($event.target as HTMLInputElement).checked)"/><span class="knowledge-tag-tree-checkbox"><span class="bi bi-check" aria-hidden="true"></span></span><span>{{ child }}</span></label></div>
              </section>
            </div>
            <footer class="knowledge-tag-dialog-footer"><span>已选择 <strong>{{ knowledgeTagDraftCount }}</strong> 个具体标签</span><div><button type="button" class="knowledge-tag-dialog-clear" :disabled="contentBusy || !knowledgeTagDraftCount" @click="clearKnowledgeTags">清空选择</button><button type="button" class="knowledge-tag-dialog-confirm" @click="closeKnowledgeTagDialog(true)">完成</button></div></footer>
          </section>
        </div>
      </Teleport>
    </main>

    <main v-else-if="route === '/'" id="top">
      <section class="hero">
        <div class="hero-copy">
          <p class="eyebrow"><span class="status-dot"></span> A PERSONAL KNOWLEDGE SPACE</p>
          <h1>把想法整理好，<br /><span>让行动有迹可循。</span></h1>
          <p class="hero-description">一个持续生长的个人门户，用来整理知识、构建工具、归档资源，并记录从想法到实践的过程。</p>
          <div class="hero-actions"><a class="btn btn-dark button button-primary" href="/#explore" @click.prevent="navigate('/#explore')">开始探索</a><a class="text-link" href="/#about" @click.prevent="navigate('/#about')">了解这个空间</a></div>
          <div class="hero-note"><span class="note-line"></span><span>持续整理 · 持续构建 · 持续迭代</span></div>
        </div>
        <div class="hero-art" aria-label="由层叠卡片构成的抽象知识结构插图" role="img">
          <div class="art-orbit orbit-one"></div><div class="art-orbit orbit-two"></div>
          <div class="art-card card-back"><span class="art-card-label">FIELD NOTES</span><span class="art-lines"><i></i><i></i><i></i></span></div>
          <div class="art-card card-front"><span class="art-card-label">THOUGHT  ACTION</span><div class="art-symbol">P<span>.</span></div><div class="art-card-footer"><span>COLLECT</span><span>CONNECT</span><span>CREATE</span></div></div>
          <span class="art-index">FIG. 001 — AN OPEN SYSTEM</span><span class="art-spark spark-one bi bi-stars" aria-hidden="true"></span><span class="art-spark spark-two bi bi-plus-lg" aria-hidden="true"></span>
        </div>
        <div class="hero-bottom"><span>独立构建的个人空间</span><span>01 / 04 <i></i> PUBLIC SECTIONS</span></div>
      </section>
      <section id="explore" class="explore-section">
        <div class="section-heading"><div><p class="eyebrow">EXPLORE THE PORTAL <span class="heading-rule"></span></p><h2>探索你需要的资源。</h2></div><p class="section-intro">从知识信息到实用工具，逐步拓展更多行业与服务。</p></div>
        <label class="search-box"><span class="search-icon bi bi-search" aria-hidden="true"></span><input class="form-control" v-model="query" name="portalSearch" type="search" placeholder="搜索资源、信息、工具或行业服务…" aria-label="搜索门户分类" /><kbd>⌘ K</kbd></label>
        <div v-if="filteredSections.length" class="section-grid row g-3">
          <article v-for="section in filteredSections" :key="section.id" class="section-card col-12 md:col-6 xl:col-4">
            <div class="card-topline"><span>{{ section.number }}</span><span class="card-arrow bi bi-arrow-up-right" aria-hidden="true"></span></div><p class="card-kicker">{{ section.subtitle }}</p><h3>{{ section.title }}</h3><p class="card-description">{{ section.description }}</p>
            <div class="tag-list"><span v-for="tag in section.tags" :key="tag">{{ tag }}</span></div><div class="card-bottom"><span>内容逐步整理中</span><i class="bi bi-arrow-up-right" aria-hidden="true"></i></div>
          </article>
        </div>
        <div v-else class="empty-search"><strong>没有匹配的分类</strong><p>试试“知识”“工具”“资源”或“项目”。</p><button type="button" @click="query = ''">清除搜索</button></div>
      </section>
      <section id="about" class="about-section"><div class="about-mark">R.</div><div class="about-copy"><p class="eyebrow">ONE PLATFORM, MULTIPLE INDUSTRIES</p><h2>从信息发现，<br />到业务真正落地。</h2><p>这里汇集跨领域的资源信息、实用工具与专业服务，并逐步构建面向商家和不同行业的专属管理工作空间。用户获取信息，商家经营业务，各类服务在统一的平台体系中连接起来。</p></div><div class="about-meta"><span>DESIGNED FOR MANY</span><span>Resources meet business.</span></div></section>
    </main>

    <main v-else-if="isPublicDirectoryPage" class="directory-page" :class="{ 'knowledge-directory-page': route === '/knowledge', 'education-resource-directory-page': route === '/education-resources' }">
      <section v-if="route !== '/knowledge' && route !== '/education-resources'" class="directory-hero">
        <a class="directory-back-link" href="/" @click.prevent="navigate('/')"><i class="bi bi-arrow-left" aria-hidden="true"></i> 返回首页</a>
        <p class="eyebrow">{{ currentDirectoryPage.eyebrow }}</p>
        <h1>{{ currentDirectoryPage.title }}<span>.</span></h1>
        <h2>{{ currentDirectoryPage.subtitle }}</h2>
        <p class="directory-description">{{ currentDirectoryPage.description }}</p>
      </section>
      <section v-if="route !== '/knowledge' && route !== '/education-resources'" class="directory-section">
        <div class="directory-section-heading"><div><p class="eyebrow">BROWSE BY CATEGORY</p><h2>按主题探索</h2></div><span>{{ String(currentDirectoryTiles.length).padStart(2, '0') }} 个分类</span></div>
        <div class="directory-tile-grid row g-3">
          <article v-for="tile in currentDirectoryTiles" :key="tile.number" class="directory-tile col-12 md:col-6 xl:col-3">
            <div class="directory-tile-top"><span>{{ tile.number }}</span><i class="bi bi-arrow-up-right" aria-hidden="true"></i></div>
            <h3>{{ tile.title }}</h3>
            <p>{{ tile.description }}</p>
            <div class="tag-list"><span v-for="tag in tile.tags" :key="tag">{{ tag }}</span></div>
          </article>
        </div>
      </section>
      <section v-if="route === '/knowledge'" class="directory-published-section knowledge-filter-section">
        <div class="knowledge-toolbar" aria-label="文章筛选与搜索">
          <div class="knowledge-toolbar-controls">
            <nav class="knowledge-category-nav" aria-label="知识分类">
              <button v-for="category in knowledgeCategoryMenus" :key="category.value" type="button" class="knowledge-category-button" :class="{ 'is-active': knowledgeCategory === category.value }" :aria-pressed="knowledgeCategory === category.value" @click="chooseKnowledgeCategory(category.value)">{{ category.label }}</button>
            </nav>
            <div v-if="knowledgeCategory !== 'recent' && knowledgeCategoryMenus.find(category => category.value === knowledgeCategory)?.children.length" class="knowledge-subcategory-nav" aria-label="子分类">
              <button type="button" class="knowledge-subcategory-button" :class="{ 'is-active': !knowledgeSubcategory }" :aria-pressed="!knowledgeSubcategory" @click="chooseKnowledgeCategory(knowledgeCategory)">全部</button>
              <button v-for="child in knowledgeCategoryMenus.find(category => category.value === knowledgeCategory)?.children || []" :key="child" type="button" class="knowledge-subcategory-button" :class="{ 'is-active': knowledgeSubcategory === child }" :aria-pressed="knowledgeSubcategory === child" @click="chooseKnowledgeCategory(knowledgeCategory, child)">{{ child }}</button>
            </div>
            <div class="knowledge-search-wrap">
              <label class="knowledge-search-form knowledge-autocomplete">
                <span class="knowledge-search-icon bi bi-search" aria-hidden="true"></span>
                <input class="form-control" v-model="knowledgeSearchQuery" type="search" placeholder="搜索文章…" aria-label="模糊搜索知识文章" autocomplete="off" @input="knowledgeVisibleCount = knowledgePageSize; highlightedKnowledgeSuggestion = -1" @keydown="onKnowledgeSearchKeydown" />
                <button v-if="knowledgeSearchQuery" type="button" class="knowledge-search-clear" aria-label="清除搜索" @click="knowledgeSearchQuery = ''; knowledgeVisibleCount = knowledgePageSize; highlightedKnowledgeSuggestion = -1"><i class="bi bi-x-lg" aria-hidden="true"></i></button>
              </label>
              <div v-if="knowledgeSearchQuery.trim() && knowledgeSearchSuggestions.length" class="knowledge-autocomplete-popper" role="listbox" aria-label="搜索建议">
                <button v-for="(item, index) in knowledgeSearchSuggestions" :key="item.kind + ':' + item.label" type="button" role="option" :aria-selected="highlightedKnowledgeSuggestion === index" class="knowledge-autocomplete-option" :class="{ highlighted: highlightedKnowledgeSuggestion === index }" @mouseenter="highlightedKnowledgeSuggestion = index" @click="handleKnowledgeAutocompleteSelect({ value: item.label, kind: item.kind })">
                  <span class="knowledge-suggestion-kind">{{ item.kind === 'theme' ? '主题' : '文章' }}</span>
                  <span class="knowledge-suggestion-copy"><strong>{{ item.label }}</strong><small v-if="'summary' in item && item.summary">{{ item.summary }}</small></span>
                  <span class="knowledge-suggestion-arrow" aria-hidden="true"><i class="bi bi-arrow-up-right"></i></span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>
      <section v-if="route === '/knowledge'" class="directory-published-section knowledge-results-section">
        <div class="directory-section-heading knowledge-results-heading"><div><p class="eyebrow">KNOWLEDGE LIBRARY</p><h2>文章列表</h2></div><span>{{ knowledgeFilteredItems.length }} 篇文章</span></div>
        <p v-if="publicContentLoading && !publicContents.length" class="directory-empty">正在读取已发布内容…</p>
        <p v-else-if="publicContentError" class="directory-empty">{{ publicContentError }}</p>
        <div v-else-if="knowledgeFilteredItems.length" class="knowledge-infinite-list">
          <a v-for="(item, index) in visibleDirectoryContentItems" :key="item.id" class="knowledge-list-item" :href="`/content/${item.slug}`" @click.prevent="navigate(`/content/${item.slug}`)">
            <span class="knowledge-list-index">{{ String(index + 1).padStart(2, '0') }}</span>
            <span class="knowledge-list-copy"><strong>{{ item.title }}</strong><span v-if="item.summary">{{ item.summary }}</span></span>
            <time>{{ item.publishedAt ? new Date(item.publishedAt).toLocaleDateString('zh-CN') : '已发布' }}</time>
            <span class="knowledge-list-arrow" aria-hidden="true"><i class="bi bi-arrow-up-right"></i></span>
          </a>
          <p class="knowledge-list-status">{{ visibleDirectoryContentItems.length < knowledgeFilteredItems.length ? '向下滚动加载更多' : `已显示全部 ${knowledgeFilteredItems.length} 篇文章` }}</p>
        </div>
        <p v-else-if="knowledgeSearchQuery.trim()" class="directory-empty">没有找到匹配“{{ knowledgeSearchQuery.trim() }}”的文章。试试其他关键词或选择下方主题。</p>
        <p v-else-if="knowledgeCategory !== 'recent'" class="directory-empty">当前分类暂时没有匹配的文章。现有文章尚未单独标注分类，筛选暂按标题和摘要中的主题词匹配。</p>
        <p v-else class="directory-empty">这里暂时还没有已发布的内容。内容发布后会自动显示在这里。</p>
      </section>
      <section v-if="route === '/education-resources'" class="directory-published-section knowledge-filter-section education-resource-filter-section">
        <div class="education-resource-heading"><p class="eyebrow">LEARNING LIBRARY</p><h1>教育资源<span>.</span></h1><p>按资源类型与适用年级筛选，快速找到需要的学习资料。</p></div>
        <div class="knowledge-toolbar" aria-label="教育资源筛选与搜索"><div class="knowledge-toolbar-controls">
          <nav class="knowledge-category-nav" aria-label="资源类型">
            <button v-for="category in educationResourceTypes" :key="category" type="button" class="knowledge-category-button" :class="{ 'is-active': educationResourceCategory === category }" :aria-pressed="educationResourceCategory === category" @click="educationResourceCategory = category; knowledgeVisibleCount = knowledgePageSize">{{ category }}</button>
          </nav>
          <nav class="knowledge-subcategory-nav" aria-label="适用年级">
            <button v-for="grade in educationResourceGrades" :key="grade" type="button" class="knowledge-subcategory-button" :class="{ 'is-active': educationResourceGrade === grade }" :aria-pressed="educationResourceGrade === grade" @click="educationResourceGrade = grade; knowledgeVisibleCount = knowledgePageSize">{{ grade }}</button>
          </nav>
          <div class="knowledge-search-wrap"><label class="knowledge-search-form knowledge-autocomplete"><span class="knowledge-search-icon bi bi-search" aria-hidden="true"></span><input class="form-control" v-model="educationResourceSearch" type="search" placeholder="搜索资源名称、说明或文件名…" aria-label="模糊搜索教育资源" autocomplete="off" @input="knowledgeVisibleCount = knowledgePageSize" /><button v-if="educationResourceSearch" type="button" class="knowledge-search-clear" aria-label="清除搜索" @click="educationResourceSearch = ''; knowledgeVisibleCount = knowledgePageSize"><i class="bi bi-x-lg" aria-hidden="true"></i></button></label></div>
        </div></div>
      </section>
      <section v-if="route === '/education-resources'" class="directory-published-section knowledge-results-section education-resource-results-section">
        <div class="directory-section-heading knowledge-results-heading"><div><p class="eyebrow">PUBLIC LEARNING MATERIALS</p><h2>资源列表</h2></div><span>{{ filteredEducationResources.length }} 份资源</span></div>
        <p v-if="educationResourceLoading && !publicTeachingResources.length" class="directory-empty">正在读取公开教育资源…</p>
        <p v-else-if="educationResourceError" class="directory-empty">{{ educationResourceError }}</p>
        <div v-else-if="filteredEducationResources.length" class="knowledge-infinite-list">
          <a v-for="item in visibleEducationResources" :key="item.id" class="knowledge-list-item education-resource-list-item" href="#resource-detail" :aria-label="`${item.title}，查看资源详情`" @click.prevent="selectedEducationResource = item">
            <span class="education-resource-file-icon" aria-hidden="true"><i :class="['bi', resourceFileIcon(item.filename, item.url)]"></i></span>
            <span class="knowledge-list-copy"><strong>{{ item.title }}</strong><span v-if="item.description">{{ item.description }}</span><span class="education-resource-meta"><b>{{ item.type }}</b><b>{{ item.grade || '不限定年级' }}</b><span class="education-resource-filename">{{ item.filename || (item.url ? '在线资源' : '学习资料') }}</span></span></span>
            <time>{{ new Date(item.updatedAt || item.createdAt).toLocaleDateString('zh-CN') }}</time><span class="knowledge-list-arrow" aria-hidden="true"><i class="bi bi-arrow-up-right"></i></span>
          </a>
          <p class="knowledge-list-status">{{ visibleEducationResources.length < filteredEducationResources.length ? '向下滚动加载更多' : `已显示全部 ${filteredEducationResources.length} 份资源` }}</p>
        </div>
        <p v-else-if="educationResourceSearch.trim() || educationResourceCategory !== '全部资源' || educationResourceGrade !== '全部年级'" class="directory-empty">没有找到符合条件的公开资源。请调整筛选条件或搜索关键词。</p>
        <p v-else class="directory-empty">目前还没有公开的教育资源。登录后在“教学资源”中创建资源，并勾选“公开展示”，资源就会显示在这里。</p>
        <div v-if="selectedEducationResource" class="share-dialog-backdrop education-resource-detail-backdrop" @click.self="selectedEducationResource = null" @keydown.esc.stop.prevent="selectedEducationResource = null">
          <section class="share-dialog education-resource-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="education-resource-detail-title" @click.stop>
            <button type="button" class="education-resource-detail-close" aria-label="关闭资源详情" @click="selectedEducationResource = null"><i class="bi bi-x-lg" aria-hidden="true"></i></button>
            <span class="share-dialog-eyebrow">LEARNING RESOURCE</span>
            <h2 id="education-resource-detail-title">{{ selectedEducationResource.title }}</h2>
            <p v-if="selectedEducationResource.description" class="education-resource-detail-description">{{ selectedEducationResource.description }}</p>
            <p v-else class="education-resource-detail-description education-resource-detail-empty-description">此资源暂无补充说明。</p>
            <dl class="education-resource-detail-meta">
              <div><dt>资源类型</dt><dd>{{ selectedEducationResource.type || '未分类' }}</dd></div>
              <div><dt>适用年级</dt><dd>{{ selectedEducationResource.grade || '不限定年级' }}</dd></div>
              <div v-if="selectedEducationResource.filename"><dt>文件名称</dt><dd>{{ selectedEducationResource.filename }}</dd></div>
              <div v-if="selectedEducationResource.fileSize != null"><dt>文件大小</dt><dd>{{ formatResourceFileSize(selectedEducationResource.fileSize) }}</dd></div>
              <div v-if="selectedEducationResource.url"><dt>在线链接</dt><dd><a class="education-resource-detail-url" :href="selectedEducationResource.url" target="_blank" rel="noopener noreferrer">{{ selectedEducationResource.url }}</a></dd></div>
              <div><dt>更新时间</dt><dd>{{ new Date(selectedEducationResource.updatedAt || selectedEducationResource.createdAt).toLocaleDateString('zh-CN') }}</dd></div>
            </dl>
            <div v-if="selectedEducationResource.filename?.toLowerCase().endsWith('.mp3') && selectedEducationResource.downloadUrl" class="education-resource-audio-preview">
              <label :for="'education-resource-audio-' + selectedEducationResource.id">MP3 音频预览</label>
              <audio :id="'education-resource-audio-' + selectedEducationResource.id" controls preload="metadata" :src="selectedEducationResource.downloadUrl.replace(/\/download$/, '/preview')">当前浏览器不支持音频播放，请使用下载功能。</audio>
            </div>
            <p v-if="!selectedEducationResource.url && !selectedEducationResource.downloadUrl" class="education-resource-detail-warning">该资源当前没有可访问的文件或链接，请联系资源发布者。</p>
            <div class="education-resource-detail-actions">
              <a v-if="selectedEducationResource.url" class="education-resource-detail-action education-resource-detail-open" :href="selectedEducationResource.url" target="_blank" rel="noopener noreferrer"><i class="bi bi-box-arrow-up-right" aria-hidden="true"></i> 打开链接</a>
              <a v-if="selectedEducationResource.downloadUrl" class="education-resource-detail-action education-resource-detail-download" :href="selectedEducationResource.downloadUrl"><i class="bi bi-download" aria-hidden="true"></i> 下载文件</a>
            </div>
          </section>
        </div>
      </section>
      <section v-if="route === '/software-tools'" class="directory-published-section">
        <div class="directory-section-heading"><div><p class="eyebrow">RECENTLY ADDED</p><h2>{{ directoryContentHeading }}</h2></div><span>{{ directoryContentItems.length }} 条</span></div>
        <p v-if="publicContentLoading && !publicContents.length" class="directory-empty">正在读取已发布内容…</p>
        <p v-else-if="publicContentError" class="directory-empty">{{ publicContentError }}</p>
        <div v-else-if="directoryContentItems.length" class="published-content-grid row g-3">
          <a v-for="(item, index) in directoryContentItems" :key="item.id" class="published-content-card col-12 md:col-6 xl:col-4" :href="`/content/${item.slug}`" @click.prevent="navigate(`/content/${item.slug}`)">
            <div class="published-content-meta"><span class="published-content-type">{{ (item.tags?.length ? item.tags.join(' · ') : contentTypeLabels[item.contentType]) }}</span><span>{{ item.publishedAt ? new Date(item.publishedAt).toLocaleDateString('zh-CN') : '已发布' }}</span></div>
            <div class="published-content-main"><span class="published-content-index">{{ String(index + 1).padStart(2, '0') }}</span><div class="published-content-copy"><h3>{{ item.title }}</h3><p>{{ item.summary || '打开查看完整内容。' }}</p><span class="published-content-link">查看详情 <i class="bi bi-arrow-up-right" aria-hidden="true"></i></span></div></div>
          </a>
        </div>
        <p v-else class="directory-empty">这里暂时还没有已发布的内容。内容发布后会自动显示在这里。</p>
      </section>
      <section v-if="route === '/homestays'" class="directory-published-section directory-coming-soon"><p class="eyebrow">LISTINGS COMING SOON</p><h2>房源内容正在整理中</h2><p>后续将在这里展示民宿图片、所在位置、价格、设施说明、入住规则及联系预订方式。</p></section>
    </main>

    <main v-else-if="isAuthPage" class="route-main">
      <section class="auth-panel glass-panel">
        <p class="eyebrow"><span class="status-dot"></span> PLATFORM ACCOUNT</p>
        <h1>{{ isRegister ? '创建你的账户' : '欢迎回来' }}</h1>
        <p class="auth-intro">{{ isRegister ? '注册后即可进入个人中心。你的账户信息将安全保存。' : '登录后进入个人中心，管理你的账户信息。' }}</p>
        <form class="auth-form" @submit.prevent="submitAuth">
          <label v-if="isRegister" class="field-label">昵称 <input class="form-control" v-model="authForm.displayName" name="displayName" type="text" autocomplete="nickname" maxlength="60" placeholder="怎么称呼你" /></label>
          <label class="field-label">邮箱地址 <input class="form-control" v-model="authForm.email" name="email" type="email" autocomplete="email" maxlength="254" required placeholder="name@example.com" /></label>
          <label class="field-label">密码 <input class="form-control" v-model="authForm.password" name="password" type="password" :autocomplete="isRegister ? 'new-password' : 'current-password'" :minlength="isRegister ? 10 : 1" maxlength="72" required :placeholder="isRegister ? '至少 10 个字符' : '输入你的密码'" /></label>
          <p v-if="isRegister" class="field-help">密码长度为 10–72 个字符。请勿重复使用其他网站的密码。</p>
          <button class="btn button button-primary auth-submit" type="submit" :disabled="authBusy">{{ authBusy ? '正在处理…' : (isRegister ? '创建账户' : '登录') }}</button>
        </form>
        <p class="auth-switch">{{ isRegister ? '已经有账户？' : '还没有账户？' }} <a :href="isRegister ? '/login' : '/register'" @click.prevent="navigate(isRegister ? '/login' : '/register')">{{ isRegister ? '立即登录' : '创建账户' }}</a></p>
        <a class="back-home" href="/" @click.prevent="navigate('/')"><i class="bi bi-arrow-left" aria-hidden="true"></i> 返回首页</a>
      </section>
    </main>

    <main v-else-if="isPublicContentPage" class="public-content-page">
      <section v-if="publicContentLoading" class="public-content-state">正在读取内容…</section>
      <section v-else-if="publicArticle" class="public-article-layout">
        <article class="public-article">
          <header class="public-article-route-header" aria-label="文章信息">
            <span class="public-article-route-type">{{ publicArticle.tags?.length ? publicArticle.tags.join(' · ') : contentTypeLabels[publicArticle.contentType] }}</span>
            <span v-if="publicArticle.publishedAt" class="public-article-route-date">{{ new Date(publicArticle.publishedAt).toLocaleDateString('zh-CN') }}</span>
          </header>
          <h1>{{ publicArticle.title }}</h1>
          <p v-if="publicArticle.summary" class="public-article-summary">{{ publicArticle.summary }}</p>
          <div class="public-article-body markdown-rendered" v-html="publicArticleHtml"></div>
        </article>
        <aside class="public-article-sidebar" aria-label="作者信息与文章操作">
          <section class="public-author-card">
            <div class="public-author-avatar" aria-hidden="true">{{ (publicArticle.authorName || '作者').slice(0, 1) }}</div>
            <div class="public-author-heading"><span>本文作者</span><strong>{{ publicArticle.authorName || '平台作者' }}</strong></div>
            <p>持续整理经验、知识与实践记录。</p>
            <div class="public-author-meta">发布于 {{ publicArticle.publishedAt ? new Date(publicArticle.publishedAt).toLocaleDateString('zh-CN') : '近期' }}</div>
          </section>
          <section class="public-article-actions" aria-label="文章互动">
            <button type="button" :class="['public-article-action', { active: publicArticle.viewerLiked }]" :disabled="publicInteractionBusy" :aria-pressed="publicArticle.viewerLiked" @click="togglePublicInteraction('like')"><span class="public-action-symbol bi bi-heart-fill" aria-hidden="true"></span><span>{{ publicArticle.viewerLiked ? '已点赞' : '点赞' }}</span><strong>{{ publicArticle.likeCount || 0 }}</strong></button>
            <button type="button" :class="['public-article-action', { active: publicArticle.viewerFavorited }]" :disabled="publicInteractionBusy" :aria-pressed="publicArticle.viewerFavorited" @click="togglePublicInteraction('favorite')"><span class="public-action-symbol bi bi-star-fill" aria-hidden="true"></span><span>{{ publicArticle.viewerFavorited ? '已收藏' : '收藏' }}</span><strong>{{ publicArticle.favoriteCount || 0 }}</strong></button>
            <button type="button" class="public-article-action" @click="sharePublicArticle"><span class="public-action-symbol bi bi-share" aria-hidden="true"></span><span>转发 / 分享</span></button>
          </section>
          <p class="public-article-login-hint" v-if="!user">登录后可点赞和收藏文章。</p>
        </aside>
      </section>
      <section v-else class="public-content-state"><h1>内容暂不可用</h1><p>{{ publicContentError || '内容不存在或尚未公开。' }}</p><a class="btn btn-dark button button-primary" href="/" @click.prevent="navigate('/')">返回首页</a></section>
      <div v-if="shareDialogOpen" class="share-dialog-backdrop" @click.self="shareDialogOpen = false" @keydown.esc="shareDialogOpen = false">
        <section class="share-dialog" role="dialog" aria-modal="true" aria-labelledby="share-dialog-title">
          <button type="button" class="share-dialog-close" aria-label="关闭分享窗口" @click="shareDialogOpen = false"><i class="bi bi-x-lg" aria-hidden="true"></i></button>
          <span class="share-dialog-eyebrow">SHARE ARTICLE</span>
          <h2 id="share-dialog-title">分享这篇文章</h2>
          <p>复制下方链接，发送给需要阅读的人。</p>
          <label class="share-dialog-label" for="share-dialog-url">文章链接</label>
          <input id="share-dialog-url" class="form-control share-dialog-url" type="text" :value="shareUrl" readonly @focus="($event.target as HTMLInputElement).select()" />
          <div class="share-dialog-actions">
            <button type="button" class="share-dialog-cancel" @click="shareDialogOpen = false">取消</button>
            <button type="button" class="share-dialog-copy" @click="copyShareUrl"><span aria-hidden="true">▢</span> 复制链接</button>
          </div>
        </section>
      </div>
    </main>

    <main v-else-if="isLegalPage" class="legal-document-page">
      <section class="legal-document-shell">
        <a class="legal-back-link" href="/" @click.prevent="navigate('/')">← 返回首页</a>
        <div v-if="legalDocumentLoading" class="legal-document-state">正在读取政策文件…</div>
        <section v-else-if="legalDocument" class="legal-document-content">
          <p class="eyebrow">POLICIES & NOTICES</p>
          <h1>{{ legalDocument.title }}</h1>
          <p v-if="legalDocument.effectiveAt" class="legal-document-effective">生效日期：{{ new Date(legalDocument.effectiveAt).toLocaleDateString('zh-CN') }}</p>
          <div class="markdown-rendered legal-markdown" v-html="legalDocumentHtml"></div>
        </section>
        <section v-else class="legal-document-state"><h1>政策文件暂不可用</h1><p>{{ legalDocumentError || '该政策尚未发布。' }}</p></section>
      </section>
    </main>

    <main v-else-if="isAccountPage" class="account-layout">

      <aside class="account-sidebar">
        <div v-for="group in filteredAccountGroups" :key="group.id" class="account-nav-group">
          <button
            type="button"
            class="account-sidebar-item account-group-toggle"
            :class="{ 'is-active': activeSidebarButton === group.id }"
            :aria-expanded="expandedAccountGroups.includes(group.id)"
            @click="activeSidebarButton = group.id; toggleAccountGroup(group.id)"
          >
            <span :class="['sidebar-group-icon', 'bi', accountGroupIcon(group.id)]" aria-hidden="true"></span>{{ group.label }}
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
            <button class="btn button button-primary auth-submit" type="button" :disabled="authBusy" @click="logout">{{ authBusy ? '正在退出…' : '退出登录' }}</button>
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
            <div class="security-info-row"><span class="security-info-icon bi bi-shield-check" aria-hidden="true"></span><div><strong>账户已验证登录</strong><p>当前浏览器持有安全会话。</p></div></div>
            <div class="account-field"><span>登录邮箱</span><strong>{{ user.email }}</strong></div>
            <div class="account-field"><span>会话有效期</span><strong>系统自动管理</strong></div>
            <button class="btn button button-primary auth-submit" type="button" :disabled="authBusy" @click="logout">{{ authBusy ? '正在退出…' : '退出当前登录' }}</button>
          </div>
        </template>

        <template v-else-if="activeAccountSection === 'copyright-compliance'">
          <ComplianceCenter :is-admin="user?.isAdmin === true" />
        </template>
        <template v-else-if="activeAccountSection === 'system-warnings'">
          <SystemOpsCenter view="warnings" />
        </template>
        <template v-else-if="activeAccountSection === 'system-logs'">
          <SystemOpsCenter view="logs" />
        </template>

        <template v-else>
          <h1 v-if="activeAccountSection !== 'content-management' && !activeTableDefinition" class="account-page-title">{{ activeAccountItem?.label || '业务中心' }}</h1>
          <p v-if="activeAccountSection !== 'content-management'" class="auth-intro">{{ activeAccountItem?.description || '管理你的业务内容与工作入口。' }}</p>
          <section v-if="activeAccountSection === 'content-management' && versionsContentId" class="content-editor-panel content-versions-panel">
            <div class="content-editor-heading">
              <div><h2>版本历史</h2><p>“{{ versionsContentTitle }}” · 每次有效编辑前自动保存快照，最多显示最近 100 个版本；恢复只替换内容字段，保留当前发布状态。</p></div>
              <button type="button" class="btn content-secondary-button" :disabled="contentBusy" @click="versionsContentId = null">返回内容列表</button>
            </div>
            <div v-if="contentVersions.length" class="content-version-list">
              <div v-for="version in contentVersions" :key="version.versionId" class="content-version-row">
                <div><strong>版本 {{ version.versionNumber }} · {{ version.title }}</strong><p>{{ new Date(version.createdAt).toLocaleString('zh-CN') }} · {{ version.slug }} · {{ contentStatusLabels[version.status] || version.status }}</p></div>
                <button type="button" class="btn content-secondary-button" :disabled="contentBusy" @click="restoreContentVersion(version)">恢复此版本</button>
              </div>
            </div>
            <div v-else class="virtual-table-empty"><strong>暂无历史版本</strong><p>保存第一次有效修改后，这里会出现可恢复的历史版本。</p></div>
          </section>
          <section v-else-if="activeTableDefinition" class="virtual-table-panel" style="margin-right: unset; margin-left: 0px;">
            <div class="virtual-table-toolbar">
              <div class="virtual-table-heading"><h2>{{ activeTableDefinition.title }}</h2><span class="virtual-table-count">{{ contentLoading && activeAccountSection === 'content-management' ? '正在加载…' : filteredTableRecords.length + ' 条记录' }}</span></div>
              <div class="content-toolbar-actions">
                <input v-model="tableQuery" name="tableSearch" class="form-control virtual-table-search" type="search" placeholder="搜索表格内容…" aria-label="搜索表格内容" @input="tableScrollTop = 0" />
                <template v-if="activeAccountSection === 'content-management'">
                  <button type="button" class="btn content-secondary-button" @click="switchContentView(contentViewMode === 'active' ? 'trash' : 'active')">{{ contentViewMode === 'active' ? '回收站' : '返回内容列表' }}</button>
                  <button v-if="contentViewMode === 'active'" type="button" class="btn content-primary-button" @click="openNewContent">新建内容</button>
                </template>
                <template v-else-if="activeAccountSection === 'teaching-resources'">
                  <button type="button" class="btn content-secondary-button" :disabled="resourceBusy" @click="switchResourceView">{{ resourceViewMode === 'active' ? '回收站' : '返回资源列表' }}</button>
                  <button v-if="resourceViewMode === 'active'" type="button" class="btn content-primary-button" @click="openResourceDialog">创建资源</button>
                </template>
              </div>
            </div>
            <div ref="virtualTableScrollElement" class="virtual-table-scroll" :style="{ '--table-columns': tableColumnTemplate }" @scroll="onVirtualTableScroll">
              <div class="virtual-table-header" role="row">
                <span v-for="column in activeTableDefinition.columns" :key="column.key" role="columnheader">{{ column.label }}</span>
              </div>
              <div v-if="filteredTableRecords.length" class="virtual-table-body" role="rowgroup">
                <div class="virtual-table-spacer" :style="{ height: virtualTableTopSpace + 'px' }"></div>
                <div v-for="(record, index) in visibleTableRecords" :key="String(record.id ?? (virtualTableStart + index))" class="virtual-table-row" role="row">
                  <template v-for="column in activeTableDefinition.columns" :key="column.key">
                    <div v-if="column.key === 'actions' && (activeAccountSection === 'content-management' || activeAccountSection === 'teaching-resources')" class="content-row-actions" role="cell">
                      <template v-if="activeAccountSection === 'teaching-resources'">
                        <template v-if="resourceViewMode === 'trash'">
                          <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="resourceBusy" @click="changeTeachingResource(String(record.id), 'restore')">恢复</button>
                          <button type="button" class="btn btn-sm btn-outline-danger content-danger-action" :disabled="resourceBusy" @click="changeTeachingResource(String(record.id), 'permanent')">永久删除</button>
                        </template>
                        <template v-else>
                          <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="resourceBusy" @click="openResourceEditById(String(record.id))">编辑</button>
                          <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="resourceBusy" @click="downloadTeachingResource(String(record.id))">{{ teachingResources.find((item) => item.id === String(record.id))?.url ? '打开链接' : '下载' }}</button>
                          <button type="button" class="btn btn-sm btn-outline-danger content-danger-action" :disabled="resourceBusy" @click="changeTeachingResource(String(record.id), 'trash')">删除</button>
                        </template>
                      </template>
                      <template v-else-if="contentViewMode === 'trash'">
                        <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="contentBusy" @click="restoreDeletedContent(String(record.id))">恢复</button>
                        <button type="button" class="btn btn-sm btn-outline-danger content-danger-action" :disabled="contentBusy" @click="permanentlyDeleteContent(String(record.id))">永久删除</button>
                      </template>
                      <template v-else>
                        <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="contentBusy" @click="editContent(String(record.id))">编辑</button>
                        <button type="button" class="btn btn-sm btn-outline-secondary" :disabled="contentBusy" @click="openContentVersions(String(record.id))">版本</button>
                        <button v-if="contentItems.find((item) => item.id === String(record.id))?.status !== 'published'" type="button" class="btn btn-sm btn-outline-secondary" :disabled="contentBusy" @click="changeContentStatus(String(record.id), 'publish')">发布</button>
                        <button v-else type="button" class="btn btn-sm btn-outline-secondary" :disabled="contentBusy" @click="changeContentStatus(String(record.id), 'unpublish')">下架</button>
                        <button type="button" class="btn btn-sm btn-outline-danger content-danger-action" :disabled="contentBusy" @click="deleteContent(String(record.id))">删除</button>
                      </template>
                    </div>
                    <span v-else role="cell"><button type="button" class="table-cell-preview" :title="Array.from(tableCellValue(record, column.key)).length > 10 ? '点击查看完整内容' : undefined" @click="openTableCell(column.label, tableCellValue(record, column.key))">{{ tableCellPreview(tableCellValue(record, column.key)) }}</button></span>
                  </template>
                </div>
                <div class="virtual-table-spacer" :style="{ height: virtualTableBottomSpace + 'px' }"></div>
              </div>
              <div v-else class="virtual-table-empty"><i class="virtual-table-empty-icon bi bi-journal-text" aria-hidden="true"></i><strong>{{ contentLoading && activeAccountSection === 'content-management' ? '正在读取内容' : tableQuery ? '没有匹配的记录' : '暂无数据' }}</strong><p>{{ contentLoading && activeAccountSection === 'content-management' ? '正在从数据库读取内容列表。' : tableQuery ? '尝试更换搜索关键词。' : activeAccountSection === 'content-management' ? (contentViewMode === 'trash' ? '回收站为空。已删除内容可以恢复，也可以永久删除。' : '创建第一篇内容，保存为草稿后再发布。') : activeAccountSection === 'teaching-resources' ? (resourceViewMode === 'trash' ? '资源回收站为空。' : '创建第一份教学资源，支持 PPT、MP3、PDF、Word 等文件。') : activeTableDefinition.note }}</p></div>
            </div>
          </section>
        </template>
      </section>
    </main>

    <main v-else class="route-main"><section class="auth-panel glass-panel"><p class="eyebrow">PAGE NOT FOUND</p><h1>页面不存在</h1><a class="btn btn-dark button button-primary" href="/" @click.prevent="navigate('/')">返回首页</a></section></main>

    <div v-if="resourceDialogOpen" class="share-dialog-backdrop resource-upload-backdrop" @click.self="!resourceDialogLocked && (resourceDialogOpen = false)" @keydown.esc="!resourceDialogLocked && (resourceDialogOpen = false)">
      <section class="share-dialog resource-create-dialog" role="dialog" aria-modal="true" aria-labelledby="resource-create-title">
        <button v-if="!resourceDialogLocked && !resourceUploadDone" type="button" class="share-dialog-close" aria-label="关闭" @click="resourceDialogOpen = false"><i class="bi bi-x-lg" aria-hidden="true"></i></button>
        <span class="share-dialog-eyebrow">TEACHING LIBRARY</span>
        <h2 id="resource-create-title">{{ editingResourceId ? '编辑教学资源' : '创建教学资源' }}</h2>
        <p>{{ editingResourceId ? '可更新资源名称、类型、说明、分享链接；选择新文件才会替换原文件。' : '文件上传与 URL 分享二选一，也可以同时填写。' }}</p>
        <form v-if="!resourceUploadDone" class="resource-create-form" @submit.prevent="createTeachingResource" @paste="handleResourcePaste">
          <label class="share-dialog-label" for="resource-title">资源名称</label>
          <input id="resource-title" v-model="resourceTitle" class="form-control share-dialog-url" maxlength="200" required placeholder="例如：七年级英语听力 Unit 1" />
          <label class="share-dialog-label" for="resource-type">类型</label>
          <VirtualizedSelect v-model="resourceType" label="教学资源类型" :options="resourceTypeOptions.map(type => ({ label: type, value: type }))" />
          <label class="share-dialog-label" for="resource-grade">年级</label>
          <VirtualizedSelect v-model="resourceGrade" label="年级" :options="resourceGradeOptions.map(grade => ({ label: grade, value: grade === '不限定年级' ? '' : grade }))" placeholder="选择适用年级" />
          <label class="resource-public-toggle"><input v-model="resourceIsPublic" type="checkbox" /><span><strong>公开展示在教育资源页</strong><small>公开后访客可查看并访问资源。请确保你有权分享所提供的内容。</small></span></label>
          <label class="share-dialog-label" for="resource-description">资源说明</label>
          <textarea id="resource-description" v-model="resourceDescription" class="form-control share-dialog-url resource-description-input" maxlength="2000" placeholder="资源内容、课程、章节或使用说明（选填）"></textarea>
          <label class="share-dialog-label" for="resource-file">上传文件（选填）</label>
          <input id="resource-file" class="form-control resource-file-input" type="file" accept=".ppt,.pptx,.mp3,.pdf,.doc,.docx,.txt" @change="resourceFile = ($event.target as HTMLInputElement).files?.[0] || null" />
          <p class="resource-file-hint">支持 PPT / PPTX、MP3、PDF、DOC / DOCX、TXT；单个文件不超过 25 MB。除点击选择文件外，也可先复制文件，再在此表单中按 Ctrl+V 粘贴（取决于浏览器是否提供剪贴板文件）。不上传文件时可直接使用 URL。</p>
          <p v-if="resourceFile" class="resource-selected-file">{{ resourceFile.name }} · {{ formatFileSize(resourceFile.size) }}{{ editingResourceId ? ' · 上传后替换原文件' : '' }}</p>
          <label class="share-dialog-label" for="resource-url">URL 链接（选填）</label>
          <input id="resource-url" v-model="resourceUrl" type="url" class="form-control share-dialog-url" placeholder="粘贴可分享的 https:// 链接" />
          <div v-if="resourceBusy" class="resource-upload-live-progress">
            <div class="resource-upload-progress-track" role="progressbar" :aria-valuenow="resourceUploadProgress" aria-valuemin="0" aria-valuemax="100"><div class="resource-upload-progress-bar" :style="{ width: resourceUploadProgress + '%' }"></div></div>
            <div class="resource-upload-progress-label"><span>{{ resourceUploadProgress >= 100 ? '正在等待服务器确认保存' : '正在上传并保存，请勿关闭窗口' }}</span><strong>{{ resourceUploadProgress }}%</strong></div>
          </div>
          <p v-if="resourceError" class="resource-error">{{ resourceError }}</p>
          <div class="resource-form-actions"><button type="button" class="btn content-secondary-button" :disabled="resourceBusy" @click="resourceDialogOpen = false">取消</button><button type="submit" class="btn content-primary-button" :disabled="resourceBusy">{{ resourceBusy ? '正在保存…' : (editingResourceId ? '保存并处理' : '保存资源') }}</button></div>
        </form>
        <div v-else class="resource-upload-completion">
          <p>{{ resourceUploadResult }}</p>
          <div class="resource-upload-progress-track" role="progressbar" :aria-valuenow="resourceUploadProgress" aria-valuemin="0" aria-valuemax="100"><div class="resource-upload-progress-bar" :style="{ width: resourceUploadProgress + '%' }"></div></div>
          <div class="resource-upload-progress-label"><span>{{ resourceUploadProgress >= 100 ? '处理完成' : '正在处理' }}</span><strong>{{ resourceUploadProgress }}%</strong></div>
          <button type="button" class="btn content-primary-button" :disabled="resourceUploadProgress < 100 || !resourceUploadDone" @click="finishResourceUpload">完成</button>
        </div>
      </section>
    </div>

    <div v-if="selectedTableCell" class="table-cell-detail-backdrop" @click.self="selectedTableCell = null" @keydown.esc="selectedTableCell = null">
      <section class="table-cell-detail" role="dialog" aria-modal="true" :aria-label="selectedTableCell.label">
        <div class="table-cell-detail-heading"><h2>{{ selectedTableCell.label }}</h2><button type="button" aria-label="关闭" @click="selectedTableCell = null"><i class="bi bi-x-lg" aria-hidden="true"></i></button></div>
        <p>{{ selectedTableCell.value }}</p>
        <button type="button" class="btn content-primary-button" @click="selectedTableCell = null">关闭</button>
      </section>
    </div>

    <NotificationHost />

    <footer v-if="!isAccountPage && !isContentEditorPage" class="site-footer"><a class="footer-brand" href="/" @click.prevent="navigate('/')">资源平台 <span>©</span></a><span>连接资源、用户与行业服务</span><nav v-if="publishedLegalDocuments.length" class="site-footer-policies" aria-label="网站政策"> <a v-for="doc in publishedLegalDocuments" :key="doc.slug" :href="`/legal/${doc.slug}`" @click.prevent="navigate(`/legal/${doc.slug}`)">{{ doc.title }}</a></nav></footer>
    <button type="button" class="scroll-to-top" aria-label="返回页面顶部" title="返回顶部" @click="scrollToTop"><i class="bi bi-arrow-up" aria-hidden="true"></i></button>
  </div>
</template>

<style>
:root {
  font-family: var(--font-family-base);
  color: #191919;
  background: #f6f6f6;
  font-synthesis: none;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  font-optical-sizing: auto;
  font-weight: 400;
  --ink: #191919;
  --muted: #555555;
  --green: #191919;
  --line: #dedede;
  --paper: #f6f6f6;
  --mono: ui-monospace, SFMono-Regular, Consolas, monospace;
}

* { box-sizing: border-box; }
html { scroll-behavior: smooth; scroll-padding-top: 88px; }
body { margin: 0; min-width: 320px; }
a { color: inherit; text-decoration: none; }
button, input { font: inherit; }
button, a { -webkit-tap-highlight-color: transparent; }
button:focus-visible, a:focus-visible, input:focus-visible { outline: 2px solid #777777; outline-offset: 4px; }
.site-shell { max-width: 1600px; margin: 0 auto; overflow: clip; }
.site-header { height: 82px; padding: 0 clamp(22px, 7.1vw, 112px); display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--line); }
.brand { display: inline-flex; align-items: center; gap: 12px; }
.brand-mark { display: grid; place-items: center; width: 40px; height: 40px; background: var(--green); color: #ffffff; font-size: var(--font-size-21px-dacf9d); font-weight: 700; letter-spacing: -2px; border-radius: 11px 11px 11px 3px; }
.brand-copy { display: flex; flex-direction: column; gap: 3px; }
.brand-copy strong { font: 500 var(--font-size-base) var(--font-family-mono); letter-spacing: .12em; }
.brand-copy small { color: var(--muted); font-size: var(--font-size-14px-d65225); letter-spacing: .08em; }
.main-nav { display: flex; align-items: center; gap: clamp(22px, 3.4vw, 52px); font-size: var(--font-size-14px-d65225); color: #5e5e5e; }
.main-nav a { transition: color .2s, transform .22s ease; }
.main-nav a:hover { transform: translateY(-1px) scale(1.04); }
.main-nav a:hover { color: var(--green); }
.main-nav .nav-contact { border: 1px solid #d6d6d6; padding: 10px 15px; border-radius: 5px; color: var(--ink); }
.nav-contact span { margin-left: 13px; color: var(--green); }
.menu-toggle { display: none; }

.hero { position: relative; min-height: 585px; padding: 94px clamp(22px, 11vw, 176px) 38px; display: grid; grid-template-columns: 1.05fr .95fr; align-items: center; column-gap: 6%; border-bottom: 1px solid var(--line); }
.hero-copy { position: relative; z-index: 1; padding-bottom: 32px; }
.eyebrow { display: flex; align-items: center; gap: 10px; margin: 0 0 27px; font: 500 var(--font-size-base) var(--font-family-mono); letter-spacing: .13em; color: #737373; }
.status-dot { width: 6px; height: 6px; border-radius: 50%; background: #868686; box-shadow: 0 0 0 4px #ececec; }
.hero h1 { margin: 0; font-size: var(--font-size-clamp-37px-4-3vw-61px-3afbd4); line-height: 1.32; letter-spacing: -.055em; font-weight: 700; }
.hero h1 span { color: var(--green); }
.hero-description { max-width: 440px; margin: 22px 0 29px; color: #707070; font-size: var(--font-size-14px-d65225); line-height: 2; }
.hero-actions { display: flex; align-items: center; gap: 25px; }
.button { display: inline-flex; align-items: center; justify-content: center; gap: 0; text-align: center; min-height: 46px; padding: 0 18px; border-radius: 4px; font-size: var(--font-size-14px-d65225); }
.button-primary { color: #ffffff; background: var(--green); transition: background .2s, transform .2s; }
.button-primary:hover { background: #454545; transform: translateY(-2px); }
.text-link { font-size: var(--font-size-14px-d65225); color: #616161; }
.text-link span { margin-left: 8px; }
.hero-note { display: flex; align-items: center; gap: 12px; margin-top: 48px; color: #919191; font: var(--font-size-10px-min14-933cc7) var(--mono); letter-spacing: .04em; }
.note-line { width: 28px; height: 1px; background: #b1b1b1; }
.hero-art { position: relative; min-height: 365px; display: grid; place-items: center; }
.art-orbit { position: absolute; border: 1px solid #e3e3e3; border-radius: 50%; transform: rotate(-26deg); }
.orbit-one { width: min(31vw, 390px); height: min(24vw, 300px); }
.orbit-two { width: min(25vw, 315px); height: min(34vw, 420px); transform: rotate(35deg); border-color: #e9e9e9; }
.art-card { position: absolute; width: min(300px, 70%); height: 220px; border: 1px solid #dddddd; border-radius: 8px; box-shadow: 0 25px 55px #32323212; }
.card-back { background: #ebebeb; transform: translate(26px, -22px) rotate(7deg); padding: 24px; }
.art-card-label { font: var(--font-size-10px-min12-933cc7) var(--mono); letter-spacing: .12em; color: #818181; }
.art-lines { display: flex; flex-direction: column; gap: 12px; margin-top: 43px; }
.art-lines i { height: 5px; border-radius: 4px; background: #d8d8d8; width: 85%; }
.art-lines i:nth-child(2) { width: 62%; }
.art-lines i:nth-child(3) { width: 74%; }
.card-front { background: #fefefe; transform: translate(-13px, 17px) rotate(-5deg); padding: 23px; display: flex; flex-direction: column; }
.art-symbol { flex: 1; display: flex; align-items: center; justify-content: center; font-size: var(--font-size-98px-b5d82b); font-weight: 700; line-height: 1; letter-spacing: -.12em; color: #505050; }
.art-symbol span { color: #cacaca; }
.art-card-footer { display: flex; justify-content: space-between; border-top: 1px solid #e8e8e8; padding-top: 12px; color: #848484; font: var(--font-size-8px-min12-9bc5fe) var(--mono); letter-spacing: .06em; }
.art-index { position: absolute; bottom: 5px; right: 0; font: var(--font-size-9px-min14-22be94) var(--mono); color: #9a9a9a; letter-spacing: .1em; }
.art-spark { position: absolute; color: #8a8a8a; }
.spark-one { top: 32px; right: 20px; font-size: var(--font-size-28px-21f02b); }
.spark-two { bottom: 60px; left: 5px; font-size: var(--font-size-24px-b29b53); font-weight: 300; }
.hero-bottom { grid-column: 1 / -1; display: flex; align-items: center; justify-content: space-between; margin-top: 34px; padding-top: 22px; border-top: 1px solid var(--line); color: #919191; font-size: var(--font-size-14px-d65225); letter-spacing: .04em; }
.hero-bottom span:last-child { font: var(--font-size-9px-min14-22be94) var(--mono); letter-spacing: .08em; }
.hero-bottom i { display: inline-block; width: 40px; height: 1px; margin: 0 10px 3px; background: #bfbfbf; }

.explore-section { padding: 93px clamp(22px, 11vw, 176px) 104px; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; }
.section-heading .eyebrow { margin-bottom: 18px; }
.heading-rule { display: inline-block; width: 35px; height: 1px; background: #bcbcbc; }
.section-heading h2 { margin: 0; font-size: var(--font-size-clamp-27px-3vw-39px-1e6316); letter-spacing: -.04em; }
.section-intro { margin: 0 0 3px; color: #828282; font-size: var(--font-size-14px-d65225); line-height: 1.9; }
.search-box { display: flex; align-items: center; gap: 13px; height: 52px; margin: 38px 0 20px; padding: 0 16px; background: #ffffff; border: 1px solid #e3e3e3; border-radius: 5px; }
.search-icon { font-size: var(--font-size-26px-a7658f); line-height: 1; color: #808080; transform: rotate(-20deg); }
.search-box input { width: 100%; min-width: 0; border: 0; outline: 0; color: var(--ink); background: transparent; font-size: var(--font-size-14px-d65225); }
.search-box input::placeholder { color: #a8a8a8; }
.search-box kbd { white-space: nowrap; padding: 4px 6px; border: 1px solid #e5e5e5; border-radius: 3px; color: #a6a6a6; font: var(--font-size-9px-min14-22be94) var(--mono); }
.section-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; }
.section-card { display: flex; flex-direction: column; min-height: 295px; padding: 22px 19px 0; border: 1px solid #e3e3e3; background: #fcfcfc; border-radius: 5px; transition: transform .2s, border-color .2s, box-shadow .2s; }
.section-card:hover { transform: translateY(-4px); border-color: #c5c5c5; box-shadow: 0 14px 30px #3535350b; }
.card-topline { display: flex; align-items: center; justify-content: space-between; color: #a1a1a1; font: var(--font-size-10px-min12-933cc7) var(--mono); }
.card-arrow { color: #9a9a9a; font-size: var(--font-size-16px-4678bd); transition: transform .2s; }
.section-card:hover .card-arrow { transform: translate(2px, -2px); }
.card-kicker { margin: 26px 0 8px; color: #7c7c7c; font: var(--font-size-9px-min12-22be94) var(--mono); letter-spacing: .12em; }
.section-card h3 { margin: 0; font-size: var(--font-size-21px-dacf9d); letter-spacing: -.03em; }
.card-description { min-height: 68px; margin: 12px 0 15px; color: #818181; font-size: var(--font-size-14px-d65225); line-height: 1.85; }
.tag-list { display: flex; flex-wrap: wrap; gap: 5px; margin-bottom: 22px; }
.tag-list span { padding: 5px 7px; border: 1px solid #e8e8e8; border-radius: 3px; color: #929292; font-size: var(--font-size-14px-d65225); }
.card-bottom { display: flex; justify-content: space-between; align-items: center; margin-top: auto; padding: 13px 0; border-top: 1px solid #eaeaea; color: #797979; font-size: var(--font-size-14px-d65225); }
.card-bottom span:last-child { color: var(--green); font-size: var(--font-size-14px-d65225); }
.empty-search { padding: 45px 20px; text-align: center; border: 1px dashed #dddddd; color: var(--muted); }
.empty-search strong { color: var(--ink); }
.empty-search p { font-size: var(--font-size-14px-d65225); }
.empty-search button { text-align: center; padding: 8px 12px; border: 1px solid #dadada; border-radius: 4px; background: transparent; color: var(--green); cursor: pointer; }

.about-section { position: relative; display: grid; grid-template-columns: .35fr 1.3fr .7fr; align-items: center; gap: 40px; padding: 64px clamp(22px, 11vw, 176px); background: #eeeeee; border-top: 1px solid #e2e2e2; border-bottom: 1px solid #e2e2e2; }
.about-mark { display: grid; place-items: center; width: 86px; height: 86px; border: 1px solid #c8c8c8; border-radius: 50%; color: #575757; font-size: var(--font-size-39px-dc4873); font-weight: 700; letter-spacing: -.12em; }
.about-copy .eyebrow { margin-bottom: 15px; color: #797979; }
.about-copy h2 { margin: 0; font-size: var(--font-size-clamp-25px-2-8vw-35px-b05804); line-height: 1.5; letter-spacing: -.04em; }
.about-copy > p:last-child { max-width: 500px; margin: 14px 0 0; color: #727272; font-size: var(--font-size-14px-d65225); line-height: 2; }
.about-meta { align-self: end; display: flex; flex-direction: column; gap: 9px; padding-bottom: 5px; color: #858585; font: var(--font-size-9px-min14-22be94) var(--mono); letter-spacing: .08em; }
.about-meta span:last-child { color: #969696; font: var(--font-size-11px-min14-4bd46d) var(--font-family-base); letter-spacing: 0; }

.site-footer { min-height: 78px; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px clamp(22px, 7.1vw, 112px); color: #909090; font-size: var(--font-size-14px-d65225); }
.footer-brand { color: #696969; font: var(--font-size-10px-min14-933cc7) var(--mono); letter-spacing: .08em; }
.footer-brand span { margin-left: 5px; }
.site-footer > a:last-child { color: #666666; }
.site-footer-policies { display:flex; flex-wrap:wrap; gap:12px 18px; align-items:center; }
.site-footer-policies a { color:#696969; text-decoration:none; font-size:12px; }
.site-footer-policies a:hover { color:#191919; text-decoration:underline; text-underline-offset:3px; }
.legal-document-page { min-height:55vh; padding:clamp(28px,6vw,80px) clamp(20px,7vw,112px); background:#fff; }
.legal-document-shell { max-width:900px; margin:0 auto; }
.legal-back-link { display:inline-block; color:#666; text-decoration:none; margin-bottom:36px; font-size:13px; }
.legal-document-content h1 { font-size:clamp(28px,4vw,42px); letter-spacing:-.04em; line-height:1.2; margin:0 0 18px; }
.legal-document-effective { color:#777; font-size:12px; margin:0 0 28px; }
.legal-markdown { color:#333; font-size:15px; line-height:1.9; overflow-wrap:anywhere; }
.legal-markdown h1,.legal-markdown h2,.legal-markdown h3 { margin:1.7em 0 .65em; line-height:1.4; }
.legal-markdown p,.legal-markdown ul,.legal-markdown ol { margin:.8em 0; }
.legal-markdown a { color:#333; text-underline-offset:3px; overflow-wrap:anywhere; }
.legal-document-state { border:1px solid #e7e7e7; padding:28px; border-radius:10px; color:#666; }
.legal-document-state h1 { color:#222; font-size:25px; }

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
  .brand-copy strong { font-size: var(--font-size-14px-d65225); }
  .brand-copy small { font-size: var(--font-size-14px-d65225); }
  .menu-toggle { display: flex; flex-direction: column; gap: 5px; padding: 10px 4px; border: 0; background: transparent; }
  .menu-toggle span { display: block; width: 23px; height: 1px; background: var(--ink); }
  .main-nav { position: absolute; z-index: 5; top: 69px; left: 0; right: 0; display: none; align-items: stretch; gap: 0; padding: 8px 20px 16px; background: var(--paper); border-bottom: 1px solid var(--line); }
  .main-nav.is-open { display: flex; flex-direction: column; }
  .main-nav a { padding: 13px 0; }
  .main-nav .nav-contact { border: 0; padding: 13px 0; }
  .hero { grid-template-columns: 1fr; padding: 67px 22px 25px; }
  .hero-copy { padding-bottom: 0; }
  .hero .eyebrow { margin-bottom: 22px; font-size: var(--font-size-14px-d65225); }
  .hero h1 { font-size: var(--font-size-clamp-34px-9vw-49px-310138); }
  .hero-description { margin-top: 17px; font-size: var(--font-size-14px-d65225); }
  .hero-note { margin-top: 30px; }
  .hero-art { min-height: 310px; margin-top: 10px; }
  .art-card { width: min(270px, 72%); height: 200px; }
  .orbit-one { width: 78vw; height: 55vw; }
  .orbit-two { width: 60vw; height: 85vw; }
  .art-index { right: 4px; bottom: 0; font-size: var(--font-size-14px-d65225); }
  .hero-bottom { margin-top: 26px; padding-top: 17px; font-size: var(--font-size-14px-d65225); }
  .hero-bottom span:last-child { font-size: var(--font-size-14px-d65225); }
  .hero-bottom i { width: 18px; margin-inline: 5px; }
  .explore-section { padding: 65px 20px 70px; }
  .section-heading { align-items: flex-start; flex-direction: column; gap: 15px; }
  .section-heading h2 { font-size: var(--font-size-29px-be22be); }
  .section-intro br { display: none; }
  .search-box { margin-top: 25px; }
  .search-box kbd { display: none; }
  .section-grid { grid-template-columns: 1fr; gap: 10px; }
  .section-card { min-height: 250px; padding-inline: 20px; }
  .card-kicker { margin-top: 20px; }
  .card-description { min-height: 0; max-width: 360px; }
  .about-section { grid-template-columns: 1fr; gap: 22px; padding: 43px 22px; }
  .about-mark { width: 60px; height: 60px; font-size: var(--font-size-28px-21f02b); }
  .about-copy h2 { font-size: var(--font-size-27px-228d28); }
  .about-meta { grid-column: auto; flex-direction: column; gap: 8px; }
  .site-footer { align-items: flex-start; flex-direction: column; padding: 23px 20px; gap: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; animation-duration: .01ms !important; }
}

/* Typography scale: Cloudflare-inspired, clearer hierarchy and larger readable text */
.brand-copy strong { font-size: var(--font-size-14px-d65225); }
.brand-copy small { font-size: var(--font-size-14px-d65225); }
.main-nav { font-size: var(--font-size-15px-344c38); }
.main-nav .nav-contact { padding: 12px 17px; }
.hero .eyebrow,
.section-heading .eyebrow,
.about-copy .eyebrow { font-size: var(--font-size-14px-d65225); }
.hero h1 { font-size: var(--font-size-clamp-44px-5vw-72px-df2163); line-height: 1.24; }
.hero-description { max-width: 510px; font-size: var(--font-size-17px-157d8a); line-height: 1.9; }
.button { min-height: 52px; padding-inline: 22px; font-size: var(--font-size-15px-344c38); }
.hero-actions { gap: 28px; }
.text-link { font-size: var(--font-size-15px-344c38); }
.hero-note { font-size: var(--font-size-14px-d65225); }
.hero-bottom { font-size: var(--font-size-14px-d65225); }
.hero-bottom span:last-child { font-size: var(--font-size-14px-d65225); }
.art-card-label { font-size: var(--font-size-14px-d65225); }
.art-card-footer { font-size: var(--font-size-14px-d65225); }
.art-index { font-size: var(--font-size-14px-d65225); }
.explore-section { padding-top: 100px; padding-bottom: 112px; }
.section-heading h2 { font-size: var(--font-size-clamp-34px-3-4vw-48px-ca24b4); }
.section-intro { font-size: var(--font-size-15px-344c38); line-height: 1.85; }
.search-box { min-height: 60px; }
.search-box input { font-size: var(--font-size-15px-344c38); }
.search-box kbd { font-size: var(--font-size-14px-d65225); padding: 5px 7px; }
.section-grid { gap: 18px; }
.section-card { min-height: 340px; padding: 25px 22px 0; }
.card-topline { font-size: var(--font-size-14px-d65225); }
.card-kicker { font-size: var(--font-size-14px-d65225); margin-top: 28px; }
.section-card h3 { font-size: var(--font-size-25px-2e6593); }
.card-description { min-height: 84px; font-size: var(--font-size-14px-d65225); line-height: 1.9; }
.tag-list { gap: 6px; }
.tag-list span { padding: 6px 9px; font-size: var(--font-size-14px-d65225); }
.card-bottom { padding: 15px 0; font-size: var(--font-size-14px-d65225); }
.card-bottom span:last-child { font-size: var(--font-size-16px-4678bd); }
.empty-search strong { font-size: var(--font-size-18px-6d27d5); }
.empty-search p { font-size: var(--font-size-15px-344c38); }
.empty-search button { padding: 10px 15px; font-size: var(--font-size-14px-d65225); }
.about-section { padding-top: 72px; padding-bottom: 72px; }
.about-copy h2 { font-size: var(--font-size-clamp-30px-3-2vw-43px-53ba6d); line-height: 1.4; }
.about-copy > p:last-child { max-width: 560px; font-size: var(--font-size-15px-344c38); line-height: 1.9; }
.about-meta { font-size: var(--font-size-14px-d65225); }
.about-meta span:last-child { font-size: var(--font-size-14px-d65225); }
.site-footer { font-size: var(--font-size-14px-d65225); }
.footer-brand { font-size: var(--font-size-14px-d65225); }
@media (min-width: 1400px) {
  .section-card { min-height: 355px; padding-inline: 25px; }
}
@media (max-width: 1050px) {
  .hero h1 { font-size: var(--font-size-clamp-42px-5-3vw-60px-069bf8); }
  .hero-description { font-size: var(--font-size-16px-4678bd); }
  .section-card { min-height: 310px; }
  .card-description { min-height: 66px; }
  .about-copy h2 { font-size: var(--font-size-clamp-30px-3-6vw-40px-beef96); }
}
@media (max-width: 700px) {
  .brand-copy strong { font-size: var(--font-size-14px-d65225); }
  .brand-copy small { font-size: var(--font-size-14px-d65225); }
  .main-nav { font-size: var(--font-size-16px-4678bd); }
  .main-nav a, .main-nav .nav-contact { padding-top: 15px; padding-bottom: 15px; }
  .hero { padding-top: 62px; }
  .hero .eyebrow { font-size: var(--font-size-14px-d65225); line-height: 1.6; letter-spacing: .09em; }
  .hero h1 { font-size: var(--font-size-clamp-36px-8-8vw-50px-ade76b); line-height: 1.3; }
  .hero-description { font-size: var(--font-size-15px-344c38); line-height: 1.9; }
  .button { min-height: 49px; padding-inline: 18px; font-size: var(--font-size-14px-d65225); }
  .hero-actions { gap: 18px; flex-wrap: wrap; }
  .text-link { font-size: var(--font-size-14px-d65225); }
  .hero-note { margin-top: 27px; font-size: var(--font-size-14px-d65225); }
  .hero-bottom { font-size: var(--font-size-14px-d65225); }
  .hero-bottom span:last-child { font-size: var(--font-size-14px-d65225); }
  .art-card-label { font-size: var(--font-size-14px-d65225); }
  .art-card-footer { font-size: var(--font-size-14px-d65225); }
  .art-index { font-size: var(--font-size-14px-d65225); }
  .explore-section { padding-top: 70px; padding-bottom: 76px; }
  .section-heading .eyebrow { font-size: var(--font-size-14px-d65225); }
  .section-heading h2 { font-size: var(--font-size-clamp-30px-7-4vw-36px-ada4bf); }
  .section-intro { font-size: var(--font-size-14px-d65225); line-height: 1.85; }
  .search-box { min-height: 56px; margin-top: 25px; gap: 10px; padding-inline: 14px; }
  .search-box input { font-size: var(--font-size-14px-d65225); }
  .section-grid { gap: 13px; }
  .section-card { min-height: 0; padding: 22px 20px 0; }
  .card-topline { font-size: var(--font-size-14px-d65225); }
  .card-kicker { margin-top: 21px; font-size: var(--font-size-14px-d65225); }
  .section-card h3 { font-size: var(--font-size-24px-b29b53); }
  .card-description { min-height: 0; margin-top: 11px; margin-bottom: 16px; font-size: var(--font-size-14px-d65225); line-height: 1.85; }
  .tag-list { margin-bottom: 19px; gap: 6px; }
  .tag-list span { padding: 6px 9px; font-size: var(--font-size-14px-d65225); }
  .card-bottom { padding: 13px 0; font-size: var(--font-size-14px-d65225); }
  .about-section { gap: 21px; padding-top: 45px; padding-bottom: 45px; }
  .about-copy .eyebrow { font-size: var(--font-size-14px-d65225); }
  .about-copy h2 { font-size: var(--font-size-clamp-28px-7vw-34px-0a9fe1); line-height: 1.45; }
  .about-copy > p:last-child { font-size: var(--font-size-14px-d65225); line-height: 1.9; }
  .about-meta { font-size: var(--font-size-14px-d65225); }
  .about-meta span:last-child { font-size: var(--font-size-14px-d65225); }
  .site-footer { font-size: var(--font-size-14px-d65225); }
  .footer-brand { font-size: var(--font-size-14px-d65225); }
}


/* Routed account pages; keep the established Liquid Glass palette and type scale. */
.route-main { min-height: min(78vh, 900px); display: grid; place-items: center; padding: clamp(34px, 7vw, 82px) 24px; }
.glass-panel { width: min(100%, 560px); padding: clamp(28px, 5vw, 52px); border: 0; border-radius: 28px; background: linear-gradient(145deg, rgba(255, 255, 255, .78), rgba(243, 243, 243, .62)); box-shadow: 0 24px 70px rgba(54, 54, 54, .10); -webkit-backdrop-filter: blur(26px) saturate(145%); backdrop-filter: blur(26px) saturate(145%); }
.auth-panel h1, .account-panel h1 { margin: 14px 0 12px; font-size: var(--font-size-clamp-36px-4-5vw-52px-659835); line-height: 1.12; letter-spacing: -.045em; }
.auth-intro { color: var(--muted); line-height: 1.8; font-size: var(--font-size-16px-4678bd); margin: 0 0 28px; }
.auth-form { display: grid; gap: 18px; }
.field-label { display: grid; gap: 9px; color: var(--ink); font-weight: 600; font-size: var(--font-size-15px-344c38); }
.field-label input { width: 100%; min-height: 52px; border: 1px solid rgba(148, 148, 148, .35); border-radius: 14px; padding: 0 16px; outline: none; color: var(--ink); background: rgba(255, 255, 255, .78); transition: border-color .18s, box-shadow .18s; }
.field-label input:focus { border-color: #929292; box-shadow: 0 0 0 4px rgba(133, 133, 133, .13); }
.field-help { margin: -8px 0 0; font-size: var(--font-size-14px-d65225); line-height: 1.7; color: var(--muted); }
.form-error, .form-notice { padding: 12px 14px; border-radius: 12px; margin: 0; font-size: var(--font-size-14px-d65225); line-height: 1.65; }
.form-error { color: #434343; background: rgba(237, 237, 237, .85); border: 1px solid rgba(110, 110, 110, .2); }
.form-notice { color: #545454; background: rgba(239, 239, 239, .78); border: 1px solid rgba(124, 124, 124, .2); margin-bottom: 18px; }
.auth-submit { width: 100%; min-height: 52px; justify-content: center; border: 0; margin-top: 2px; }
.auth-submit:disabled { opacity: .65; cursor: wait; }
.auth-switch { margin: 24px 0 0; text-align: center; color: var(--muted); font-size: var(--font-size-14px-d65225); }
.auth-switch a { color: var(--green); font-weight: 650; text-decoration: underline; text-underline-offset: 3px; }
.back-home { display: inline-block; margin-top: 26px; color: var(--muted); font-size: var(--font-size-14px-d65225); }
.account-panel { width: min(100%, 650px); }
.account-details { display: grid; gap: 0; }
.account-avatar { display: grid; place-items: center; width: 68px; height: 68px; border-radius: 22px; margin: 0 0 20px; color: #ffffff; font-size: var(--font-size-30px-46d0b6); font-weight: 650; background: linear-gradient(145deg, #a1a1a1, #777777); }
.account-field { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 18px 0; border-bottom: 1px solid rgba(143, 143, 143, .2); }
.account-field span { color: var(--muted); font-size: var(--font-size-14px-d65225); }
.account-field strong { color: var(--ink); font-size: var(--font-size-15px-344c38); font-weight: 600; text-align: right; overflow-wrap: anywhere; }
.account-details .auth-submit { margin-top: 28px; }
.account-loading { padding: 22px 0; color: var(--muted); }
.main-nav .nav-auth { padding: 11px 19px; border: 1px solid rgba(134, 134, 134, .32); border-radius: 999px; background: rgba(255, 255, 255, .72); color: #4a4a4a; font-weight: 650; box-shadow: 0 4px 14px rgba(74, 74, 74, .06); }
.main-nav .nav-auth:hover { background: rgba(255, 255, 255, .92); color: #3c3c3c; }
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
  border-right: 1px solid rgba(73, 73, 73, .09);
  border-radius: 0;
  background: rgba(249, 249, 249, .26);
  box-shadow: none;
  -webkit-backdrop-filter: blur(20px) saturate(130%);
  backdrop-filter: blur(20px) saturate(130%);
}
.account-sidebar::-webkit-scrollbar { display: none; width: 0; height: 0; }
.account-sidebar-heading { display: none; }
.account-nav-group + .account-nav-group { margin-top: 5px; }
.account-group-toggle { font-weight: 600; }
.account-sidebar-collapse { padding: 4px 8px 8px 14px; }
.account-sidebar-subitem { width: calc(100% - 6px); min-height: 42px; margin-left: 6px; padding-left: 11px; font-size: var(--font-size-15px-344c38); }
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
  color: #6a6a6a;
  text-align: left;
  font-size: var(--font-size-16px-4678bd);
  background: transparent;
  transition: color .18s ease, background .18s ease;
}
.account-sidebar-item:hover { color: #4a4a4a; background: rgba(255, 255, 255, .32); }
.account-sidebar-item.is-active { color: #494949; background: #e9e9e9; box-shadow: inset 3px 0 0 #707070; font-weight: 650; }
.account-sidebar-subitem { width: calc(100% - 6px); min-height: 42px; margin-left: 6px; padding: 0 10px; font-size: var(--font-size-15px-344c38); }
.virtual-table-panel { display: flex; flex-direction: column; min-width: 0; min-height: 0; height: calc(100% - 94px); margin: 0; overflow: hidden; border: 0; border-radius: 0; background: transparent; box-shadow: none; }
.virtual-table-toolbar { display: flex; flex: 0 0 auto; align-items: center; justify-content: space-between; gap: 18px; padding: 12px 18px; border-bottom: 1px solid rgba(154, 154, 154, .16); }
.virtual-table-heading { display: flex; align-items: baseline; flex-wrap: wrap; gap: 12px; min-width: 0; }
.virtual-table-heading h2 { margin: 0; color: #444444; font-size: var(--font-size-18px-6d27d5); font-weight: 650; letter-spacing: -.02em; }
.virtual-table-count { color: #939393; font-size: var(--font-size-14px-d65225); white-space: nowrap; }
.virtual-table-search { width: min(260px, 42%); min-width: 140px; height: 38px; padding: 0 12px; border: 1px solid rgba(147, 147, 147, .2); border-radius: 10px; color: #484848; background: rgba(255, 255, 255, .56); font-size: var(--font-size-14px-d65225); outline: none; }
.virtual-table-search:focus { border-color: rgba(116, 116, 116, .5); box-shadow: 0 0 0 3px rgba(116, 116, 116, .08); }
.virtual-table-scroll { flex: 1 1 auto; min-width: 0; min-height: 0; max-height: none; overflow-x: scroll; overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
.virtual-table-header, .virtual-table-row { display: grid; grid-template-columns: var(--table-columns); width: 100%; min-width: 100%; align-items: center; box-sizing: border-box; }
.virtual-table-header { position: sticky; top: 0; z-index: 1; min-height: 44px; color: #828282; background: rgba(246, 246, 246, .96); border-bottom: 1px solid rgba(154, 154, 154, .16); font-size: var(--font-size-14px-d65225); font-weight: 600; }
.virtual-table-header span, .virtual-table-row span { overflow: hidden; padding: 0 16px; text-overflow: ellipsis; white-space: nowrap; }
.virtual-table-row { min-height: 44px; color: #535353; border-bottom: 1px solid rgba(154, 154, 154, .11); font-size: var(--font-size-14px-d65225); }
.virtual-table-row:hover { background: rgba(255, 255, 255, .5); }
.virtual-table-empty { display: flex; min-height: 220px; flex-direction: column; align-items: center; justify-content: center; padding: 30px 18px; text-align: center; }
.virtual-table-empty-icon { display: grid; place-items: center; width: 38px; height: 38px; margin-bottom: 12px; border: 1px solid rgba(154, 154, 154, .18); border-radius: 12px; color: #818181; background: rgba(255, 255, 255, .52); font-size: var(--font-size-18px-6d27d5); }
.virtual-table-empty strong { color: #646464; font-size: var(--font-size-14px-d65225); font-weight: 600; }
.virtual-table-empty p { margin: 7px 0 0; color: #969696; font-size: var(--font-size-14px-d65225); }
.virtual-table-footnote { margin: 0; padding: 10px 16px; border-top: 1px solid rgba(154, 154, 154, .12); color: #a4a4a4; font-size: var(--font-size-14px-d65225); }
@media (max-width: 680px) { .virtual-table-toolbar { align-items: stretch; flex-direction: column; gap: 12px; padding: 12px; } .virtual-table-search { width: 100%; } .account-content-table-mode h1.account-page-title { margin: 12px 12px 4px; } .account-content-table-mode > .auth-intro { margin: 0 12px 8px; } .virtual-table-panel { height: calc(100% - 86px); } }
.workspace-placeholder { margin-top: 28px; padding: 34px; border-radius: 22px; text-align: center; background: rgba(255, 255, 255, .28); }
.workspace-placeholder-icon { display: grid; place-items: center; width: 54px; height: 54px; margin: 0 auto 14px; border-radius: 18px; color: #5a5a5a; background: rgba(255, 255, 255, .55); font-size: var(--font-size-28px-21f02b); }
.workspace-placeholder h2 { margin: 0 0 8px; color: #444444; font-size: var(--font-size-20px-998b02); }
.workspace-placeholder p { margin: 0; color: #848484; font-size: var(--font-size-14px-d65225); }
.sidebar-item-dot { width: 6px; height: 6px; flex: 0 0 6px; border-radius: 50%; background: rgba(121, 121, 121, .4); }
.account-sidebar-item.is-active .sidebar-item-dot { background: #616161; box-shadow: 0 0 0 4px rgba(112, 112, 112, .16); }
.account-sidebar-home { display: block; margin: 4px 20px 20px; padding-top: 16px; border-top: 1px solid rgba(73, 73, 73, .07); color: #7f7f7f; font-size: var(--font-size-14px-d65225); }
.account-sidebar-home:hover { color: #5a5a5a; }
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
.account-content-table-mode h1.account-page-title { flex: 0 0 auto; margin: 16px 18px 4px; font-size: var(--font-size-clamp-26px-2-4vw-34px-91deb5); }
.account-content-table-mode > .auth-intro { display: none; }
.account-search-empty { padding: 10px 8px; color: #8a8a8a; font-size: var(--font-size-14px-d65225); text-align: center; }
.account-content::-webkit-scrollbar { display: none; width: 0; height: 0; }
.account-content h1 { margin: 14px 0 12px; font-size: var(--font-size-clamp-34px-3-4vw-48px-ca24b4); line-height: 1.16; letter-spacing: -.045em; }
.account-content h1.account-page-title { margin-top: 0; }
.sidebar-group-icon { display: inline-grid; place-items: center; flex: 0 0 20px; width: 20px; height: 20px; border-radius: 6px; color: #5a5a5a; background: rgba(255, 255, 255, .52); font-size: var(--font-size-14px-d65225); font-weight: 500; line-height: 1; }
.account-summary-card {
  display: flex;
  align-items: center;
  gap: 17px;
  padding: 20px;
  margin: 4px 0 14px;
  border: 1px solid rgba(73, 73, 73, .065);
  border-radius: 20px;
  background: rgba(255, 255, 255, .25);
}
.account-summary-card .account-avatar { flex: 0 0 60px; width: 60px; height: 60px; margin: 0; border-radius: 19px; }
.account-summary-card > div:nth-child(2) { display: grid; min-width: 0; gap: 6px; }
.account-summary-name { color: #3e3e3e; font-size: var(--font-size-18px-6d27d5); overflow-wrap: anywhere; }
.account-summary-email { color: #818181; font-size: var(--font-size-14px-d65225); overflow-wrap: anywhere; }
.account-status { display: inline-flex; align-items: center; gap: 7px; margin-left: auto; white-space: nowrap; color: #747474; font-size: var(--font-size-14px-d65225); }
.account-status i { width: 6px; height: 6px; border-radius: 50%; background: #8f8f8f; }
.security-info-row { display: flex; align-items: center; gap: 14px; padding: 20px; border: 1px solid rgba(126, 126, 126, .12); border-radius: 18px; background: rgba(242, 242, 242, .35); }
.security-info-icon { display: grid; place-items: center; flex: 0 0 36px; height: 36px; border-radius: 50%; color: #686868; background: rgba(255, 255, 255, .6); font-size: var(--font-size-18px-6d27d5); }
.security-info-row strong { color: #545454; font-size: var(--font-size-15px-344c38); }
.security-info-row p { margin: 5px 0 0; color: #818181; font-size: var(--font-size-14px-d65225); }
@media (max-width: 760px) {
  .account-layout { height: 100dvh; min-height: 0; max-height: 100dvh; grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); align-content: stretch; gap: 0; padding: 0; overflow: hidden; }
  .account-sidebar { position: relative; height: auto; max-height: 34dvh; border-radius: 0; border-right: 0; border-bottom: 1px solid rgba(73, 73, 73, .09); padding: 10px 12px; }
  .account-content { height: 100%; max-height: 100%; overflow: auto; padding: 26px 21px; }
  .account-sidebar-heading { min-height: 72px; padding: 14px 16px; }
  .account-sidebar-collapse { padding: 4px 8px 8px; }
  .account-sidebar-collapse .account-sidebar-item { min-height: 40px; }
  .account-sidebar-home { margin: 4px 12px 10px; }
  .account-content { min-height: 0; padding: 26px 21px; }
  .account-content.account-content-table-mode { padding: 0; }
  .account-content h1 { font-size: var(--font-size-clamp-30px-8vw-38px-5af08c); }
  .account-summary-card { align-items: flex-start; flex-wrap: wrap; padding: 16px; gap: 12px; }
  .account-summary-card .account-avatar { flex-basis: 52px; width: 52px; height: 52px; }
  .account-status { margin-left: 0; }
}
@media (prefers-reduced-transparency: reduce) {
  .account-sidebar, .account-content { -webkit-backdrop-filter: none; backdrop-filter: none; }
  .account-sidebar { background: #f2f2f2; }
  .account-content { background: transparent; }
}


/* Content management editor and row actions */
.content-toolbar-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; min-width: 0; }
.content-editor-panel { width: 100%; min-width: 0; padding: 18px clamp(12px, 2vw, 28px) 28px; overflow: auto; }
.content-editor-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-bottom: 22px; }
.content-editor-heading h2 { margin: 0; color: #444444; font-size: var(--font-size-21px-dacf9d); font-weight: 650; }
.content-editor-heading p { margin: 7px 0 0; color: #828282; font-size: var(--font-size-14px-d65225); line-height: 1.6; }
.content-editor-form { display: grid; gap: 17px; max-width: 1000px; }
.notion-content-form { max-width: 1180px; margin: 0 auto; gap: 0; }
.notion-title-input { margin: 12px 0 22px; padding: 0 !important; border: 0 !important; border-radius: 0 !important; background: transparent !important; box-shadow: none !important; color: #2d2d2d !important; font-size: var(--font-size-clamp-30px-4vw-42px-981052) !important; font-weight: 720 !important; letter-spacing: -.045em; line-height: 1.25; }
.notion-title-input::placeholder { color: #bfbfbf; }
.notion-document-properties { display: grid; grid-template-columns: minmax(160px, .7fr) minmax(160px, .7fr) minmax(250px, 1.6fr); gap: 10px; padding: 0 0 20px; border-bottom: 1px solid #f0f0f0; }
.notion-document-properties label, .notion-summary-property { display: grid; gap: 7px; min-width: 0; }
.notion-document-properties label > span, .notion-summary-property > span { color: #858585; font-size: var(--font-size-14px-d65225); font-weight: 600; }
.notion-document-properties input, .notion-document-properties select { min-height: 36px !important; padding: 7px 9px !important; border: 1px solid #eaeaea !important; border-radius: 6px !important; background: #fcfcfc !important; font-size: var(--font-size-14px-d65225) !important; }
.notion-document-properties small { font-size: var(--font-size-14px-d65225) !important; }
.notion-summary-property { padding: 17px 0 14px; }
.notion-summary-property textarea { min-height: 48px; resize: vertical; border: 0 !important; border-radius: 0 !important; padding: 5px 0 !important; background: transparent !important; box-shadow: none !important; font-size: var(--font-size-14px-d65225) !important; }
.notion-document-divider { height: 1px; background: #f0f0f0; margin-bottom: 12px; }
.notion-document-actions { align-items: center; margin-top: 20px; padding-top: 16px; border-top: 1px solid #f0f0f0; }
.notion-document-actions > div { display: flex; gap: 8px; }
.notion-save-hint { color: #959595; font-size: var(--font-size-14px-d65225); }
.content-editor-form label { display: grid; gap: 8px; color: #535353; font-size: var(--font-size-14px-d65225); font-weight: 650; }
.content-editor-form input, .content-editor-form select, .content-editor-form textarea { width: 100%; min-width: 0; border: 1px solid rgba(147, 147, 147, .24); border-radius: 11px; padding: 11px 13px; color: #484848; background: rgba(255, 255, 255, .65); font: inherit; font-weight: 400; outline: none; }
.content-editor-form input, .content-editor-form select { min-height: 42px; }
.content-editor-form textarea { resize: vertical; line-height: 1.75; }
.content-editor-form input:focus, .content-editor-form select:focus, .content-editor-form textarea:focus { border-color: rgba(116, 116, 116, .56); box-shadow: 0 0 0 3px rgba(116, 116, 116, .09); }
.content-editor-form small { color: #8e8e8e; font-size: var(--font-size-14px-d65225); font-weight: 400; }
.content-form-row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.content-editor-actions { display: flex; justify-content: flex-end; gap: 10px; padding-top: 4px; }
.content-primary-button, .content-secondary-button { flex: 0 0 auto; min-height: 38px; padding: 0 15px; border: 1px solid transparent; border-radius: 10px; font-size: var(--font-size-14px-d65225); font-weight: 650; cursor: pointer; transition: transform .18s, background .18s, box-shadow .18s; }
.content-primary-button { color: white; background: linear-gradient(135deg, #888888, #585858); box-shadow: 0 5px 14px rgba(90, 90, 90, .16); }
.content-secondary-button { color: #646464; background: rgba(255, 255, 255, .52); border-color: rgba(147, 147, 147, .2); }
.content-primary-button:hover, .content-secondary-button:hover { transform: translateY(-1px); }
.content-primary-button:disabled, .content-secondary-button:disabled, .content-row-actions button:disabled { opacity: .55; cursor: wait; }
.content-feedback { margin: 0 0 12px; padding: 10px 13px; border-radius: 10px; font-size: var(--font-size-14px-d65225); line-height: 1.65; }
.content-feedback-error { color: #434343; background: rgba(237, 237, 237, .8); }
.content-feedback-success { color: #545454; background: rgba(239, 239, 239, .78); }
.content-row-actions { display: flex; align-items: center; gap: 7px; padding: 0 10px; white-space: nowrap; }
.content-row-actions button { padding: 5px 8px; border: 1px solid rgba(147, 147, 147, .18); border-radius: 7px; color: #5a5a5a; background: rgba(255, 255, 255, .48); font-size: var(--font-size-14px-d65225); cursor: pointer; }
.content-row-actions .content-danger-action { color: #5d5d5d; }
.content-version-list { display: grid; gap: 0; border-top: 1px solid rgba(147, 147, 147, .2); }
.content-version-row { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 16px 4px; border-bottom: 1px solid rgba(147, 147, 147, .16); }
.content-version-row strong { color: #484848; font-size: var(--font-size-14px-d65225); }
.content-version-row p { margin: 6px 0 0; color: #8e8e8e; font-size: var(--font-size-14px-d65225); }
@media (max-width: 680px) { .content-version-row { align-items: flex-start; flex-direction: column; gap: 10px; } }
@media (max-width: 680px) {
  .content-toolbar-actions { width: 100%; align-items: stretch; flex-direction: column; }
  .content-toolbar-actions .virtual-table-search { width: 100%; }
  .content-form-row { grid-template-columns: 1fr; }
  .content-editor-heading { align-items: flex-start; }
  .content-row-actions { padding: 0 8px; }
}


/* Tiptap block editor */
.content-editor-shell{position:relative;min-width:0;background:#ffffff;border-radius:13px}.content-editor-hint{display:flex;justify-content:space-between;gap:16px;padding:12px 24px;border-bottom:1px solid #f0f0f0;color:#999999;font-size: var(--font-size-14px-d65225)}.content-editor-hint>span:first-child{font-weight:650;color:#616161}.content-editor-hint kbd{border:1px solid #e0e0e0;border-radius:4px;padding:1px 4px;background:#fbfbfb;font:inherit}.content-editor-canvas{position:relative;min-height:480px;padding:32px clamp(22px,6vw,78px) 56px}.content-editor-document{min-height:380px;outline:none;color:#3d3d3d;font-size:var(--font-size-15px-344c38);line-height:1.85;overflow-wrap:anywhere;caret-color:#676767}.content-editor-document>*{margin-top:0;margin-bottom:12px}.content-editor-document p{min-height:1.85em}.content-editor-document h1,.content-editor-document h2,.content-editor-document h3{color:#2b2b2b;font-weight:700;line-height:1.35;letter-spacing:-.025em}.content-editor-document h1{font-size:var(--font-size-30px-46d0b6)}.content-editor-document h2{font-size:var(--font-size-23px-75f47a)}.content-editor-document h3{font-size:var(--font-size-18px-6d27d5)}.content-editor-document ul,.content-editor-document ol{padding-left:1.7em}.content-editor-document blockquote{margin:12px 0;padding:4px 0 4px 15px;border-left:3px solid #cccccc;color:#666666}.content-editor-document pre{overflow:auto;padding:15px;border-radius:7px;color:#ececec;background:#2a2a2a;font-size: var(--font-size-14px-d65225)}.content-editor-document code{padding:2px 5px;border-radius:4px;color:#565656;background:#f2f2f2;font-family:ui-monospace,monospace;font-size: var(--font-size-14px-d65225)}.content-editor-document pre code{padding:0;color:inherit;background:transparent}.content-editor-document a{color:#686868;text-underline-offset:3px}.content-editor-document mark{background:#eeeeee}.content-editor-document img{max-width:100%;height:auto;border-radius:6px}.content-editor-document table{width:100%;border-collapse:collapse;table-layout:fixed}.content-editor-document th,.content-editor-document td{min-width:80px;padding:7px 10px;border:1px solid #e5e5e5;vertical-align:top}.content-editor-document th{background:#f7f7f7}.content-editor-document ul[data-type=taskList]{list-style:none;padding-left:0}.content-editor-document ul[data-type=taskList]>li{display:flex;gap:8px;align-items:flex-start}.content-editor-document ul[data-type=taskList]>li>label{margin-top:5px}.content-editor-document ul[data-type=taskList]>li>div{flex:1;min-width:0}.content-editor-document ul[data-type=taskList]>li[data-checked=true]>div{color:#a2a2a2;text-decoration:line-through}.content-editor-document p.is-editor-empty:first-child:before{float:left;height:0;color:#acacac;content:attr(data-placeholder);pointer-events:none}.content-editor-bubble-menu{display:flex;gap:3px;padding:5px;border:1px solid #e4e4e4;border-radius:9px;background:#272727;box-shadow:0 8px 24px #1f1f1f25}.content-editor-bubble-menu button{min-width:28px;height:28px;border:0;border-radius:5px;color:#ffffff;background:transparent;cursor:pointer}.content-editor-bubble-menu button:hover,.content-editor-bubble-menu button.active{background:#515151}.content-editor-drag-handle{display:grid;place-items:center;width:25px;height:29px;border:0;border-radius:5px;background:transparent;color:#a2a2a2;cursor:grab;font-size:var(--font-size-19px-cb8c27)}.content-editor-drag-handle:hover{background:#f2f2f2;color:#494949}.content-editor-slash-menu,.content-editor-block-menu{position:absolute;z-index:20;top:20px;left:24px;display:grid;width:min(300px,calc(100% - 48px));max-height:330px;overflow:auto;padding:7px;border:1px solid #e7e7e7;border-radius:10px;background:#ffffff;box-shadow:0 12px 35px #29292920}.content-editor-menu-caption{display:flex;justify-content:space-between;gap:8px;padding:8px;color:#929292;font-size: var(--font-size-14px-d65225);font-weight:650}.content-editor-menu-caption small{font-size: var(--font-size-14px-d65225);font-weight:400}.content-editor-slash-menu button,.content-editor-block-menu button{display:flex;align-items:center;gap:9px;padding:7px;border:0;border-radius:6px;background:transparent;color:#404040;text-align:left;font:inherit;font-size: var(--font-size-14px-d65225);cursor:pointer}.content-editor-slash-menu button.selected,.content-editor-slash-menu button:hover,.content-editor-block-menu button:hover{background:#f4f4f4}.content-editor-menu-icon{display:grid;place-items:center;flex:0 0 28px;width:28px;height:28px;border:1px solid #e9e9e9;border-radius:6px;color:#636363;font-size: var(--font-size-14px-d65225)}.content-editor-slash-menu button>span:last-child{display:grid;gap:2px}.content-editor-slash-menu strong{font-size: var(--font-size-14px-d65225);font-weight:600}.content-editor-slash-menu small{color:#949494;font-size: var(--font-size-14px-d65225)}.content-editor-block-menu .content-editor-cancel-action{color:#949494}.content-editor-footer{display:flex;justify-content:space-between;gap:10px;padding:10px 24px;border-top:1px solid #f0f0f0;color:#959595;font-size: var(--font-size-14px-d65225)}
.content-editor-no-results{padding:8px;color:#929292;font-size: var(--font-size-14px-d65225)}
.content-editor-shell.is-disabled{opacity:.76}
.content-editor-bubble-menu button:disabled{opacity:.5;cursor:not-allowed}
@media(max-width:680px){.content-editor-hint,.content-editor-footer{flex-direction:column;padding:10px 14px}.content-editor-canvas{padding:24px 20px 42px}.content-editor-document h1{font-size:var(--font-size-25px-2e6593)}}

/* Public content listing and article view */
.published-content-section { padding: 10px clamp(22px, 11vw, 176px) 96px; }
.published-content-heading { display: flex; align-items: end; justify-content: space-between; gap: 18px; margin-bottom: 20px; }
.published-content-heading h2 { margin: 8px 0 0; color: #444444; font-size: var(--font-size-clamp-26px-3vw-38px-deca7a); letter-spacing: -.035em; }
.published-content-heading > span { color: #828282; font-size: var(--font-size-14px-d65225); }
.published-content-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.published-content-card { display: flex; min-width: 0; flex-direction: column; padding: 22px; border: 1px solid rgba(73, 73, 73, .075); border-radius: 18px; color: inherit; text-decoration: none; background: rgba(249, 249, 249, .42); box-shadow: 0 10px 34px rgba(53, 53, 53, .06); -webkit-backdrop-filter: blur(22px) saturate(145%); backdrop-filter: blur(22px) saturate(145%); transition: transform .2s, box-shadow .2s, background .2s; }
.published-content-card:hover { transform: translateY(-3px); background: rgba(252, 252, 252, .65); box-shadow: 0 16px 38px rgba(53, 53, 53, .11); }
.published-content-type { color: #777777; font-size: var(--font-size-14px-d65225); font-weight: 650; }
.published-content-card h3 { margin: 12px 0 8px; color: #444444; font-size: var(--font-size-18px-6d27d5); line-height: 1.5; overflow-wrap: anywhere; }
.published-content-card p { flex: 1; margin: 0 0 20px; color: #828282; font-size: var(--font-size-14px-d65225); line-height: 1.8; overflow-wrap: anywhere; }
.published-content-link { color: #5a5a5a; font-size: var(--font-size-14px-d65225); font-weight: 650; }
.published-content-empty { padding: 22px; border-radius: 14px; color: #828282; background: rgba(249, 249, 249, .35); font-size: var(--font-size-14px-d65225); }
.public-content-page { min-height: 76vh; padding: clamp(32px, 7vw, 82px) 24px; }
.public-article { width: min(100%, 900px); margin: 0 auto; }
.public-article-route-header { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 0 0 18px; color: #6f6f6f; font-size: var(--font-size-13px-min14-233710); line-height: 1.5; }
.public-article-route-type { color: #3f3f3f; font-weight: 650; }
.public-article-route-date { color: #8c8c8c; }
.public-article-route-date::before { content: '·'; margin-right: 10px; color: #b5b5b5; }
.public-article h1 { margin: 0; color: #3b3b3b; font-size: var(--font-size-clamp-34px-5vw-58px-f830bb); line-height: 1.25; letter-spacing: -.04em; overflow-wrap: anywhere; }
.public-article-summary { margin: 22px 0 0; color: #737373; font-size: var(--font-size-17px-157d8a); line-height: 1.9; }
.public-article-body { margin-top: 38px; padding-top: 30px; border-top: 1px solid rgba(63, 63, 63, .09); color: #535353; font-size: var(--font-size-16px-4678bd); line-height: 1.85; overflow-wrap: anywhere; }
.content-markdown-workspace { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: stretch; gap: 16px; }
.content-markdown-source { min-width: 0; }
.content-markdown-source textarea { min-height: 390px; resize: vertical; font-family: var(--mono); font-size: var(--font-size-14px-d65225); tab-size: 2; }
.content-markdown-preview { min-width: 0; min-height: 390px; overflow: auto; border: 1px solid rgba(147, 147, 147, .2); border-radius: 11px; background: rgba(255, 255, 255, .5); }
.content-markdown-preview-heading { position: sticky; top: 0; z-index: 1; padding: 12px 14px; border-bottom: 1px solid rgba(147, 147, 147, .16); color: #828282; background: rgba(247, 247, 247, .96); font-size: var(--font-size-14px-d65225); font-weight: 650; }
.content-markdown-preview-empty { padding: 18px; color: #969696; font-size: var(--font-size-14px-d65225); }
.markdown-rendered { color: #535353; font-size: var(--font-size-15px-344c38); line-height: 1.85; overflow-wrap: anywhere; }
.markdown-rendered > :first-child { margin-top: 0; }
.markdown-rendered > :last-child { margin-bottom: 0; }
.markdown-rendered .md-align-center { text-align: center; }
.markdown-rendered .md-align-right { text-align: right; }
.markdown-rendered h1, .markdown-rendered h2, .markdown-rendered h3, .markdown-rendered h4 { margin: 1.45em 0 .65em; color: #434343; line-height: 1.4; letter-spacing: -.025em; }
.markdown-rendered h1 { font-size: var(--font-size-1-8em-8f4eb0); }
.markdown-rendered h2 { font-size: var(--font-size-1-5em-bf0de9); }
.markdown-rendered h3 { font-size: var(--font-size-1-22em-30fc4a); }
.markdown-rendered p { margin: .8em 0; }
.markdown-rendered ul, .markdown-rendered ol { padding-left: 1.6em; margin: .8em 0; }
.markdown-rendered li + li { margin-top: .25em; }
.markdown-rendered blockquote { margin: 1em 0; padding: .15em 1em; border-left: 3px solid #b3b3b3; color: #7b7b7b; background: rgba(237, 237, 237, .35); }
.markdown-rendered a { color: #606060; text-decoration: underline; text-underline-offset: 3px; }
.markdown-rendered hr { border: 0; border-top: 1px solid rgba(147, 147, 147, .24); margin: 1.6em 0; }
.markdown-rendered code { padding: .12em .35em; border-radius: 5px; color: #595959; background: rgba(234, 234, 234, .75); font-family: var(--mono); font-size: var(--font-size-14px-d65225); }
.markdown-rendered pre { max-width: 100%; overflow: auto; padding: 15px; border-radius: 9px; color: #ececec; background: #323232; line-height: 1.65; }
.markdown-rendered pre code { padding: 0; color: inherit; background: transparent; font-size: var(--font-size-14px-d65225); }
.markdown-rendered table { width: 100%; border-collapse: collapse; margin: 1.2em 0; font-size: var(--font-size-14px-d65225); }
.markdown-rendered th, .markdown-rendered td { padding: 8px 10px; border: 1px solid rgba(147, 147, 147, .24); text-align: left; }
.markdown-rendered th { background: rgba(235, 235, 235, .6); font-weight: 650; }
.markdown-rendered img { max-width: 100%; height: auto; border-radius: 8px; }
.public-content-state { display: flex; width: min(100%, 620px); min-height: 320px; flex-direction: column; align-items: flex-start; justify-content: center; gap: 12px; margin: 0 auto; color: #646464; }
.public-content-state h1 { margin: 0; color: #444444; font-size: var(--font-size-32px-3056f7); }
.public-content-state p { margin: 0 0 10px; line-height: 1.8; }
@media (max-width: 900px) { .published-content-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) {
  .published-content-section { padding: 0 20px 60px; }
  .published-content-grid { grid-template-columns: 1fr; gap: 10px; }
  .published-content-heading { align-items: flex-start; flex-direction: column; gap: 8px; }
  .public-content-page { padding: 30px 20px 56px; }
  .public-article-summary { font-size: var(--font-size-15px-344c38); }
  .public-article-body { margin-top: 28px; font-size: var(--font-size-15px-344c38); }
  .content-markdown-workspace { grid-template-columns: minmax(0, 1fr); }
  .content-markdown-source textarea { min-height: 260px; }
  .content-markdown-preview { min-height: 220px; }
}

.resource-create-dialog { max-height: min(86vh, 900px); overflow-y: auto; scrollbar-width: none; -ms-overflow-style: none; }
.resource-create-dialog::-webkit-scrollbar { display: none; }
.resource-create-form .form-select { border-radius: 12px; border-color: #dedede; padding: 12px 14px; color: #191919; background-color: rgba(255,255,255,.72); }
.resource-upload-completion { display: grid; gap: 18px; margin-top: 26px; }
.resource-upload-live-progress { display: grid; gap: 10px; margin-top: 16px; }
.resource-upload-progress-track { height: 10px; border-radius: 999px; background: #e5e5e5; overflow: hidden; }
.resource-upload-progress-bar { height: 100%; border-radius: inherit; background: #222; transition: width .18s ease; }
.resource-upload-progress-label { display: flex; justify-content: space-between; color: #555; font-size: var(--font-size-9rem-df724a); }
.resource-upload-completion .content-primary-button { justify-self: end; min-width: 110px; }
</style>

<style src="./theme.css"></style>
