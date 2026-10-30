<!--
Responsibility: compose portal routes, page-level reactive state, and the user-facing application template.
Dependencies: focused composables for URL resolution, authentication, data loading, editing, and table behavior.
Collaboration: App.vue coordinates route/session gates and view state; Go remains the authority for authentication and permissions.
Boundaries: avoid embedding reusable business workflows here; browser lifecycle and route sequencing remain explicit,
while extracted composables own independently testable state transitions and side effects.
-->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, onUpdated, ref, watch, type Ref } from 'vue'
import { apiRequest } from './api/client'
import { createKnowledgeDirectoryController } from './state/knowledgeDirectory'
import { createDebouncedValue } from './state/debouncedValue'
import { mountVirtualizedSelect, type VirtualizedSelectOption } from './components/virtualizedSelect'
import { mountNotificationHost } from './components/notificationHost'
import { mountTeachingResourceDrawer } from './components/teachingResourceDrawer'
import { portalNotify } from './utils/notification'
import { taxonomyChildOptions } from './utils/taxonomy'
import { knowledgeTagOptions, taxonomyParentNames as knowledgeTaxonomyParentNames } from './utils/knowledgeTags'
import { createKnowledgeTagSelection } from './state/knowledgeTagSelection'
import type { MutableValue } from './state/valueContracts'
import { createScrollRestoration } from './state/scrollRestoration'
import { createPublicArticleActions } from './composables/usePublicArticleActions'
import { createLegalDocumentsController } from './state/legalDocuments'
import { filterEducationResources } from './utils/resourceFiltering'
import { tableCellPreview, tableCellValue } from './utils/accountTablePresentation'
import { createAccountTableModel } from './state/accountTableModel'
import { createPortalAuthentication, type PortalUser } from './composables/usePortalAuthentication'
import { createProtectedRouteGuard } from './composables/useProtectedRouteGuard'
import { createRetryableLazyLoader } from './utils/retryableLazyLoader'
import { accountGroups } from './navigation/accountNavigation'
import { routeSections, sectionRoutes } from './navigation/portalRoutes'
import { createPortalNavigationController } from './navigation/portalNavigationController'
import { directoryPages, directoryTiles } from './config/publicDirectories'
import { contentStatusLabels, contentTypeLabels } from './config/accountTables'
import { homeSections } from './config/homeSections'
import { knowledgeCategoryMenus } from './config/knowledgeCategories'
import { knowledgeFuzzyMatch } from './utils/knowledgeSearch'
import type { Content as APIContent, ContentVersion as APIContentVersion, TeachingResource as APITeachingResource } from './api/generated'

const createVueValue = <T>(initial: T): MutableValue<T> => ref(initial) as MutableValue<T>

