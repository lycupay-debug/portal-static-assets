/**
 * Responsibility: render login/registration forms with native DOM APIs and forward authentication intent.
 * Dependencies: DOM host, caller-owned form/busy values, submit action and route navigation callback.
 * Collaboration: portalApplication.ts supplies the existing framework-neutral authentication workflow; Go remains the auth authority.
 * Boundaries: no HTML from users is interpolated; values are assigned through DOM properties, and listeners are disposed.
 */
import type { MutableValue, ReadonlyValue } from '../state/valueContracts'

type AuthForm = { displayName: string; email: string; password: string }

export function mountAuthPage(host: HTMLElement, options: {
  isRegister: boolean
  authForm: MutableValue<AuthForm>
  authBusy: ReadonlyValue<boolean>
  submitAuth: () => Promise<unknown>
  navigate: (target: string) => unknown
}) {
  const document = host.ownerDocument
  const panel = document.createElement('section')
  panel.className = 'auth-panel glass-panel'
  const eyebrow = document.createElement('p')
  eyebrow.className = 'eyebrow'
  eyebrow.innerHTML = '<span class="status-dot"></span> PLATFORM ACCOUNT'
  const heading = document.createElement('h1')
  heading.textContent = options.isRegister ? '创建你的账户' : '欢迎回来'
  const intro = document.createElement('p')
  intro.className = 'auth-intro'
  intro.textContent = options.isRegister ? '注册后即可进入个人中心。你的账户信息将安全保存。' : '登录后进入个人中心，管理你的账户信息。'
  const form = document.createElement('form')
  form.className = 'auth-form'

  const makeField = (labelText: string, name: string, type: string, autocomplete: string, placeholder: string, maxLength: number, minLength = 0) => {
    const label = document.createElement('label')
    label.className = 'field-label'
    label.append(document.createTextNode(labelText))
    const input = document.createElement('input')
    input.className = 'form-control'
    input.name = name
    input.type = type
    input.autocomplete = autocomplete as AutoFill
    input.maxLength = maxLength
    input.placeholder = placeholder
    input.required = name !== 'displayName'
    if (minLength) input.minLength = minLength
    label.append(input)
    form.append(label)
    return input
  }

  const displayName = options.isRegister ? makeField('昵称', 'displayName', 'text', 'nickname', '怎么称呼你', 60) : null
  const email = makeField('邮箱地址', 'email', 'email', 'email', 'name@example.com', 254)
  const password = makeField('密码', 'password', 'password', options.isRegister ? 'new-password' : 'current-password', options.isRegister ? '至少 10 个字符' : '输入你的密码', 72, options.isRegister ? 10 : 1)
  if (options.isRegister) {
    const help = document.createElement('p')
    help.className = 'field-help'
    help.textContent = '密码长度为 10–72 个字符。请勿重复使用其他网站的密码。'
    form.append(help)
  }
  const submit = document.createElement('button')
  submit.className = 'btn button button-primary auth-submit'
  submit.type = 'submit'
  form.append(submit)

  const switchText = document.createElement('p')
  switchText.className = 'auth-switch'
  const switchLink = document.createElement('a')
  switchLink.href = options.isRegister ? '/login' : '/register'
  switchLink.textContent = options.isRegister ? '立即登录' : '立即注册'
  switchText.append(document.createTextNode(options.isRegister ? '已经有账户？ ' : '还没有账户？ '), switchLink)
  const backHome = document.createElement('a')
  backHome.className = 'back-home'
  backHome.href = '/'
  backHome.textContent = '← 返回首页'
  panel.append(eyebrow, heading, intro, form, switchText, backHome)
  host.replaceChildren(panel)

  const renderBusy = () => {
    submit.disabled = options.authBusy.value
    submit.textContent = options.authBusy.value ? '正在处理…' : options.isRegister ? '创建账户' : '登录'
  }
  const onInput = () => {
    options.authForm.value = { displayName: displayName?.value || '', email: email.value, password: password.value }
  }
  const onSubmit = async (event: SubmitEvent) => {
    event.preventDefault()
    onInput()
    renderBusy()
    try { const pending = options.submitAuth(); renderBusy(); await pending }
    finally { renderBusy(); password.value = options.authForm.value.password }
  }
  const onNavigate = (event: MouseEvent) => {
    event.preventDefault()
    const target = (event.currentTarget as HTMLAnchorElement).getAttribute('href') || '/'
    options.navigate(target)
  }
  form.addEventListener('input', onInput)
  form.addEventListener('submit', onSubmit)
  switchLink.addEventListener('click', onNavigate)
  backHome.addEventListener('click', onNavigate)
  renderBusy()
  return {
    dispose() {
      form.removeEventListener('input', onInput)
      form.removeEventListener('submit', onSubmit)
      switchLink.removeEventListener('click', onNavigate)
      backHome.removeEventListener('click', onNavigate)
      host.replaceChildren()
    },
  }
}
