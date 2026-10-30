/**
 * Responsibility: compose portal route state, API workflows and native DOM views for one application root.
 * Dependencies: framework-independent state/controllers and browser APIs; no framework runtime, SFC or template compiler.
 * Collaboration: native page views own their DOM; Go API owns authorization and business rules; this module owns
 * application-wide state coordination, route gates, lazy data loading and mount/dispose ordering.
 * Boundaries: API contracts remain generated from OpenAPI; no production configuration or service lifecycle changes.
 */
import { computed, createWatchScope, nextTick, ref, watch } from './state/reactivity'
import { apiRequest } from './api/client'
import { createKnowledgeDirectoryController } from './state/knowledgeDirectory'
import { createDebouncedValue } from './state/debouncedValue'
import { mountNotificationHost } from './components/notificationHost'
import { mountTeachingResourceDrawer } from './components/teachingResourceDrawer'
import { portalNotify } from './utils/notification'
import { taxonomyChildOptions } from './utils/taxonomy'
import { knowledgeTagOptions, taxonomyParentNames as knowledgeTaxonomyParentNames } from './utils/knowledgeTags'
import { createKnowledgeTagSelection } from './state/knowledgeTagSelection'
import type { MutableValue, ReadonlyValue } from './state/valueContracts'
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
import { mountSiteHeaderView } from './views/siteHeaderView'
import { mountSiteFooterView } from './views/siteFooterView'
import { mountKnowledgeTagDialogView } from './views/knowledgeTagDialogView'
import { mountContentSettingsView } from './views/contentSettingsView'
import { mountContentWorkspaceTopbarView } from './views/contentWorkspaceTopbarView'
import { mountPortalRootView, type PortalRootHostName } from './views/portalRootView'
import { contentStatusLabels, contentTypeLabels } from './config/accountTables'
import { homeSections } from './config/homeSections'
import { knowledgeCategoryMenus } from './config/knowledgeCategories'
import { knowledgeFuzzyMatch } from './utils/knowledgeSearch'
import type { Content as APIContent, ContentVersion as APIContentVersion, TeachingResource as APITeachingResource } from './api/generated'