const taxonomyParentNames = knowledgeTaxonomyParentNames
const visibilityOptions: VirtualizedSelectOption[] = [{ label: '仅自己可见', value: 'private' }, { label: '公开', value: 'public' }]

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
const complianceCenterHost = ref<HTMLElement | null>(null)
let disposeComplianceCenter: (() => void) | null = null
let complianceMountSequence = 0
watch([activeAccountSection, complianceCenterHost, () => user.value?.isAdmin === true], ([section, host, isAdmin]) => {
  const sequence = ++complianceMountSequence
  disposeComplianceCenter?.()
  disposeComplianceCenter = null
  if (section !== 'copyright-compliance' || !host) return
  void (async () => {
    const [module] = await Promise.all([
      import('./components/complianceCenterView'),
      import('./components/complianceCenter.css'),
    ])
    if (sequence !== complianceMountSequence || host !== complianceCenterHost.value) return
    disposeComplianceCenter = module.mountComplianceCenter(host, isAdmin === true).dispose
  })().catch((error: unknown) => {
    if (sequence !== complianceMountSequence || host !== complianceCenterHost.value) return
    host.textContent = '合规中心加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '合规中心加载失败', '合规中心')
  })
}, { flush: 'post' })
const systemOpsHost = ref<HTMLElement | null>(null)
let disposeSystemOpsCenter: (() => void) | null = null
let systemOpsMountSequence = 0
watch([activeAccountSection, systemOpsHost], ([section, host]) => {
  const sequence = ++systemOpsMountSequence
  disposeSystemOpsCenter?.()
  disposeSystemOpsCenter = null
  if (!host) return
  const view = section === 'system-warnings' ? 'warnings' : section === 'system-logs' ? 'logs' : section === 'system-execution-policy' ? 'policy' : null
  if (!view) return
  void (async () => {
    const module = await import('./components/systemOpsCenter')
    await import('./components/systemOpsCenter.css')
    if (view === 'policy') {
      await Promise.all([
        import('./components/permissionManagement.css'),
        import('./components/systemExecutionPolicyCenter.css'),
      ])
    }
    if (sequence !== systemOpsMountSequence || host !== systemOpsHost.value) return
    disposeSystemOpsCenter = module.mountSystemOpsCenter(host, { view }).dispose
  })().catch((error: unknown) => {
    if (sequence !== systemOpsMountSequence || host !== systemOpsHost.value) return
    host.textContent = '系统运维界面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '系统运维界面加载失败', '系统运维')
  })
}, { flush: 'post' })
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
const accountIdentityHost = ref<HTMLElement | null>(null)
let accountIdentityMountSequence = 0
let accountIdentityView: { update: (state: { section: 'overview' | 'profile' | 'security'; loading: boolean; busy: boolean; user: typeof user.value }) => void; dispose: () => void } | null = null
function updateAccountIdentityView() {
  const sequence = ++accountIdentityMountSequence
  const host = accountIdentityHost.value
  const section = activeAccountSection.value
  if (!host || (section !== 'overview' && section !== 'profile' && section !== 'security')) {
    accountIdentityView?.dispose(); accountIdentityView = null; return
  }
  const state: { section: 'overview' | 'profile' | 'security'; loading: boolean; busy: boolean; user: typeof user.value } = { section, loading: authLoading.value, busy: authBusy.value, user: user.value }
  if (!accountIdentityView) {
    void import('./views/accountIdentityView').then(module => {
      if (sequence !== accountIdentityMountSequence || host !== accountIdentityHost.value) return
      accountIdentityView = module.mountAccountIdentityView(host, { getState: () => ({ section: activeAccountSection.value as 'overview' | 'profile' | 'security', loading: authLoading.value, busy: authBusy.value, user: user.value }), logout })
    }).catch((error: unknown) => {
      if (sequence !== accountIdentityMountSequence || host !== accountIdentityHost.value) return
      host.textContent = '账户信息加载失败，请刷新页面后重试。'
      portalNotify.error(error instanceof Error ? error.message : '账户信息加载失败', '账户中心')
    })
    return
  }
  accountIdentityView.update(state)
}
watch([accountIdentityHost, activeAccountSection, user, authLoading, authBusy], updateAccountIdentityView, { deep: true, flush: 'post' })
function toggleAccountGroup(groupId: string) {
  expandedAccountGroups.value = expandedAccountGroups.value.includes(groupId)
    ? expandedAccountGroups.value.filter((id) => id !== groupId)
    : [...expandedAccountGroups.value, groupId]
}
const accountSidebarHost = ref<HTMLElement | null>(null)
let accountSidebarView: { update: (state: { groups: typeof filteredAccountGroups.value; active: string; expanded: string[] }) => void; dispose: () => void } | null = null
let accountSidebarMountSequence = 0
function updateAccountSidebarView() {
  const sequence = ++accountSidebarMountSequence
  const host = accountSidebarHost.value
  if (!host) { accountSidebarView?.dispose(); accountSidebarView = null; return }
  const state = { groups: filteredAccountGroups.value, active: activeSidebarButton.value, expanded: expandedAccountGroups.value }
  if (!accountSidebarView) {
    void import('./views/accountSidebarView').then(module => {
      if (sequence !== accountSidebarMountSequence || host !== accountSidebarHost.value) return
      accountSidebarView = module.mountAccountSidebarView(host, { getState: () => ({ groups: filteredAccountGroups.value, active: activeSidebarButton.value, expanded: expandedAccountGroups.value }), toggleGroup: toggleAccountGroup, selectSection: selectAccountSection, navigate })
    }).catch((error: unknown) => {
      if (host !== accountSidebarHost.value) return
      host.textContent = '账户导航加载失败，请刷新页面后重试。'
      portalNotify.error(error instanceof Error ? error.message : '账户导航加载失败', '账户中心')
    })
    return
  }
  accountSidebarView.update(state)
}
watch([accountSidebarHost, filteredAccountGroups, activeSidebarButton, expandedAccountGroups], updateAccountSidebarView, { deep: true, flush: 'post' })



