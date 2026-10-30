/**
 * Responsibility: own client-side session loading and login/register/logout request orchestration.
 * Dependencies: injected reactive auth state, same-origin API transport, route navigation, and notification callback.
 * Collaboration: portalApplication.ts owns page rendering and navigation policy; this composable updates auth state and asks
 * the caller to navigate after a successful login or after logout cleanup. Server-side authorization remains in Go.
 * Boundaries: no DOM access or history mutation; stale session responses are ignored, and passwords are cleared
 * only after a successful login/register response. API failures are surfaced through the supplied authError ref.
 */
import { apiRequest as defaultApiRequest } from '../api/client'
import type { LoginRequest, RegisterRequest, User as APIUser } from '../api/generated'
import type { MutableValue, ReadonlyValue } from '../state/valueContracts'

export type PortalUser = APIUser & { isAdmin?: boolean }

type AuthRequest = <T>(path: string, init?: RequestInit, fallbackMessage?: string) => Promise<T>
type AuthNotifier = (message: string, title?: string) => void

type PortalAuthenticationOptions = {
  user: MutableValue<PortalUser | null>
  authLoading: MutableValue<boolean>
  authBusy: MutableValue<boolean>
  authError: MutableValue<string>
  authNotice: MutableValue<string>
  authForm: MutableValue<{ displayName: string; email: string; password: string }>
  isRegister: ReadonlyValue<boolean>
  navigate: (target: string) => Promise<unknown> | unknown
  notifySuccess: AuthNotifier
  notifyError: AuthNotifier
  request?: AuthRequest
}

/**
 * Create auth actions around caller-owned refs so the session state remains visible to portalApplication.ts and tests.
 * The request sequence prevents an older `/auth/me` response from overwriting a newer navigation's session.
 */
export function createPortalAuthentication(options: PortalAuthenticationOptions) {
  const request = options.request ?? defaultApiRequest
  let sessionRequestSequence = 0

  async function loadSession(): Promise<boolean> {
    const requestId = ++sessionRequestSequence
    options.authLoading.value = true
    try {
      const result = await request<{ user: PortalUser }>('/auth/me')
      if (requestId === sessionRequestSequence) options.user.value = result.user
      return true
    } catch {
      if (requestId === sessionRequestSequence) options.user.value = null
      return false
    } finally {
      if (requestId === sessionRequestSequence) options.authLoading.value = false
    }
  }

  async function submitAuth(): Promise<void> {
    const registering = options.isRegister.value
    options.authBusy.value = true
    options.authError.value = ''
    options.authNotice.value = ''
    try {
      const endpoint = registering ? '/api/auth/register' : '/api/auth/login'
      const payload: RegisterRequest | LoginRequest = registering
        ? {
            displayName: options.authForm.value.displayName.trim(),
            email: options.authForm.value.email.trim(),
            password: options.authForm.value.password,
          }
        : {
            email: options.authForm.value.email.trim(),
            password: options.authForm.value.password,
          }
      const result = await request<{ user?: PortalUser; error?: string }>(endpoint.slice('/api'.length), {
        method: 'POST',
        body: JSON.stringify(payload),
      }, '操作失败，请稍后再试。')
      if (!result.user) throw new Error(result.error || '操作失败，请稍后再试。')
      options.user.value = result.user
      options.authForm.value.password = ''
      await options.navigate('/account')
      options.notifySuccess(registering ? '账户创建成功。' : '登录成功。', registering ? '注册成功' : '登录成功')
    } catch (error) {
      options.authError.value = error instanceof Error ? error.message : '网络连接失败，请稍后再试。'
    } finally {
      options.authBusy.value = false
    }
  }

  async function logout(): Promise<void> {
    options.authBusy.value = true
    let requestFailed = false
    let requestError: unknown
    try {
      await request<unknown>('/auth/logout', { method: 'POST' })
    } catch (error) {
      requestFailed = true
      requestError = error
    } finally {
      options.user.value = null
      options.authBusy.value = false
      await options.navigate('/')
    }

    if (requestFailed) {
      options.notifyError(requestError instanceof Error ? requestError.message : '退出登录失败。', '退出登录失败')
      return
    }
    options.notifySuccess('已安全退出登录。', '退出登录成功')
  }

  return { loadSession, submitAuth, logout }
}
