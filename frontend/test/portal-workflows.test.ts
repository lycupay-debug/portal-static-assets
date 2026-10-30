/**
 * Framework-independent workflow regression tests using structural value cells.
 * Dependencies: Node test APIs and API contracts; no Vue runtime or browser mount is required.
 * Collaboration: App.vue currently supplies Vue refs through the same value interface; native state cells can replace them.
 * Boundaries: requests are mocked; no credentials, network calls, database writes or production actions occur.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createPortalAuthentication } from '../src/composables/usePortalAuthentication'
import { createContentManagementActions } from '../src/composables/useContentManagementActions'
import type { MutableValue } from '../src/state/valueContracts'

function cell<T>(value: T): MutableValue<T> { return { value } }

const emptyForm = () => ({ title: '', slug: '', contentType: 'article' as const, tags: [], summary: '', body: '', visibility: 'private' as const })

test('authentication loads session into framework-neutral state cells', async () => {
  const user = cell<any>(null), authLoading = cell(false), authBusy = cell(false), authError = cell(''), authNotice = cell('')
  const authForm = cell({ displayName: '', email: 'user@example.test', password: 'secret-password' })
  const calls: string[] = []
  const auth = createPortalAuthentication({
    user, authLoading, authBusy, authError, authNotice, authForm,
    isRegister: cell(false), navigate: target => calls.push(`navigate:${target}`),
    notifySuccess: message => calls.push(`success:${message}`), notifyError: message => calls.push(`error:${message}`),
    request: async <T>(path: string) => { calls.push(path); return { user: { id: 'u1', email: 'user@example.test', displayName: 'User', createdAt: '2026-01-01T00:00:00Z' } } as T },
  })
  assert.equal(await auth.loadSession(), true)
  assert.equal(user.value.email, 'user@example.test')
  assert.equal(authLoading.value, false)
  assert.deepEqual(calls, ['/auth/me'])
})

test('login clears the password only after success and navigates to the account', async () => {
  const user = cell<any>(null), authLoading = cell(false), authBusy = cell(false), authError = cell(''), authNotice = cell('')
  const authForm = cell({ displayName: '', email: 'user@example.test', password: 'secret-password' })
  const calls: string[] = []
  const auth = createPortalAuthentication({
    user, authLoading, authBusy, authError, authNotice, authForm,
    isRegister: cell(false), navigate: target => calls.push(`navigate:${target}`),
    notifySuccess: message => calls.push(`success:${message}`), notifyError: message => calls.push(`error:${message}`),
    request: async <T>(path: string, init?: RequestInit) => { calls.push(`${path}:${init?.method || 'GET'}`); return { user: { id: 'u1', email: 'user@example.test', displayName: 'User', createdAt: '2026-01-01T00:00:00Z' } } as T },
  })
  await auth.submitAuth()
  assert.equal(user.value.id, 'u1')
  assert.equal(authForm.value.password, '')
  assert.equal(authBusy.value, false)
  assert.ok(calls.includes('/auth/login:POST'))
  assert.ok(calls.includes('navigate:/account'))
})

test('failed login preserves password and exposes the API error', async () => {
  const user = cell<any>(null), authLoading = cell(false), authBusy = cell(false), authError = cell(''), authNotice = cell('')
  const authForm = cell({ displayName: '', email: 'user@example.test', password: 'secret-password' })
  const auth = createPortalAuthentication({
    user, authLoading, authBusy, authError, authNotice, authForm,
    isRegister: cell(false), navigate: () => undefined, notifySuccess: () => undefined, notifyError: () => undefined,
    request: async <T>() => { throw new Error('invalid credentials') },
  })
  await auth.submitAuth()
  assert.equal(authError.value, 'invalid credentials')
  assert.equal(authForm.value.password, 'secret-password')
  assert.equal(authBusy.value, false)
})

test('content action validation rejects an empty body without issuing a request', async () => {
  const state = {
    routeContentId: cell<string | null>(null), contentItems: cell<any[]>([]), contentTrashItems: cell<any[]>([]),
    contentBusy: cell(false), contentError: cell(''), contentNotice: cell(''), contentEditorOpen: cell(false),
    editingContentId: cell<string | null>(null), contentForm: cell({ ...emptyForm(), title: 'Draft', slug: 'draft' }),
    contentVersions: cell<any[]>([]), versionsContentId: cell<string | null>(null), versionsContentTitle: cell(''),
  }
  let requestCount = 0
  const actions = createContentManagementActions({
    ...state, emptyContentForm: emptyForm, loadContentList: () => undefined, navigate: () => undefined,
    request: async <T>() => { requestCount += 1; return {} as T },
  })
  await actions.saveContent()
  assert.equal(state.contentError.value, '正文不能为空。')
  assert.equal(requestCount, 0)
})

test('content save sends a typed PATCH payload and navigates only after a successful response', async () => {
  const state = {
    routeContentId: cell<string | null>(null), contentItems: cell<any[]>([]), contentTrashItems: cell<any[]>([]),
    contentBusy: cell(false), contentError: cell(''), contentNotice: cell(''), contentEditorOpen: cell(true),
    editingContentId: cell<string | null>('content-1'), contentForm: cell({ ...emptyForm(), title: ' Draft ', slug: 'draft', tags: ['教育'], body: '# Body' }),
    contentVersions: cell<any[]>([]), versionsContentId: cell<string | null>(null), versionsContentTitle: cell(''),
  }
  const calls: Array<{ path: string; init?: RequestInit }> = []
  const actions = createContentManagementActions({
    ...state, emptyContentForm: emptyForm, loadContentList: () => undefined, navigate: target => calls.push({ path: `navigate:${target}` }),
    request: async <T>(path: string, init?: RequestInit) => { calls.push({ path, init }); return { item: { id: 'content-1' } } as T },
  })
  await actions.saveContent()
  assert.equal(calls[0].path, '/content/content-1')
  assert.equal(calls[0].init?.method, 'PATCH')
  assert.equal(JSON.parse(String(calls[0].init?.body)).title, 'Draft')
  assert.equal(calls[1].path, 'navigate:/content-management')
  assert.equal(state.contentBusy.value, false)
})