const tableQuery = ref('')
const tableQueryDebounce = createDebouncedValue(tableQuery, 150, { createValue: createVueValue, subscribe: (source, listener) => watch(source as never, listener) })
const debouncedTableQuery = tableQueryDebounce.value
watch(debouncedTableQuery as typeof tableQuery, (query) => {
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
const teachingResourceEditorHost = ref<HTMLElement | null>(null)
let disposeTeachingResourceEditor: (() => void) | null = null
let teachingResourceEditorSequence = 0
watch([resourceDialogOpen, resourceDialogItem, teachingResourceEditorHost], ([open, resource, host]) => {
  const sequence = ++teachingResourceEditorSequence
  disposeTeachingResourceEditor?.()
  disposeTeachingResourceEditor = null
  if (!open || !host) return
  void import('./components/teachingResourceEditorDialog').then(module => {
    if (sequence !== teachingResourceEditorSequence || host !== teachingResourceEditorHost.value || !resourceDialogOpen.value) return
    disposeTeachingResourceEditor = module.mountTeachingResourceEditorDialog(host, {
      resource,
      resourceTypeOptions,
      resourceGradeOptions,
      onClose: () => { resourceDialogOpen.value = false },
      onBusyChange: busy => { resourceBusy.value = busy },
      onSaved: onResourceDialogSaved,
      onCompleted: onResourceDialogCompleted,
    }).dispose
  }).catch((error: unknown) => {
    if (sequence !== teachingResourceEditorSequence || host !== teachingResourceEditorHost.value) return
    host.textContent = '教学资源编辑器加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '教学资源编辑器加载失败', '教学资源')
  })
}, { flush: 'post' })
watch(authError, (message) => { if (message) portalNotify.error(message, '登录或注册失败') }, { flush: 'sync' })
watch(authNotice, (message) => { if (message) portalNotify.info(message) }, { flush: 'sync' })
watch(contentError, (message) => { if (message) portalNotify.error(message, '内容操作失败') }, { flush: 'sync' })
watch(contentNotice, (message) => { if (message) portalNotify.success(message) }, { flush: 'sync' })
const emptyContentForm = (): ContentForm => ({
  title: '', slug: '', contentType: 'article', tags: [], summary: '', body: '', visibility: 'private',
})
const contentForm = ref<ContentForm>(emptyContentForm())
const contentEditorHost = ref<HTMLElement | null>(null)
let nativeContentEditor: { setMarkdown: (markdown: string) => void; setEditable: (editable: boolean) => void; dispose: () => void } | null = null
let contentEditorMountSequence = 0
watch(contentEditorHost, host => {
  const sequence = ++contentEditorMountSequence
  nativeContentEditor?.dispose()
  nativeContentEditor = null
  if (!host) return
  void import('./components/nativeContentEditorView').then(module => {
    if (sequence !== contentEditorMountSequence || host !== contentEditorHost.value) return
    nativeContentEditor = module.mountNativeContentEditorView({
      host,
      markdown: contentForm.value.body,
      editable: !contentBusy.value,
      onMarkdownChange: markdown => { contentForm.value.body = markdown },
    })
  }).catch((error: unknown) => {
    if (sequence !== contentEditorMountSequence || host !== contentEditorHost.value) return
    host.textContent = '内容编辑器加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '内容编辑器加载失败', '内容编辑器')
  })
}, { flush: 'post' })
watch(() => contentForm.value.body, markdown => nativeContentEditor?.setMarkdown(markdown))
watch(contentBusy, busy => nativeContentEditor?.setEditable(!busy))
const contentVisibilitySelectHost = ref<HTMLElement | null>(null)
let disposeContentVisibilitySelect: (() => void) | null = null
let contentVisibilitySelect: ReturnType<typeof mountVirtualizedSelect> | null = null
watch(contentVisibilitySelectHost, (host) => {
  disposeContentVisibilitySelect?.()
  disposeContentVisibilitySelect = null
  contentVisibilitySelect = null
  if (!host) return
  contentVisibilitySelect = mountVirtualizedSelect(host, {
    label: '可见范围',
    options: visibilityOptions,
    value: contentForm.value.visibility,
    disabled: contentBusy.value,
    onChange: (value) => { contentForm.value.visibility = value as ContentForm['visibility'] },
  })
  disposeContentVisibilitySelect = contentVisibilitySelect.dispose
}, { flush: 'post' })
watch(() => contentForm.value.visibility, value => contentVisibilitySelect?.update({ value }))
watch(contentBusy, disabled => contentVisibilitySelect?.update({ disabled }))
const {
  dialogOpen: knowledgeTagDialogOpen,
  trigger: knowledgeTagTrigger,
  dialog: knowledgeTagDialog,
  draft: knowledgeTagDraft,
  expandedGroups: expandedKnowledgeTagGroups,
  selectedCount: selectedKnowledgeTagCountValue,
  draftCount: knowledgeTagDraftCountValue,
  isSelected: isKnowledgeTagSelected,
  open: openKnowledgeTagDialog,
  close: closeKnowledgeTagDialog,
  toggleGroup: toggleKnowledgeTagGroup,
  toggleTag: toggleKnowledgeTag,
  clearDraft: clearKnowledgeTags,
  removeSelected: removeSelectedKnowledgeTag,
  clearSelected: clearSelectedKnowledgeTags,
} = createKnowledgeTagSelection(contentForm, contentBusy, {
  createValue: createVueValue,
  afterRender: () => nextTick(),
})
const selectedKnowledgeTagCount = computed(() => selectedKnowledgeTagCountValue.value)
const knowledgeTagDraftCount = computed(() => knowledgeTagDraftCountValue.value)
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
const educationResourceDirectoryHost = ref<HTMLElement | null>(null)
let educationResourceDirectoryMountSequence = 0
let educationResourceDirectoryView: { update: (state: { category: string; grade: string; search: string; types: string[]; grades: string[]; resources: typeof filteredEducationResources.value; visibleResources: typeof visibleEducationResources.value; loading: boolean; error: string; selected: typeof selectedEducationResource.value }) => void; dispose: () => void } | null = null
function getEducationResourceDirectoryState() {
  return {
    category: educationResourceCategory.value,
    grade: educationResourceGrade.value,
    search: educationResourceSearch.value,
    types: educationResourceTypes,
    grades: educationResourceGrades,
    resources: filteredEducationResources.value,
    visibleResources: visibleEducationResources.value,
    loading: educationResourceLoading.value,
    error: educationResourceError.value,
    selected: selectedEducationResource.value,
  }
}
function updateEducationResourceDirectoryView() { educationResourceDirectoryView?.update(getEducationResourceDirectoryState()) }
watch([educationResourceDirectoryHost, route], ([host, path]) => {
  const sequence = ++educationResourceDirectoryMountSequence
  educationResourceDirectoryView?.dispose()
  educationResourceDirectoryView = null
  if (!host || path !== '/education-resources') return
  void import('./views/educationResourceDirectoryView').then(module => {
    if (sequence !== educationResourceDirectoryMountSequence || host !== educationResourceDirectoryHost.value) return
    educationResourceDirectoryView = module.mountEducationResourceDirectoryView(host, {
      getState: getEducationResourceDirectoryState,
      setCategory: value => { educationResourceCategory.value = value; knowledgeVisibleCount.value = knowledgePageSize },
      setGrade: value => { educationResourceGrade.value = value; knowledgeVisibleCount.value = knowledgePageSize },
      setSearch: value => { educationResourceSearch.value = value },
      resetVisible: () => { knowledgeVisibleCount.value = knowledgePageSize },
      selectResource: value => { selectedEducationResource.value = value },
      ensureListFilled: ensureKnowledgeListFilled,
    })
    updateEducationResourceDirectoryView()
  }).catch((error: unknown) => {
    if (sequence !== educationResourceDirectoryMountSequence || host !== educationResourceDirectoryHost.value) return
    host.textContent = '教育资源页面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '教育资源页面加载失败', '教育资源')
  })
}, { flush: 'post' })
watch([educationResourceCategory, educationResourceGrade, educationResourceSearch, filteredEducationResources, visibleEducationResources, educationResourceLoading, educationResourceError, selectedEducationResource], updateEducationResourceDirectoryView, { deep: true })

const publicArticle = ref<PublicContent | null>(null)
const publicArticleHtml = ref('')
const publicContentLoading = ref(false)
const publicContentError = ref('')
const publicInteractionBusy = ref(false)
const shareDialogOpen = ref(false)
const shareUrl = ref('')
watch(publicContentError, (message) => { if (message) portalNotify.error(message, '内容读取失败') }, { flush: 'sync' })
const accountTableModel = createAccountTableModel({
  activeAccountSection, contentViewMode, contentItems, contentTrashItems, resourceViewMode,
  teachingResources, teachingResourceTrash, debouncedTableQuery, tableScrollTop, virtualTableViewportWidth,
  screenWidth: () => window.innerWidth,
})
const activeTableDefinition = computed(() => accountTableModel.activeTableDefinition.value)
const filteredTableRecords = computed(() => accountTableModel.filteredTableRecords.value)
const virtualTableStart = computed(() => accountTableModel.virtualTableStart.value)
const visibleTableRecords = computed(() => accountTableModel.visibleTableRecords.value)
const tableColumnTemplate = computed(() => accountTableModel.tableColumnTemplate.value)
const virtualTableTopSpace = computed(() => accountTableModel.virtualTableTopSpace.value)
const virtualTableBottomSpace = computed(() => accountTableModel.virtualTableBottomSpace.value)
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
type ContentManagementActions = ReturnType<typeof import('./composables/useContentManagementActions').createContentManagementActions>
const getContentManagementActions = createRetryableLazyLoader<ContentManagementActions>(() =>
  import('./composables/useContentManagementActions').then(({ createContentManagementActions }) => createContentManagementActions({
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
const isRegister = computed(() => route.value === '/register')
const isAuthPage = computed(() => route.value === '/login' || route.value === '/register')
const isPublicContentPage = computed(() => /^\/content\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value))
const legalDocumentsController = createLegalDocumentsController(route, { createValue: createVueValue })
const legalSlug = computed(() => legalDocumentsController.legalSlug.value)
const publishedLegalDocuments = legalDocumentsController.publishedLegalDocuments as Ref<typeof legalDocumentsController.publishedLegalDocuments.value>
const legalDocument = legalDocumentsController.legalDocument as Ref<typeof legalDocumentsController.legalDocument.value>
const legalDocumentHtml = legalDocumentsController.legalDocumentHtml as Ref<string>
const legalDocumentLoading = legalDocumentsController.legalDocumentLoading as Ref<boolean>
const legalDocumentError = legalDocumentsController.legalDocumentError as Ref<string>
const { loadPublishedLegalDocuments, loadLegalDocument } = legalDocumentsController
const legalPageHost = ref<HTMLElement | null>(null)
let legalPageMountSequence = 0
let legalPageView: { update: (state: { document: typeof legalDocument.value; html: string; loading: boolean; error: string }) => void; dispose: () => void } | null = null
function updateLegalPageView() {
  legalPageView?.update({ document: legalDocument.value, html: legalDocumentHtml.value, loading: legalDocumentLoading.value, error: legalDocumentError.value })
}
watch([legalPageHost, legalSlug], ([host, slug]) => {
  const sequence = ++legalPageMountSequence
  legalPageView?.dispose()
  legalPageView = null
  if (!host || !slug) return
  void import('./views/legalDocumentView').then(module => {
    if (sequence !== legalPageMountSequence || host !== legalPageHost.value) return
    legalPageView = module.mountLegalDocumentView(host, navigate)
    updateLegalPageView()
  }).catch((error: unknown) => {
    if (sequence !== legalPageMountSequence || host !== legalPageHost.value) return
    host.textContent = '政策文件界面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '政策文件界面加载失败', '政策文件')
  })
}, { flush: 'post' })
watch([legalDocument, legalDocumentHtml, legalDocumentLoading, legalDocumentError], updateLegalPageView, { deep: true })
const publicArticleHost = ref<HTMLElement | null>(null)
let publicArticleMountSequence = 0
let publicArticleView: { update: (state: { article: typeof publicArticle.value; html: string; loading: boolean; error: string; busy: boolean; loggedIn: boolean; shareOpen: boolean; shareUrl: string }) => void; dispose: () => void } | null = null
function updatePublicArticleView() {
  publicArticleView?.update({ article: publicArticle.value, html: publicArticleHtml.value, loading: publicContentLoading.value, error: publicContentError.value, busy: publicInteractionBusy.value, loggedIn: Boolean(user.value), shareOpen: shareDialogOpen.value, shareUrl: shareUrl.value })
}
watch([publicArticleHost, route], ([host, path]) => {
  const sequence = ++publicArticleMountSequence
  publicArticleView?.dispose()
  publicArticleView = null
  if (!host || !/^\/content\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(path)) return
  void import('./views/publicArticleView').then(module => {
    if (sequence !== publicArticleMountSequence || host !== publicArticleHost.value) return
    publicArticleView = module.mountPublicArticleView(host, {
      contentTypeLabels,
      toggleInteraction: togglePublicInteraction,
      shareArticle: sharePublicArticle,
      copyShareUrl,
      closeShare: () => { shareDialogOpen.value = false },
      navigate,
    })
    updatePublicArticleView()
  }).catch((error: unknown) => {
    if (sequence !== publicArticleMountSequence || host !== publicArticleHost.value) return
    host.textContent = '文章界面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '文章界面加载失败', '公开内容')
  })
}, { flush: 'post' })
watch([publicArticle, publicArticleHtml, publicContentLoading, publicContentError, publicInteractionBusy, user, shareDialogOpen, shareUrl], updatePublicArticleView, { deep: true })
const homePageHost = ref<HTMLElement | null>(null)
let homePageMountSequence = 0
let homePageReady: Promise<void> = Promise.resolve()
let homePageView: { dispose: () => void } | null = null
watch([homePageHost, route], ([host, path]) => {
  const sequence = ++homePageMountSequence
  homePageView?.dispose()
  homePageView = null
  if (!host || path !== '/') return
  homePageReady = import('./views/homePageView').then(module => {
    if (sequence !== homePageMountSequence || host !== homePageHost.value) return
    homePageView = module.mountHomePageView(host, homeSections, navigate)
  }).catch((error: unknown) => {
    if (sequence !== homePageMountSequence || host !== homePageHost.value) return
    host.textContent = '首页内容加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '首页内容加载失败', '首页')
  })
}, { flush: 'post' })
const isPublicDirectoryPage = computed(() => Object.prototype.hasOwnProperty.call(directoryPages, route.value))
const directoryContentItems = computed(() => {
  const contentType = route.value === '/knowledge' ? 'article' : route.value === '/education-resources' ? 'resource' : route.value === '/software-tools' ? 'tool' : null
  return contentType ? publicContents.value.filter((item) => item.contentType === contentType) : []
})
const directoryOverviewHost = ref<HTMLElement | null>(null)
let directoryOverviewMountSequence = 0
let directoryOverviewView: { update: (state: { items: typeof directoryContentItems.value; loading: boolean; error: string }) => void; dispose: () => void } | null = null
function updateDirectoryOverviewView() {
  directoryOverviewView?.update({ items: directoryContentItems.value, loading: publicContentLoading.value, error: publicContentError.value })
}
watch([directoryOverviewHost, route], ([host, path]) => {
  const sequence = ++directoryOverviewMountSequence
  directoryOverviewView?.dispose()
  directoryOverviewView = null
  if (!host || (path !== '/homestays' && path !== '/software-tools')) return
  void import('./views/publicDirectoryOverviewView').then(module => {
    if (sequence !== directoryOverviewMountSequence || host !== directoryOverviewHost.value) return
    directoryOverviewView = module.mountPublicDirectoryOverview(host, {
      page: directoryPages[path]!,
      tiles: directoryTiles[path] || [],
      contentTypeLabel: path === '/software-tools' ? contentTypeLabels.tool : '',
      navigate,
    })
    updateDirectoryOverviewView()
  }).catch((error: unknown) => {
    if (sequence !== directoryOverviewMountSequence || host !== directoryOverviewHost.value) return
    host.textContent = '目录页面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '目录页面加载失败', '资源目录')
  })
}, { flush: 'post' })
watch([directoryContentItems, publicContentLoading, publicContentError], updateDirectoryOverviewView)

const knowledgeDirectory = createKnowledgeDirectoryController(route, directoryContentItems, filteredEducationResources, navigate, {
  createValue: createVueValue,
  browser: window,
  document,
})
const knowledgeVisibleCount = knowledgeDirectory.knowledgeVisibleCount as Ref<number>
const knowledgeCategory = knowledgeDirectory.knowledgeCategory as Ref<string>
const knowledgeSubcategory = knowledgeDirectory.knowledgeSubcategory as Ref<string>
const knowledgeSearchQuery = knowledgeDirectory.knowledgeSearchQuery as Ref<string>
const highlightedKnowledgeSuggestion = knowledgeDirectory.highlightedKnowledgeSuggestion as Ref<number>
const {
  knowledgePageSize, handleKnowledgeAutocompleteSelect, onKnowledgeSearchKeydown,
  chooseKnowledgeCategory, onKnowledgePageScroll, ensureKnowledgeListFilled,
} = knowledgeDirectory
const knowledgeFilteredItems = computed(() => knowledgeDirectory.knowledgeFilteredItems.value)
const knowledgeSearchSuggestions = computed(() => knowledgeDirectory.knowledgeSearchSuggestions.value)
const visibleDirectoryContentItems = computed(() => knowledgeDirectory.visibleDirectoryContentItems.value)
const knowledgeDirectoryHost = ref<HTMLElement | null>(null)
let knowledgeDirectoryMountSequence = 0
let knowledgeDirectoryView: { update: (state: { category: string; subcategory: string; query: string; highlightedSuggestion: number; suggestions: typeof knowledgeSearchSuggestions.value; filteredItems: typeof knowledgeFilteredItems.value; visibleItems: typeof visibleDirectoryContentItems.value; loading: boolean; error: string }) => void; dispose: () => void } | null = null
function getKnowledgeDirectoryViewState() {
  return {
    category: knowledgeCategory.value,
    subcategory: knowledgeSubcategory.value,
    query: knowledgeSearchQuery.value,
    highlightedSuggestion: highlightedKnowledgeSuggestion.value,
    suggestions: knowledgeSearchSuggestions.value,
    filteredItems: knowledgeFilteredItems.value,
    visibleItems: visibleDirectoryContentItems.value,
    loading: publicContentLoading.value,
    error: publicContentError.value,
  }
}
function updateKnowledgeDirectoryView() { knowledgeDirectoryView?.update(getKnowledgeDirectoryViewState()) }
watch([knowledgeDirectoryHost, route], ([host, path]) => {
  const sequence = ++knowledgeDirectoryMountSequence
  knowledgeDirectoryView?.dispose()
  knowledgeDirectoryView = null
  if (!host || path !== '/knowledge') return
  void import('./views/knowledgeDirectoryView').then(module => {
    if (sequence !== knowledgeDirectoryMountSequence || host !== knowledgeDirectoryHost.value) return
    knowledgeDirectoryView = module.mountKnowledgeDirectoryView(host, {
      categories: knowledgeCategoryMenus,
      getState: getKnowledgeDirectoryViewState,
      chooseCategory: chooseKnowledgeCategory,
      setQuery: value => { knowledgeSearchQuery.value = value; highlightedKnowledgeSuggestion.value = -1 },
      setHighlightedSuggestion: value => { highlightedKnowledgeSuggestion.value = value },
      chooseSuggestion: handleKnowledgeAutocompleteSelect,
      onSearchKeydown: onKnowledgeSearchKeydown,
      setVisibleCount: () => { knowledgeVisibleCount.value = knowledgePageSize },
      ensureListFilled: ensureKnowledgeListFilled,
      navigate,
    })
    updateKnowledgeDirectoryView()
  }).catch((error: unknown) => {
    if (sequence !== knowledgeDirectoryMountSequence || host !== knowledgeDirectoryHost.value) return
    host.textContent = '知识广场加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '知识广场加载失败', '知识广场')
  })
}, { flush: 'post' })
watch([knowledgeCategory, knowledgeSubcategory, knowledgeSearchQuery, highlightedKnowledgeSuggestion, knowledgeFilteredItems, visibleDirectoryContentItems, publicContentLoading, publicContentError], updateKnowledgeDirectoryView, { deep: true })
onUpdated(ensureKnowledgeListFilled)

function closeMenu() { menuOpen.value = false }
const { saveScrollPosition, restoreScrollPosition } = createScrollRestoration(route, { browser: window, document, storage: sessionStorage, afterRender: () => nextTick() })
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
const knowledgeSearchDebounce = createDebouncedValue(knowledgeSearchQuery, 220, { createValue: createVueValue, subscribe: (source, listener) => watch(source as never, listener) })
const debouncedKnowledgeSearchQuery = knowledgeSearchDebounce.value
const educationResourceSearchDebounce = createDebouncedValue(educationResourceSearch, 220, { createValue: createVueValue, subscribe: (source, listener) => watch(source as never, listener) })
const debouncedEducationResourceSearchQuery = educationResourceSearchDebounce.value
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
watch(debouncedKnowledgeSearchQuery as typeof tableQuery, (query) => {
  if (route.value !== '/knowledge') return
  void loadPublicContents(query)
})
watch(debouncedEducationResourceSearchQuery as typeof tableQuery, (query) => {
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
const { toggleInteraction: togglePublicInteraction, shareArticle: sharePublicArticle, copyShareUrl } = createPublicArticleActions({
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

let portalNavigation: ReturnType<typeof createPortalNavigationController> | null = null
function navigate(target: string): Promise<void> {
  return portalNavigation?.navigate(target) ?? Promise.resolve()
}
const { loadSession, submitAuth, logout } = createPortalAuthentication({
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
const authPageHost = ref<HTMLElement | null>(null)
let disposeAuthPage: (() => void) | null = null
let authPageMountSequence = 0
watch([authPageHost, route], ([host, path]) => {
  const sequence = ++authPageMountSequence
  disposeAuthPage?.()
  disposeAuthPage = null
  if (!host || (path !== '/login' && path !== '/register')) return
  void import('./views/authPageView').then(module => {
    if (sequence !== authPageMountSequence || host !== authPageHost.value) return
    disposeAuthPage = module.mountAuthPage(host, { isRegister: path === '/register', authForm, authBusy, submitAuth, navigate }).dispose
  }).catch((error: unknown) => {
    if (sequence !== authPageMountSequence || host !== authPageHost.value) return
    host.textContent = '登录界面加载失败，请刷新页面后重试。'
    portalNotify.error(error instanceof Error ? error.message : '登录界面加载失败', '账户')
  })
}, { flush: 'post' })
portalNavigation = createPortalNavigationController({
  browser: window,
  document,
  routeSections,
  nextNavigationId: () => ++navigationSequence,
  isNavigationCurrent: navigationId => navigationId === navigationSequence,
  getRoute: () => route.value,
  setRoute: value => { route.value = value },
  isProtectedPage: () => isProtectedPage.value,
  isAccountPage: () => isAccountPage.value,
  setKnowledgeLocation: resolved => {
    knowledgeCategory.value = resolved.knowledgeCategory
    knowledgeSubcategory.value = resolved.knowledgeSubcategory
    knowledgeVisibleCount.value = knowledgePageSize
  },
  setAccountSection: section => { activeAccountSection.value = section },
  setActiveSidebarButton: section => { activeSidebarButton.value = section },
  ensureRouteAccess,
  saveScrollPosition,
  restoreScrollPosition,
  closeMenu,
  clearAuthError: () => { authError.value = '' },
  clearAuthNotice: () => { authNotice.value = '' },
  loadPublishedLegalDocuments,
  loadContentList: () => loadContentList(),
  loadTeachingResources: () => loadTeachingResources(),
  loadRouteData,
  loadSession,
  observeVirtualTableWidth,
  onScrollCapture,
  afterRender: async () => { await nextTick(); await homePageReady },
})
let disposeNotificationHost: (() => void) | null = null
onMounted(() => {
  disposeNotificationHost = mountNotificationHost(document.body)
  void portalNavigation?.start()
})
onBeforeUnmount(() => {
  authPageMountSequence += 1
  homePageMountSequence += 1
  homePageView?.dispose()
  homePageView = null
  accountSidebarMountSequence += 1
  accountSidebarView?.dispose()
  accountSidebarView = null
  accountIdentityMountSequence += 1
  accountIdentityView?.dispose()
  accountIdentityView = null
  educationResourceDirectoryMountSequence += 1
  educationResourceDirectoryView?.dispose()
  educationResourceDirectoryView = null
  knowledgeDirectoryMountSequence += 1
  knowledgeDirectoryView?.dispose()
  knowledgeDirectoryView = null
  directoryOverviewMountSequence += 1
  directoryOverviewView?.dispose()
  directoryOverviewView = null
  publicArticleMountSequence += 1
  publicArticleView?.dispose()
  publicArticleView = null
  legalPageMountSequence += 1
  legalPageView?.dispose()
  legalPageView = null
  disposeAuthPage?.()
  disposeAuthPage = null
  contentEditorMountSequence += 1
  nativeContentEditor?.dispose()
  nativeContentEditor = null
  teachingResourceEditorSequence += 1
  disposeTeachingResourceEditor?.()
  disposeTeachingResourceEditor = null
  complianceMountSequence += 1
  disposeComplianceCenter?.()
  disposeComplianceCenter = null
  systemOpsMountSequence += 1
  disposeSystemOpsCenter?.()
  disposeSystemOpsCenter = null
  disposeNotificationHost?.()
  disposeNotificationHost = null
  disposeContentVisibilitySelect?.()
  disposeContentVisibilitySelect = null
  contentVisibilitySelect = null
  disposeTeachingResourceDrawer?.()
  disposeTeachingResourceDrawer = null
  if (selectedEducationResource.value) document.body.style.overflow = educationResourcePreviousBodyOverflow
  educationResourcePreviousBodyOverflow = ''
  saveScrollPosition()
  portalNavigation?.dispose()
  portalNavigation = null
  tableResizeObserver?.disconnect()
  tableQueryDebounce.dispose()
  knowledgeSearchDebounce.dispose()
  educationResourceSearchDebounce.dispose()
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
      <div class="content-workspace-grid">
        <div ref="contentEditorHost" class="content-editor-native-main"></div>
        <aside class="content-tools-sidebar">
          <div class="workspace-sidebar-settings">
          <details class="workspace-fold workspace-settings-fold" open>
            <summary><span><strong>创建内容</strong></span><span class="workspace-fold-chevron bi bi-chevron-down" aria-hidden="true"></span></summary>
            <section class="workspace-properties">
              <label><span>内容标题</span><input class="form-control" v-model="contentForm.title" name="contentTitle" required maxlength="200" placeholder="输入内容标题" /></label>
              <fieldset class="workspace-tag-field"><legend>知识广场标签 <small>可多选</small></legend><button type="button" ref="knowledgeTagTrigger" class="workspace-tag-picker-trigger" :disabled="contentBusy" :aria-expanded="knowledgeTagDialogOpen" aria-haspopup="dialog" @click.stop="openKnowledgeTagDialog"><span class="bi bi-diagram-3" aria-hidden="true"></span><span>{{ selectedKnowledgeTagCount ? `已选择 ${selectedKnowledgeTagCount} 个标签` : '点击选择标签' }}</span><span class="bi bi-chevron-down" aria-hidden="true"></span></button><div v-if="selectedKnowledgeTagCount" class="workspace-tag-selected-preview"><span v-for="tag in contentForm.tags.filter(item => !taxonomyParentNames.has(item))" :key="tag" class="workspace-tag-selected-chip">{{ tag }}<button type="button" :disabled="contentBusy" :aria-label="`移除标签 ${tag}`" @click="removeSelectedKnowledgeTag(tag)">×</button></span><button type="button" class="workspace-tag-clear" :disabled="contentBusy" @click="clearSelectedKnowledgeTags">清空</button></div><p v-else class="workspace-tag-empty-hint">尚未选择标签</p></fieldset>
              <label><span>可见范围</span><div ref="contentVisibilitySelectHost"></div></label>
              <label><span>URL 别名</span><input class="form-control" v-model="contentForm.slug" name="contentSlug" required maxlength="100" pattern="[a-z0-9]+(-[a-z0-9]+)*" placeholder="例如：go-language-notes" /></label>
              <label><span>摘要</span><textarea class="form-control" v-model="contentForm.summary" name="contentSummary" maxlength="500" rows="3" placeholder="简要介绍这篇内容…"></textarea></label>

              <p class="workspace-privacy-note"><span>◈</span> 内容默认保存为草稿；保存后可在内容列表单独发布。</p>
            </section>
          </details>
          <div class="workspace-create-actions">
            <button type="button" class="btn content-secondary-button" :disabled="contentBusy" @click="cancelContentEditor">取消</button>
            <button type="button" class="btn btn-dark workspace-save-draft" :disabled="contentBusy" @click="saveContent()">{{ contentBusy ? '正在保存…' : '保存草稿' }}</button>
          </div>
            <p class="context-menu-hint">在正文中单击右键，打开统一编辑菜单。</p>
          </div>
        </aside>
      </div>
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

    <div v-else-if="route === '/'" ref="homePageHost"></div>

    <div v-else-if="route === '/knowledge'" ref="knowledgeDirectoryHost"></div>

    <div v-else-if="route === '/homestays' || route === '/software-tools'" ref="directoryOverviewHost"></div>

    <div v-else-if="route === '/education-resources'" ref="educationResourceDirectoryHost"></div>

    <main v-else-if="isAuthPage" class="route-main">
      <div ref="authPageHost"></div>
    </main>

    <main v-else-if="isPublicContentPage" class="public-content-page">
      <div ref="publicArticleHost"></div>
    </main>

    <main v-else-if="legalSlug" class="legal-document-page">
      <div ref="legalPageHost"></div>
    </main>

    <main v-else-if="isAccountPage" class="account-layout">

      <aside ref="accountSidebarHost" class="account-sidebar"></aside>

      <section class="account-content" :class="{ 'account-content-table-mode': !!activeTableDefinition }">
        <template v-if="activeAccountSection === 'overview' || activeAccountSection === 'profile' || activeAccountSection === 'security'">
          <div ref="accountIdentityHost"></div>
        </template>

        <template v-else-if="activeAccountSection === 'copyright-compliance'">
          <div ref="complianceCenterHost"></div>
        </template>
        <template v-else-if="activeAccountSection === 'system-warnings'">
          <div ref="systemOpsHost"></div>
        </template>
        <template v-else-if="activeAccountSection === 'system-logs'">
          <div ref="systemOpsHost"></div>
        </template>
        <template v-else-if="activeAccountSection === 'system-execution-policy'">
          <div ref="systemOpsHost"></div>
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

    <div v-if="resourceDialogOpen" ref="teachingResourceEditorHost"></div>

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
