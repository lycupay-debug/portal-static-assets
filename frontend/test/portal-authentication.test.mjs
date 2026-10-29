/**
 * Responsibility: verify client-side session sequencing and auth request behavior independently of App.vue rendering.
 * Dependencies: Node test runner, Vue refs/computed, and injected API/navigation/notification adapters.
 * Collaboration: protects the authentication composable's contract while Go remains the authoritative authz boundary.
 * Boundaries: tests request/state orchestration only; no real network, browser history, or production credentials.
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, ref } from 'vue'
import { usePortalAuthentication } from '../src/composables/usePortalAuthentication.ts'

function setup({ register = false, request = async () => ({ user: { id: 'user-1' } }), navigate = async () => {}, notifySuccess = () => {}, notifyError = () => {} } = {}) {
  const state = {
    user: ref(null),
    authLoading: ref(false),
    authBusy: ref(false),
    authError: ref('old error'),
    authNotice: ref('old notice'),
    authForm: ref({ displayName: ' Example User ', email: ' user@example.com ', password: 'secret' }),
  }
  const auth = usePortalAuthentication({
    ...state,
    isRegister: computed(() => typeof register === 'object' ? register.value : register),
    navigate,
    notifySuccess,
    notifyError,
    request,
  })
  return { ...state, ...auth }
}

function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

test('session loader commits the returned user and clears loading state', async () => {
  const expected = { id: 'session-user' }
  const auth = setup({ request: async path => { assert.equal(path, '/auth/me'); return { user: expected } } })
  assert.equal(await auth.loadSession(), true)
  assert.deepEqual(auth.user.value, expected)
  assert.equal(auth.authLoading.value, false)
})

test('session loader clears an invalid session without hiding the failure as success', async () => {
  const auth = setup({ request: async () => { throw new Error('offline') } })
  auth.user.value = { id: 'stale-user' }
  assert.equal(await auth.loadSession(), false)
  assert.equal(auth.user.value, null)
  assert.equal(auth.authLoading.value, false)
})

test('older session responses cannot overwrite a newer request or clear its loading state', async () => {
  const first = deferred()
  const second = deferred()
  let calls = 0
  const auth = setup({ request: () => (++calls === 1 ? first.promise : second.promise) })
  const older = auth.loadSession()
  const newer = auth.loadSession()
  first.resolve({ user: { id: 'older' } })
  assert.equal(await older, true)
  assert.equal(auth.user.value, null)
  assert.equal(auth.authLoading.value, true)
  second.resolve({ user: { id: 'newer' } })
  assert.equal(await newer, true)
  assert.deepEqual(auth.user.value, { id: 'newer' })
  assert.equal(auth.authLoading.value, false)
})

test('login trims email, clears the password after success, navigates, and notifies', async () => {
  const calls = []
  const notices = []
  const auth = setup({
    request: async (path, init) => {
      calls.push({ path, init })
      return { user: { id: 'logged-in' } }
    },
    navigate: async path => calls.push({ navigate: path }),
    notifySuccess: (...args) => notices.push(args),
  })
  await auth.submitAuth()
  assert.equal(calls[0].path, '/auth/login')
  assert.deepEqual(JSON.parse(calls[0].init.body), { email: 'user@example.com', password: 'secret' })
  assert.deepEqual(auth.user.value, { id: 'logged-in' })
  assert.equal(auth.authForm.value.password, '')
  assert.deepEqual(calls[1], { navigate: '/account' })
  assert.deepEqual(notices, [['登录成功。', '登录成功']])
  assert.equal(auth.authBusy.value, false)
  assert.equal(auth.authError.value, '')
  assert.equal(auth.authNotice.value, '')
})

test('registration preserves its success notice when navigation changes the current route', async () => {
  const calls = []
  const notices = []
  const registerMode = ref(true)
  const auth = setup({
    register: registerMode,
    request: async (path, init) => { calls.push({ path, init }); return { user: { id: 'new-user' } } },
    navigate: async path => { calls.push({ navigate: path }); registerMode.value = false },
    notifySuccess: (...args) => notices.push(args),
  })
  await auth.submitAuth()
  assert.equal(calls[0].path, '/auth/register')
  assert.deepEqual(JSON.parse(calls[0].init.body), { displayName: 'Example User', email: 'user@example.com', password: 'secret' })
  assert.deepEqual(notices, [['账户创建成功。', '注册成功']])
})

test('missing user response surfaces an error and always releases the busy flag', async () => {
  const auth = setup({ request: async () => ({ error: '账号不可用' }) })
  await auth.submitAuth()
  assert.equal(auth.authError.value, '账号不可用')
  assert.equal(auth.authBusy.value, false)
  assert.equal(auth.user.value, null)
})

test('logout clears local session and navigates after the API request settles', async () => {
  const calls = []
  const notices = []
  const auth = setup({
    request: async path => { calls.push(path); return {} },
    navigate: async path => calls.push(path),
    notifySuccess: (...args) => notices.push(args),
  })
  auth.user.value = { id: 'logged-in' }
  await auth.logout()
  assert.deepEqual(calls, ['/auth/logout', '/'])
  assert.equal(auth.user.value, null)
  assert.equal(auth.authBusy.value, false)
  assert.deepEqual(notices, [['已安全退出登录。', '退出登录成功']])
})


test('failed logout reports an error instead of displaying a false success notification', async () => {
  const calls = []
  const notices = []
  const errors = []
  const auth = setup({
    request: async path => { calls.push(path); throw new Error('服务器暂不可用') },
    navigate: async path => calls.push(path),
    notifySuccess: (...args) => notices.push(args),
    notifyError: (...args) => errors.push(args),
  })
  auth.user.value = { id: 'logged-in' }
  await auth.logout()
  assert.deepEqual(calls, ['/auth/logout', '/'])
  assert.equal(auth.user.value, null)
  assert.equal(auth.authBusy.value, false)
  assert.deepEqual(notices, [])
  assert.deepEqual(errors, [['服务器暂不可用', '退出登录失败']])
})
