<!--
Responsibility: compose portal routes, page-level reactive state, and the user-facing application template.
Dependencies: focused composables for URL resolution, authentication, data loading, editing, and table behavior.
Collaboration: App.vue coordinates route/session gates and view state; Go remains the authority for authentication and permissions.
Boundaries: avoid embedding reusable business workflows here; browser lifecycle and route sequencing remain explicit,
while extracted composables own independently testable state transitions and side effects.
-->
<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, onUpdated, ref, watch } from 'vue'
import { apiRequest } from './api/client'
import { useKnowledgeDirectory } from './composables/useKnowledgeDirectory'
import { useDebouncedValue } from './composables/useDebouncedValue'
import VirtualizedSelect from './components/VirtualizedSelect.vue'
import { mountNotificationHost } from './components/notificationHost'
import { mountTeachingResourceDrawer } from './components/teachingResourceDrawer'
import { portalNotify } from './utils/notification'
import { taxonomyChildOptions } from './utils/taxonomy'
import { knowledgeTagOptions, taxonomyParentNames as knowledgeTaxonomyParentNames } from './utils/knowledgeTags'
import { useKnowledgeTagSelection } from './composables/useKnowledgeTagSelection'
import { useScrollRestoration } from './composables/useScrollRestoration'
import { usePublicArticleActions } from './composables/usePublicArticleActions'
import { useLegalDocuments } from './composables/useLegalDocuments'
import { filterEducationResources } from './utils/resourceFiltering'
import { tableCellPreview, tableCellValue } from './utils/accountTablePresentation'
import { useAccountTable } from './composables/useAccountTable'
import { usePortalAuthentication, type PortalUser } from './composables/usePortalAuthentication'
import { createProtectedRouteGuard } from './composables/useProtectedRouteGuard'
import { formatResourceFileSize } from './utils/resourceFiles'
import { resourceFileIcon } from './utils/resourceIcons'
import { createRetryableLazyLoader } from './utils/retryableLazyLoader'
import { accountGroups, accountGroupIcon } from './navigation/accountNavigation'
import { routeSections, sectionRoutes } from './navigation/portalRoutes'
import { resolvePortalLocation } from './navigation/resolvePortalLocation'
import { directoryPages, directoryTiles } from './config/publicDirectories'
import { contentStatusLabels, contentTypeLabels } from './config/accountTables'
import { homeSections } from './config/homeSections'
import { knowledgeCategoryMenus } from './config/knowledgeCategories'
import { knowledgeFuzzyMatch } from './utils/knowledgeSearch'
import type { Content as APIContent, ContentVersion as APIContentVersion, TeachingResource as APITeachingResource } from './api/generated'

const ComplianceCenter = defineAsyncComponent(() => import('./components/ComplianceCenter.vue'))
const SystemOpsCenter = defineAsyncComponent(() => import('./components/SystemOpsCenter.vue'))
const TeachingResourceEditorDialog = defineAsyncComponent(() => import('./components/TeachingResourceEditorDialog.vue'))
const ContentDocumentEditor = defineAsyncComponent(() => import('./components/ContentDocumentEditor.vue'))
const taxonomyParentNames = knowledgeTaxonomyParentNames
const visibilityOptions = [{ label: '仅自己可见', value: 'private' }, { label: '公开', value: 'public' }]

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
const contentEditorRouteId = computed(() => route.value.match(/^\/content-management\/([0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})$/i)?.[1] || null)
const isContentEditorPage = computed(() => route.value === '/content-management/new' || contentEditorRouteId.value !== null)
const isProtectedPage = computed(() => isContentEditorPage.value || route.value === '/account' || Object.prototype.hasOwnProperty.call(routeSections, route.value))
const isAccountPage = computed(() => !isContentEditorPage.value && (route.value === '/account' || Object.prototype.hasOwnProperty.call(routeSections, route.value)))
const activeSidebarButton = ref('overview')
const accountNavQuery = ref('')
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



