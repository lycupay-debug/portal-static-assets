/** Native login/registration view tests; all authentication requests are injected and mocked. */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import { mountAuthPage } from '../src/views/authPageView'
import type { MutableValue } from '../src/state/valueContracts'
function cell<T>(value: T): MutableValue<T> { return { value } }

test('login view renders accessible fields and forwards form values to the auth workflow', async () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/login' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const authForm = cell({ displayName: '', email: '', password: '' })
  const busy = cell(false)
  const calls: string[] = []
  const view = mountAuthPage(host, { isRegister: false, authForm, authBusy: busy, submitAuth: async () => { calls.push('submit'); busy.value = true; await Promise.resolve(); busy.value = false; authForm.value.password = '' }, navigate: target => calls.push(target) })
  const email = host.querySelector<HTMLInputElement>('input[name="email"]')!
  const password = host.querySelector<HTMLInputElement>('input[name="password"]')!
  email.value = 'person@example.test'; password.value = 'secret-password'
  email.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  password.dispatchEvent(new dom.window.Event('input', { bubbles: true }))
  assert.equal(authForm.value.email, 'person@example.test')
  assert.equal(authForm.value.password, 'secret-password')
  host.querySelector('form')!.dispatchEvent(new dom.window.SubmitEvent('submit', { bubbles: true, cancelable: true }))
  await new Promise(resolve => setTimeout(resolve, 0))
  assert.deepEqual(calls, ['submit'])
  assert.equal(password.value, '')
  assert.equal(host.querySelector('input[name="displayName"]'), null)
  view.dispose()
  assert.equal(host.childElementCount, 0)
  dom.window.close()
})

test('registration view enforces registration-specific fields and navigation links', () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'https://portal.example.test/register' })
  const host = dom.window.document.querySelector<HTMLElement>('#host')!
  const calls: string[] = []
  const view = mountAuthPage(host, { isRegister: true, authForm: cell({ displayName: '', email: '', password: '' }), authBusy: cell(false), submitAuth: async () => undefined, navigate: target => calls.push(target) })
  const password = host.querySelector<HTMLInputElement>('input[name="password"]')!
  assert.equal(host.querySelector('input[name="displayName"]')?.maxLength, 60)
  assert.equal(password.minLength, 10)
  assert.equal(password.autocomplete, 'new-password')
  assert.match(host.textContent || '', /密码长度为 10–72 个字符/)
  host.querySelector<HTMLAnchorElement>('.auth-switch a')!.click()
  host.querySelector<HTMLAnchorElement>('.back-home')!.click()
  assert.deepEqual(calls, ['/login', '/'])
  view.dispose()
  dom.window.close()
})