export function mountPortalApplication(root: HTMLElement): () => void {
  const watchScope = createWatchScope()
  return watchScope.run(() => {
const createStateValue = <T>(initial: T): MutableValue<T> => ref(initial)

const taxonomyParentNames = knowledgeTaxonomyParentNames

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

let portalRootView: ReturnType<typeof mountPortalRootView> | null = null
const menuOpen = ref(false)
const route = ref(window.location.pathname || '/')
const siteHeaderHost = ref<HTMLElement | null>(null)
const siteFooterHost = ref<HTMLElement | null>(null)
const knowledgeTagDialogHost = ref<HTMLElement | null>(null)
let knowledgeTagDialogView: ReturnType<typeof mountKnowledgeTagDialogView> | null = null
let siteHeaderView: { update: (state: { menuOpen: boolean; loggedIn: boolean }) => void; dispose: () => void } | null = null
let siteFooterView: { update: (documents: typeof publishedLegalDocuments.value) => void; dispose: () => void } | null = null
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
  const view = section === 'system-warnings' ? 'warnings' : section === 'system-logs' ? 'logs' : section === 'system-permissions' ? 'permissions' : null
  if (!view) return
  void (async () => {
    const module = await import('./components/systemOpsCenter')
    await import('./components/systemOpsCenter.css')
    if (view === 'permissions') await import('./components/permissionManagement.css')
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
const tableQueryDebounce = createDebouncedValue(tableQuery, 150, { createValue: createStateValue, subscribe: (source, listener) => watch(source as never, listener) })
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
const contentWorkspaceTopbarHost = ref<HTMLElement | null>(null)
let contentWorkspaceTopbarView: ReturnType<typeof mountContentWorkspaceTopbarView> | null = null
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
watch([contentWorkspaceTopbarHost, contentForm, contentBusy], () => {
  const host = contentWorkspaceTopbarHost.value
  if (!host) { contentWorkspaceTopbarView?.dispose(); contentWorkspaceTopbarView = null; return }
  const getState = () => ({ title: contentForm.value.title, busy: contentBusy.value })
  if (!contentWorkspaceTopbarView) contentWorkspaceTopbarView = mountContentWorkspaceTopbarView(host, { getState, returnToList: returnToContentList, cancel: cancelContentEditor })
  contentWorkspaceTopbarView.update(getState())
}, { deep: true, flush: 'post' })
const contentSettingsHost = ref<HTMLElement | null>(null)
let contentSettingsView: ReturnType<typeof mountContentSettingsView> | null = null
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
  createValue: createStateValue,
  afterRender: () => nextTick(),
})
const selectedKnowledgeTagCount = computed(() => selectedKnowledgeTagCountValue.value)
const knowledgeTagDraftCount = computed(() => knowledgeTagDraftCountValue.value)
watch([contentSettingsHost, contentForm, contentBusy, knowledgeTagDialogOpen], () => {
  const host = contentSettingsHost.value
  if (!host) { contentSettingsView?.dispose(); contentSettingsView = null; return }
  const getState = () => ({
    title: contentForm.value.title, slug: contentForm.value.slug, summary: contentForm.value.summary,
    visibility: contentForm.value.visibility, tags: contentForm.value.tags, busy: contentBusy.value,
    tagDialogOpen: knowledgeTagDialogOpen.value,
  })
  if (!contentSettingsView) contentSettingsView = mountContentSettingsView(host, {
    getState,
    visibilityOptions: [{ label: '仅自己可见', value: 'private' }, { label: '公开', value: 'public' }],
    taxonomyParentNames,
    setField: (field, value) => { contentForm.value[field] = value },
    setVisibility: value => { contentForm.value.visibility = value as ContentForm['visibility'] },
    openTagPicker: openKnowledgeTagDialog,
    setTagTrigger: element => { knowledgeTagTrigger.value = element },
    removeTag: removeSelectedKnowledgeTag,
    clearTags: clearSelectedKnowledgeTags,
    cancel: cancelContentEditor,
    save: () => { void saveContent() },
  })
  contentSettingsView.update(getState())
}, { deep: true, flush: 'post' })
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
watch([educationResourceCategory, educationResourceGrade, educationResourceSearch, filteredEducationResources, educationResourceLoading, educationResourceError, selectedEducationResource], updateEducationResourceDirectoryView, { deep: true })

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
const accountTableViewHost = ref<HTMLElement | null>(null)
let accountTableViewMountSequence = 0
let accountTableView: { update: (state: ReturnType<typeof getAccountTableViewState>) => void; dispose: () => void } | null = null
function getAccountTableViewState() {
  return {
    section: activeAccountSection.value,
    itemLabel: activeAccountItem.value?.label || '',
    itemDescription: activeAccountItem.value?.description || '',
    definition: activeTableDefinition.value,
    versionsContentId: versionsContentId.value,
    versionsContentTitle: versionsContentTitle.value,
    versions: contentVersions.value,
    contentBusy: contentBusy.value,
    contentLoading: contentLoading.value,
    resourceBusy: resourceBusy.value,
    records: filteredTableRecords.value,
    visibleRecords: visibleTableRecords.value,
    columnTemplate: tableColumnTemplate.value,
    topSpace: virtualTableTopSpace.value,
    bottomSpace: virtualTableBottomSpace.value,
    scrollTop: tableScrollTop.value,
    query: tableQuery.value,
    contentViewMode: contentViewMode.value,
    resourceViewMode: resourceViewMode.value,
    selectedCell: selectedTableCell.value,
  }
}
function updateAccountTableView() {
  const sequence = ++accountTableViewMountSequence
  const host = accountTableViewHost.value
  if (!host) { accountTableView?.dispose(); accountTableView = null; return }
  const state = getAccountTableViewState()
  if (!accountTableView) {
    void import('./views/accountTableView').then(module => {
      if (sequence !== accountTableViewMountSequence || host !== accountTableViewHost.value) return
      accountTableView = module.mountAccountTableView(host, {
        getState: getAccountTableViewState,
        setQuery: value => { tableQuery.value = value; tableScrollTop.value = 0 },
        setScrollTop: value => { tableScrollTop.value = value },
        setScrollElement: element => { virtualTableScrollElement.value = element; observeVirtualTableWidth() },
        createContent: openNewContent,
        switchContentView,
        createResource: openResourceDialog,
        switchResourceView,
        resourceHasUrl: id => Boolean(teachingResources.value.find(resource => resource.id === id)?.url),
        openResource: openResourceDetails,
        editResource: openResourceEditById,
        downloadResource: downloadTeachingResource,
        changeResource: changeTeachingResource,
        editContent,
        openVersions: openContentVersions,
        changeContentStatus,
        deleteContent,
        restoreContent: restoreDeletedContent,
        permanentlyDeleteContent,
        restoreVersion: restoreContentVersion,
        backToContentList: () => { versionsContentId.value = null },
        openCell: openTableCell,
        closeCell: () => { selectedTableCell.value = null },
        cellValue: tableCellValue,
        cellPreview: tableCellPreview,
        contentStatusLabel: status => contentStatusLabels[status as keyof typeof contentStatusLabels] || status,
      })
    }).catch((error: unknown) => {
      if (sequence !== accountTableViewMountSequence || host !== accountTableViewHost.value) return
      host.textContent = '账户业务视图加载失败，请刷新页面后重试。'
      portalNotify.error(error instanceof Error ? error.message : '账户业务视图加载失败', '账户中心')
    })
    return
  }
  accountTableView.update(state)
}
watch([accountTableViewHost, activeAccountSection, activeAccountItem, activeTableDefinition, versionsContentId, versionsContentTitle, contentVersions, contentBusy, contentLoading, resourceBusy, filteredTableRecords, visibleTableRecords, virtualTableTopSpace, virtualTableBottomSpace, tableQuery, contentViewMode, resourceViewMode, selectedTableCell], updateAccountTableView, { deep: true, flush: 'post' })
watch(tableColumnTemplate, value => { virtualTableScrollElement.value?.style.setProperty('--table-columns', value) })
const isRegister = computed(() => route.value === '/register')
const isAuthPage = computed(() => route.value === '/login' || route.value === '/register')
const isPublicContentPage = computed(() => /^\/content\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(route.value))
const legalDocumentsController = createLegalDocumentsController(route, { createValue: createStateValue })
const legalSlug = computed(() => legalDocumentsController.legalSlug.value)
const publishedLegalDocuments = legalDocumentsController.publishedLegalDocuments as ReadonlyValue<typeof legalDocumentsController.publishedLegalDocuments.value>
const legalDocument = legalDocumentsController.legalDocument as ReadonlyValue<typeof legalDocumentsController.legalDocument.value>
const legalDocumentHtml = legalDocumentsController.legalDocumentHtml as ReadonlyValue<string>
const legalDocumentLoading = legalDocumentsController.legalDocumentLoading as ReadonlyValue<boolean>
const legalDocumentError = legalDocumentsController.legalDocumentError as ReadonlyValue<string>
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
  createValue: createStateValue,
  browser: window,
  document,
})
const knowledgeVisibleCount = knowledgeDirectory.knowledgeVisibleCount as MutableValue<number>
watch(knowledgeVisibleCount, updateEducationResourceDirectoryView)
const knowledgeCategory = knowledgeDirectory.knowledgeCategory as MutableValue<string>
const knowledgeSubcategory = knowledgeDirectory.knowledgeSubcategory as MutableValue<string>
const knowledgeSearchQuery = knowledgeDirectory.knowledgeSearchQuery as MutableValue<string>
const highlightedKnowledgeSuggestion = knowledgeDirectory.highlightedKnowledgeSuggestion as MutableValue<number>
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
watch([knowledgeDirectoryHost, visibleDirectoryContentItems], ensureKnowledgeListFilled, { flush: 'post' })

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
const knowledgeSearchDebounce = createDebouncedValue(knowledgeSearchQuery, 220, { createValue: createStateValue, subscribe: (source, listener) => watch(source as never, listener) })
const debouncedKnowledgeSearchQuery = knowledgeSearchDebounce.value
const educationResourceSearchDebounce = createDebouncedValue(educationResourceSearch, 220, { createValue: createStateValue, subscribe: (source, listener) => watch(source as never, listener) })
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
function setPortalRootHost(name: PortalRootHostName, element: HTMLElement | null) {
  switch (name) {
    case 'knowledgeTagDialogHost': knowledgeTagDialogHost.value = element; break
    case 'siteHeaderHost': siteHeaderHost.value = element; break
    case 'siteFooterHost': siteFooterHost.value = element; break
    case 'contentWorkspaceTopbarHost': contentWorkspaceTopbarHost.value = element; break
    case 'contentEditorHost': contentEditorHost.value = element; break
    case 'contentSettingsHost': contentSettingsHost.value = element; break
    case 'homePageHost': homePageHost.value = element; break
    case 'knowledgeDirectoryHost': knowledgeDirectoryHost.value = element; break
    case 'directoryOverviewHost': directoryOverviewHost.value = element; break
    case 'educationResourceDirectoryHost': educationResourceDirectoryHost.value = element; break
    case 'authPageHost': authPageHost.value = element; break
    case 'publicArticleHost': publicArticleHost.value = element; break
    case 'legalPageHost': legalPageHost.value = element; break
    case 'accountSidebarHost': accountSidebarHost.value = element; break
    case 'accountIdentityHost': accountIdentityHost.value = element; break
    case 'complianceCenterHost': complianceCenterHost.value = element; break
    case 'systemOpsHost': systemOpsHost.value = element; break
    case 'accountTableViewHost': accountTableViewHost.value = element; break
    case 'teachingResourceEditorHost': teachingResourceEditorHost.value = element; break
    case 'teachingResourceDrawerHost': teachingResourceDrawerHost.value = element; break
  }
}
function getPortalRootState() {
  return {
    route: route.value,
    contentEditorPage: isContentEditorPage.value,
    accountPage: isAccountPage.value,
    authPage: isAuthPage.value,
    publicContentPage: isPublicContentPage.value,
    legalPage: Boolean(legalSlug.value),
    accountSection: activeAccountSection.value,
    accountTableMode: Boolean(activeTableDefinition.value),
  }
}
watch([route, activeAccountSection, activeTableDefinition], () => {
  portalRootView?.update(getPortalRootState())
}, { flush: 'post' })
watch([siteHeaderHost, menuOpen, user], () => {
  const host = siteHeaderHost.value
  if (!host) { siteHeaderView?.dispose(); siteHeaderView = null; return }
  const state = { menuOpen: menuOpen.value, loggedIn: Boolean(user.value) }
  if (!siteHeaderView) siteHeaderView = mountSiteHeaderView(host, { getState: () => ({ menuOpen: menuOpen.value, loggedIn: Boolean(user.value) }), navigate, setMenuOpen: value => { menuOpen.value = value } })
  else siteHeaderView.update(state)
}, { deep: true, flush: 'post' })
watch([siteFooterHost, publishedLegalDocuments], () => {
  const host = siteFooterHost.value
  if (!host) { siteFooterView?.dispose(); siteFooterView = null; return }
  if (!siteFooterView) siteFooterView = mountSiteFooterView(host, { navigate })
  siteFooterView.update(publishedLegalDocuments.value)
}, { deep: true, flush: 'post' })
watch([knowledgeTagDialogHost, knowledgeTagDialogOpen, knowledgeTagDraft, expandedKnowledgeTagGroups, contentBusy], () => {
  const host = knowledgeTagDialogHost.value
  if (!host) { knowledgeTagDialogView?.dispose(); knowledgeTagDialogView = null; return }
  const state = {
    open: knowledgeTagDialogOpen.value,
    busy: contentBusy.value,
    categories: knowledgeTagOptions,
    expandedGroups: expandedKnowledgeTagGroups.value,
    selectedTags: knowledgeTagDraft.value,
    draftCount: knowledgeTagDraftCount.value,
  }
  if (!knowledgeTagDialogView) knowledgeTagDialogView = mountKnowledgeTagDialogView(host, {
    getState: () => ({
      open: knowledgeTagDialogOpen.value, busy: contentBusy.value, categories: knowledgeTagOptions,
      expandedGroups: expandedKnowledgeTagGroups.value, selectedTags: knowledgeTagDraft.value,
      draftCount: knowledgeTagDraftCount.value,
    }),
    close: closeKnowledgeTagDialog,
    toggleGroup: toggleKnowledgeTagGroup,
    toggleTag: toggleKnowledgeTag,
    clear: clearKnowledgeTags,
    setDialogElement: element => { knowledgeTagDialog.value = element },
  })
  knowledgeTagDialogView.update(state)
}, { deep: true, flush: 'post' })

const rootHost = document.createElement('div')
rootHost.id = 'portal-root-host'
root.replaceChildren(rootHost)
disposeNotificationHost = mountNotificationHost(document.body)
portalRootView = mountPortalRootView(rootHost, { getState: getPortalRootState, navigate, scrollToTop, setHost: setPortalRootHost })
void portalNavigation?.start()

return () => {
  watchScope.dispose()
    portalRootView?.dispose()
    portalRootView = null
    siteHeaderView?.dispose()
    siteHeaderView = null
    siteFooterView?.dispose()
    siteFooterView = null
    knowledgeTagDialogView?.dispose()
    knowledgeTagDialogView = null
    authPageMountSequence += 1
    homePageMountSequence += 1
    homePageView?.dispose()
    homePageView = null
    accountTableViewMountSequence += 1
    accountTableView?.dispose()
    accountTableView = null
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
    contentSettingsView?.dispose()
    contentSettingsView = null
    contentWorkspaceTopbarView?.dispose()
    contentWorkspaceTopbarView = null
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
  root.replaceChildren()
}
  })
}