const tableQuery = ref('')
const debouncedTableQuery = useDebouncedValue(tableQuery, 150)
watch(debouncedTableQuery, (query) => {
  if (activeAccountSection.value === 'content-management') void loadContentList(query)
  else if (activeAccountSection.value === 'teaching-resources') void loadTeachingResources(query)
})
const tableScrollTop = ref(0)
const virtualTableScrollElement = ref<HTMLElement | null>(null)
const virtualTableViewportWidth = ref(0)
let tableResizeObserver: ResizeObserver | null = null
const selectedTableCell = ref<{ label: string; value: string } | null>(null)
const selectedResourceDetail = ref<TeachingResource | null>(null)
const teachingResourceDrawerHost = ref<HTMLElement | null>(null)
let disposeTeachingResourceDrawer: (() => void) | null = null
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
const resourceDialogItem = ref<TeachingResource | null>(null)
const resourceBusy = ref(false)
const resourceError = ref('')
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
const {
  dialogOpen: knowledgeTagDialogOpen,
  trigger: knowledgeTagTrigger,
  dialog: knowledgeTagDialog,
  draft: knowledgeTagDraft,
  expandedGroups: expandedKnowledgeTagGroups,
  selectedCount: selectedKnowledgeTagCount,
  draftCount: knowledgeTagDraftCount,
  isSelected: isKnowledgeTagSelected,
  open: openKnowledgeTagDialog,
  close: closeKnowledgeTagDialog,
  toggleGroup: toggleKnowledgeTagGroup,
  toggleTag: toggleKnowledgeTag,
  clearDraft: clearKnowledgeTags,
  removeSelected: removeSelectedKnowledgeTag,
  clearSelected: clearSelectedKnowledgeTags,
} = useKnowledgeTagSelection(contentForm, contentBusy)
const publicContents = ref<PublicContentCard[]>([])
const publicTeachingResources = ref<PublicTeachingResource[]>([])
const educationResourceLoading = ref(false)
const educationResourceError = ref('')
const educationResourceCategory = ref('全部资源')
const educationResourceGrade = ref('全部年级')
const educationResourceSearch = ref('')
const educationResourceTypes = ['全部资源', ...resourceTypeOptions]
const educationResourceGrades = ['全部年级', ...taxonomyChildOptions('年级').map(option => option.value)]

const filteredEducationResources = computed(() => filterEducationResources(
  publicTeachingResources.value,
  educationResourceCategory.value,
  educationResourceGrade.value,
  educationResourceSearch.value,
))
const visibleEducationResources = computed(() => filteredEducationResources.value.slice(0, knowledgeVisibleCount.value))

const publicArticle = ref<PublicContent | null>(null)
const publicArticleHtml = ref('')
const publicContentLoading = ref(false)
const publicContentError = ref('')
const publicInteractionBusy = ref(false)
const shareDialogOpen = ref(false)
const shareUrl = ref('')
watch(publicContentError, (message) => { if (message) portalNotify.error(message, '内容读取失败') }, { flush: 'sync' })
const {
  activeTableDefinition,
  filteredTableRecords,
  virtualTableStart,
  visibleTableRecords,
  tableColumnTemplate,
  virtualTableTopSpace,
  virtualTableBottomSpace,
} = useAccountTable({
  activeAccountSection,
  contentViewMode,
  contentItems,
  contentTrashItems,
  resourceViewMode,
  teachingResources,
  teachingResourceTrash,
  debouncedTableQuery,
  tableScrollTop,
  virtualTableViewportWidth,
})
function openTableCell(label: string, value: string) {
  selectedTableCell.value = { label, value }
}
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
  tableScrollTop.value = 0
  await nextTick()
  if (virtualTableScrollElement.value) virtualTableScrollElement.value.scrollTop = 0
  observeVirtualTableWidth()
})
watch([activeAccountSection, contentViewMode, resourceViewMode, debouncedTableQuery], async () => {
  tableScrollTop.value = 0
  await nextTick()
  if (virtualTableScrollElement.value) virtualTableScrollElement.value.scrollTop = 0
})
function onVirtualTableScroll(event: Event) {
  tableScrollTop.value = (event.target as HTMLElement).scrollTop
}
type ContentListLoader = ReturnType<typeof import('./composables/loadContentList').createContentListLoader>
const getContentListLoader = createRetryableLazyLoader<ContentListLoader>(() =>
  import('./composables/loadContentList').then(({ createContentListLoader }) => createContentListLoader({
    viewMode: contentViewMode,
    activeItems: contentItems,
    trashItems: contentTrashItems,
    loading: contentLoading,
    error: contentError,
  })),
)
function loadContentList(searchTerm = tableQuery.value) {
  return getContentListLoader().then(loader => loader.loadContentList(searchTerm)).catch((error: unknown) => {
    contentError.value = error instanceof Error ? error.message : '网络连接失败，无法读取内容。'
    contentLoading.value = false
  })
}
function switchContentView(mode: 'active' | 'trash') {
  contentViewMode.value = mode
  versionsContentId.value = null
  tableQuery.value = ''
  void loadContentList()
}
type TeachingResourceListLoader = ReturnType<typeof import('./composables/loadTeachingResourceList').createTeachingResourceListLoader>
const getTeachingResourceListLoader = createRetryableLazyLoader<TeachingResourceListLoader>(() =>
  import('./composables/loadTeachingResourceList').then(({ createTeachingResourceListLoader }) => createTeachingResourceListLoader({
    viewMode: resourceViewMode,
    activeItems: teachingResources,
    trashItems: teachingResourceTrash,
    loading: resourceBusy,
    error: resourceError,
    onError: message => portalNotify.error(message, '教学资源'),
  })),
)
function loadTeachingResources(searchTerm = tableQuery.value) {
  return getTeachingResourceListLoader().then(loader => loader.loadTeachingResources(searchTerm)).catch((error: unknown) => {
    resourceError.value = error instanceof Error ? error.message : '读取教学资源失败。'
    resourceBusy.value = false
    portalNotify.error(resourceError.value, '教学资源')
  })
}
type ContentManagementActions = ReturnType<typeof import('./composables/useContentManagementActions').useContentManagementActions>
const getContentManagementActions = createRetryableLazyLoader<ContentManagementActions>(() =>
  import('./composables/useContentManagementActions').then(({ useContentManagementActions }) => useContentManagementActions({
    routeContentId: contentEditorRouteId,
    contentItems,
    contentTrashItems,
    contentBusy,
    contentError,
    contentNotice,
    contentEditorOpen,
    editingContentId,
    contentForm,
    contentVersions,
    versionsContentId,
    versionsContentTitle,
    emptyContentForm,
    loadContentList,
    navigate,
  })),
)
function runContentManagementAction<T>(action: (actions: ContentManagementActions) => Promise<T> | T): Promise<T | undefined> {
  return getContentManagementActions().then(action).catch((error: unknown) => {
    contentError.value = error instanceof Error ? error.message : '内容操作失败。'
    contentBusy.value = false
    return undefined
  })
}
function initializeContentEditorRoute() { return runContentManagementAction(actions => actions.initializeContentEditorRoute()) }
function openContentVersions(id: string) { return runContentManagementAction(actions => actions.openContentVersions(id)) }
function restoreContentVersion(version: ContentVersion) { return runContentManagementAction(actions => actions.restoreContentVersion(version)) }
function restoreDeletedContent(id: string) { return runContentManagementAction(actions => actions.restoreDeletedContent(id)) }
function permanentlyDeleteContent(id: string) { return runContentManagementAction(actions => actions.permanentlyDeleteContent(id)) }
function saveContent() { return runContentManagementAction(actions => actions.saveContent()) }
function changeContentStatus(id: string, action: 'publish' | 'unpublish') { return runContentManagementAction(actions => actions.changeContentStatus(id, action)) }
function deleteContent(id: string) { return runContentManagementAction(actions => actions.deleteContent(id)) }

function openResourceDialog() { resourceDialogItem.value = null; resourceDialogOpen.value = true }
function openResourceEdit(item: TeachingResource) { resourceDialogItem.value = item; resourceDialogOpen.value = true }
function openResourceEditById(id: string) { const item = teachingResources.value.find((resource) => resource.id === id); if (item) openResourceEdit(item) }
function openResourceDetails(id: string) { selectedResourceDetail.value = [...teachingResources.value, ...teachingResourceTrash.value].find((resource) => resource.id === id) || null }
function closeResourceDetails() { selectedResourceDetail.value = null }
function onResourceDialogSaved() { void loadTeachingResources() }
function onResourceDialogCompleted() { portalNotify.success('教学资源已保存。') }
async function changeTeachingResource(id: string, action: 'trash' | 'restore' | 'permanent') {
  const message = action === 'permanent' ? '永久删除后将同时删除服务器文件，无法恢复。确定继续？' : action === 'trash' ? '将该资源移入回收站？' : ''
  if (message && !window.confirm(message)) return
  resourceBusy.value = true
  try {
    const suffix = action === 'restore' ? '/restore' : action === 'permanent' ? '/permanent' : ''
    await apiRequest<unknown>(`/resources/${encodeURIComponent(id)}${suffix}`, { method: action === 'restore' ? 'POST' : 'DELETE' }, '资源操作失败。')
    await loadTeachingResources(); portalNotify.success(action === 'trash' ? '已移入回收站。' : action === 'restore' ? '资源已恢复。' : '资源及文件已永久删除。')
  } catch (error) { portalNotify.error(error instanceof Error ? error.message : '资源操作失败。', '教学资源') }
  finally { resourceBusy.value = false }
}
function switchResourceView() { resourceViewMode.value = resourceViewMode.value === 'active' ? 'trash' : 'active'; tableQuery.value = ''; void loadTeachingResources() }
function downloadTeachingResource(id: string) { const item = teachingResources.value.find((resource) => resource.id === id); if (item?.url) window.open(item.url, '_blank', 'noopener,noreferrer'); else window.location.href = `/api/resources/${encodeURIComponent(id)}/download` }
watch([selectedResourceDetail, resourceViewMode], ([resource, mode]) => {
  disposeTeachingResourceDrawer?.()
  disposeTeachingResourceDrawer = null
  if (!resource || !teachingResourceDrawerHost.value) return
  disposeTeachingResourceDrawer = mountTeachingResourceDrawer(teachingResourceDrawerHost.value, resource, mode, {
    onClose: closeResourceDetails,
    onEdit: (id) => { closeResourceDetails(); openResourceEditById(id) },
    onDownload: downloadTeachingResource,
  })
}, { flush: 'post' })


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
  window.location.assign('/content-management/new')
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
function editContent(id: string) {
  window.location.assign('/content-management/' + encodeURIComponent(id))
}
const filteredSections = computed(() => {
  const keyword = query.value.trim().toLocaleLowerCase()
  if (!keyword) return homeSections
  return homeSections.filter((section) => [section.title, section.subtitle, section.description, ...section.tags].join(' ').toLocaleLowerCase().includes(keyword))
})
const isRegister = computed(() => route.value === '/register')
const isAuthPage = computed(() => route.value === '/login' || route.value === '/register')
const isPublicContentPage = computed(() => /^\/content\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value))
const {
  legalSlug,
  publishedLegalDocuments,
  legalDocument,
  legalDocumentHtml,
  legalDocumentLoading,
  legalDocumentError,
  loadPublishedLegalDocuments,
  loadLegalDocument,
} = useLegalDocuments(route)
const isPublicDirectoryPage = computed(() => Object.prototype.hasOwnProperty.call(directoryPages, route.value))
const currentDirectoryPage = computed(() => directoryPages[route.value] || directoryPages['/knowledge']!)
const currentDirectoryTiles = computed(() => directoryTiles[route.value] || [])
const directoryContentItems = computed(() => {
  const contentType = route.value === '/knowledge' ? 'article' : route.value === '/education-resources' ? 'resource' : route.value === '/software-tools' ? 'tool' : null
  return contentType ? publicContents.value.filter((item) => item.contentType === contentType) : []
})
const directoryContentHeading = computed(() => route.value === '/knowledge' ? '最新文章' : route.value === '/education-resources' ? '已发布的学习资料' : '已发布的工具')

const {
  knowledgeVisibleCount,
  knowledgePageSize,
  knowledgeCategory,
  knowledgeSubcategory,
  knowledgeSearchQuery,
  highlightedKnowledgeSuggestion,
  knowledgeFilteredItems,
  knowledgeSearchSuggestions,
  visibleDirectoryContentItems,
  selectedKnowledgeCategoryLabel,
  chooseKnowledgeSuggestion,
  queryKnowledgeSearch,
  handleKnowledgeAutocompleteSelect,
  onKnowledgeSearchKeydown,
  submitKnowledgeSearch,
  selectKnowledgeCategory,
  chooseKnowledgeCategory,
  scrollKnowledgeCategories,
  loadMoreKnowledge,
  onKnowledgePageScroll,
} = useKnowledgeDirectory(route, directoryContentItems, filteredEducationResources, navigate)

function closeMenu() { menuOpen.value = false }
function setRouteFromLocation() {
  const resolved = resolvePortalLocation(window.location.pathname, window.location.search)
  route.value = resolved.route
  if (route.value === '/knowledge') {
    knowledgeCategory.value = resolved.knowledgeCategory
    knowledgeSubcategory.value = resolved.knowledgeSubcategory
    knowledgeVisibleCount.value = knowledgePageSize
  }
  if (isAccountPage.value) {
    const section = routeSections[route.value] || 'overview'
    activeAccountSection.value = section
    activeSidebarButton.value = section
  }
}
const { saveScrollPosition, restoreScrollPosition } = useScrollRestoration(route)
function onScrollCapture() { saveScrollPosition(); onKnowledgePageScroll() }
type PublicDirectoryLoader = ReturnType<typeof import('./composables/loadPublicDirectoryData').createPublicDirectoryLoader>
const getPublicDirectoryLoader = createRetryableLazyLoader<PublicDirectoryLoader>(() =>
  import('./composables/loadPublicDirectoryData').then(({ createPublicDirectoryLoader }) => createPublicDirectoryLoader({
    publicContents,
    publicTeachingResources,
    publicContentLoading,
    publicContentError,
    educationResourceLoading,
    educationResourceError,
    knowledgeVisibleCount,
    knowledgePageSize,
    isContentViewCurrent: () => route.value === '/' || route.value === '/knowledge' || (isPublicDirectoryPage.value && route.value !== '/education-resources'),
  })),
)
const debouncedKnowledgeSearchQuery = useDebouncedValue(knowledgeSearchQuery, 220)
const debouncedEducationResourceSearchQuery = useDebouncedValue(educationResourceSearch, 220)
let navigationSequence = 0
function loadPublicContents(searchTerm = '') {
  return getPublicDirectoryLoader().then(loader => loader.loadPublicContents(searchTerm)).catch((error: unknown) => {
    publicContentError.value = error instanceof Error ? error.message : '读取公开内容失败。'
    publicContentLoading.value = false
  })
}
function loadPublicTeachingResources(searchTerm = '') {
  return getPublicDirectoryLoader().then(loader => loader.loadPublicTeachingResources(searchTerm)).catch((error: unknown) => {
    educationResourceError.value = error instanceof Error ? error.message : '读取公开教育资源失败。'
    educationResourceLoading.value = false
  })
}
watch(debouncedKnowledgeSearchQuery, (query) => {
  if (route.value !== '/knowledge') return
  void loadPublicContents(query)
})
watch(debouncedEducationResourceSearchQuery, (query) => {
  if (route.value !== '/education-resources') return
  void loadPublicTeachingResources(query)
})
type PublicArticleLoader = ReturnType<typeof import('./composables/loadPublicArticle').createPublicArticleLoader>
const getPublicArticleLoader = createRetryableLazyLoader<PublicArticleLoader>(() =>
  import('./composables/loadPublicArticle').then(({ createPublicArticleLoader }) => createPublicArticleLoader({
    route,
    article: publicArticle,
    html: publicArticleHtml,
    loading: publicContentLoading,
    error: publicContentError,
  })),
)
function loadPublicArticle(slug: string) {
  return getPublicArticleLoader().then(loader => loader.loadPublicArticle(slug)).catch((error: unknown) => {
    publicContentError.value = error instanceof Error ? error.message : '读取内容失败。'
    publicContentLoading.value = false
  })
}
const { toggleInteraction: togglePublicInteraction, shareArticle: sharePublicArticle, copyShareUrl } = usePublicArticleActions({
  article: publicArticle,
  busy: publicInteractionBusy,
  authNotice,
  shareDialogOpen,
  shareUrl,
  navigate,
})
function loadRouteData(): void {
  if (route.value === '/' || (isPublicDirectoryPage.value && route.value !== '/education-resources')) void loadPublicContents()
  if (route.value === '/education-resources') void loadPublicTeachingResources()
  if (isContentEditorPage.value) void initializeContentEditorRoute()
  if (isPublicContentPage.value) void loadPublicArticle(route.value.slice('/content/'.length))
  if (legalSlug.value) void loadLegalDocument(legalSlug.value)
}

const { ensureRouteAccess } = createProtectedRouteGuard({
  isProtected: () => isProtectedPage.value,
  loadSession: () => loadSession(),
  isCurrent: (navigationId: number) => navigationId === navigationSequence,
  redirectToLogin: () => {
    history.replaceState({}, '', '/login')
    route.value = '/login'
    authNotice.value = '请先登录，再进入工作空间。'
  },
})

async function navigate(target: string) {
  const navigationId = ++navigationSequence
  saveScrollPosition()
  closeMenu()
  history.pushState({}, '', target)
  setRouteFromLocation()
  void loadPublishedLegalDocuments()
  authError.value = ''
  authNotice.value = ''
  if (!await ensureRouteAccess(navigationId)) return
  if (route.value === '/content-management') {
    await loadContentList()
    if (navigationId !== navigationSequence) return
  }
  if (route.value === '/education/teaching-resources') {
    await loadTeachingResources()
    if (navigationId !== navigationSequence) return
  }
  if (navigationId !== navigationSequence) return
  loadRouteData()
  if (route.value === '/' && window.location.hash) {
    await nextTick()
    if (navigationId !== navigationSequence) return
    document.querySelector(window.location.hash)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  } else {
    window.scrollTo({ top: 0, behavior: 'auto' })
    if (isAccountPage.value) {
      await nextTick()
      if (navigationId !== navigationSequence) return
      const content = document.querySelector('.account-content') as HTMLElement | null
      const table = document.querySelector('.virtual-table-scroll') as HTMLElement | null
      if (content) content.scrollTop = 0
      if (table) table.scrollTop = 0
    }
  }
}
const { loadSession, submitAuth, logout } = usePortalAuthentication({
  user,
  authLoading,
  authBusy,
  authError,
  authNotice,
  authForm,
  isRegister,
  navigate,
  notifySuccess: (message, title) => portalNotify.success(message, title),
  notifyError: (message, title) => portalNotify.error(message, title),
})
function onPopState() {
  const navigationId = ++navigationSequence
  setRouteFromLocation()
  closeMenu()
  authError.value = ''
  const continueNavigation = async () => {
    if (!await ensureRouteAccess(navigationId)) return
    if (navigationId !== navigationSequence) return
    if (route.value === '/content-management') void loadContentList()
    if (route.value === '/education/teaching-resources') void loadTeachingResources()
    loadRouteData()
    void restoreScrollPosition()
  }
  void continueNavigation()
}
let disposeNotificationHost: (() => void) | null = null
onMounted(() => {
  disposeNotificationHost = mountNotificationHost(document.body)
  const navigationId = ++navigationSequence
  setRouteFromLocation()
  void nextTick().then(observeVirtualTableWidth)
  window.addEventListener('popstate', onPopState)
  document.addEventListener('scroll', onScrollCapture, true)
  window.addEventListener('beforeunload', saveScrollPosition)
  void loadPublishedLegalDocuments()
  if (isProtectedPage.value) {
    void ensureRouteAccess(navigationId).then((allowed) => {
      if (!allowed || navigationId !== navigationSequence) return
      if (route.value === '/content-management') void loadContentList()
      if (route.value === '/education/teaching-resources') void loadTeachingResources()
      loadRouteData()
      void restoreScrollPosition()
    })
  } else {
    loadRouteData()
    // Public pages still load the session so the header reflects an existing login.
    void loadSession()
    void restoreScrollPosition()
  }
})
onBeforeUnmount(() => {
  disposeNotificationHost?.()
  disposeNotificationHost = null
  disposeTeachingResourceDrawer?.()
  disposeTeachingResourceDrawer = null
  if (selectedEducationResource.value) document.body.style.overflow = educationResourcePreviousBodyOverflow
  educationResourcePreviousBodyOverflow = ''
  saveScrollPosition()
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
        <a v-else class="btn btn-outline-secondary nav-auth" href="/login" @click.prevent="navigate('/login')">登录</a>
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
        <p v-if="isRegister" class="auth-switch">已经有账户？ <a href="/login" @click.prevent="navigate('/login')">立即登录</a></p>
        <p v-else class="auth-switch">还没有账户？ <a href="/register" @click.prevent="navigate('/register')">立即注册</a></p>
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

    <main v-else-if="legalSlug" class="legal-document-page">
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
        <template v-else-if="activeAccountSection === 'system-execution-policy'">
          <SystemOpsCenter view="policy" />
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
                <div v-for="(record, index) in visibleTableRecords" :key="String(record.id ?? (virtualTableStart + index))" class="virtual-table-row" role="row" :class="{ 'resource-clickable-row': activeAccountSection === 'teaching-resources' }" :tabindex="activeAccountSection === 'teaching-resources' ? 0 : undefined" @click="activeAccountSection === 'teaching-resources' && openResourceDetails(String(record.id))" @keydown.enter="activeAccountSection === 'teaching-resources' && openResourceDetails(String(record.id))">
                  <template v-for="column in activeTableDefinition.columns" :key="column.key">
                    <div v-if="column.key === 'actions' && (activeAccountSection === 'content-management' || activeAccountSection === 'teaching-resources')" class="content-row-actions" role="cell" @click.stop>
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
                    <span v-else role="cell"><template v-if="activeAccountSection === 'teaching-resources'">{{ tableCellValue(record, column.key) }}</template><button v-else type="button" class="table-cell-preview" :title="Array.from(tableCellValue(record, column.key)).length > 10 ? '点击查看完整内容' : undefined" @click="openTableCell(column.label, tableCellValue(record, column.key))">{{ tableCellPreview(tableCellValue(record, column.key)) }}</button></span>
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

    <TeachingResourceEditorDialog
      v-if="resourceDialogOpen"
      :resource="resourceDialogItem"
      :resource-type-options="resourceTypeOptions"
      :resource-grade-options="resourceGradeOptions"
      :busy="resourceBusy"
      @close="resourceDialogOpen = false"
      @update:busy="resourceBusy = $event"
      @saved="onResourceDialogSaved"
      @completed="onResourceDialogCompleted"
    />

    <div v-if="selectedTableCell" class="table-cell-detail-backdrop" @click.self="selectedTableCell = null" @keydown.esc="selectedTableCell = null">
      <section class="table-cell-detail" role="dialog" aria-modal="true" :aria-label="selectedTableCell.label">
        <div class="table-cell-detail-heading"><h2>{{ selectedTableCell.label }}</h2><button type="button" aria-label="关闭" @click="selectedTableCell = null"><i class="bi bi-x-lg" aria-hidden="true"></i></button></div>
        <p>{{ selectedTableCell.value }}</p>
        <button type="button" class="btn content-primary-button" @click="selectedTableCell = null">关闭</button>
      </section>
    </div>

    <div ref="teachingResourceDrawerHost"></div>


    <footer v-if="!isAccountPage && !isContentEditorPage" class="site-footer"><a class="footer-brand" href="/" @click.prevent="navigate('/')">资源平台 <span>©</span></a><span>连接资源、用户与行业服务</span><nav v-if="publishedLegalDocuments.length" class="site-footer-policies" aria-label="网站政策"> <a v-for="doc in publishedLegalDocuments" :key="doc.slug" :href="`/legal/${doc.slug}`" @click.prevent="navigate(`/legal/${doc.slug}`)">{{ doc.title }}</a></nav></footer>
    <button type="button" class="scroll-to-top" aria-label="返回页面顶部" title="返回顶部" @click="scrollToTop"><i class="bi bi-arrow-up" aria-hidden="true"></i></button>
  </div>
</template>

<style src="./app.css"></style>

<style src="./theme.css"></style>
