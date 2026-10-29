/**
 * 职责：按当前视口请求布局策略，并将允许的 CSS token 应用到根元素。
 * 依赖：浏览器 window/document/fetch；失败时由 CSS 默认 token 提供降级。
 * 协作：main.ts 在应用挂载前初始化；返回的 dispose 用于移除监听并使待处理请求失效。
 * 边界：只接受 version=1 的策略，且只写入 --layout- 前缀的字符串变量。
 */
interface ResponsiveLayoutPolicy {
  version: number
  viewport: { width: number; height: number }
  profile: string
  cssVariables: Record<string, string>
}

let resizeTimer: ReturnType<typeof setTimeout> | undefined
let activeRequest = 0

async function applyResponsivePolicy(): Promise<void> {
  const requestId = ++activeRequest
  const query = new URLSearchParams({
    width: String(window.innerWidth),
    height: String(window.innerHeight),
  })

  try {
    const response = await fetch(`/api/layout-policy?${query.toString()}`, {
      method: 'GET',
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) return
    const policy = await response.json() as ResponsiveLayoutPolicy
    if (requestId !== activeRequest || policy.version !== 1) return

    const root = document.documentElement
    root.dataset.layoutProfile = policy.profile
    for (const [name, value] of Object.entries(policy.cssVariables)) {
      if (name.startsWith('--layout-') && typeof value === 'string') {
        root.style.setProperty(name, value)
      }
    }
  } catch {
    // CSS media queries and default tokens remain the offline-safe fallback.
  }
}

function scheduleResponsivePolicy(): void {
  if (resizeTimer !== undefined) clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    resizeTimer = undefined
    void applyResponsivePolicy()
  }, 160)
}

/** Register responsive listeners and return cleanup for timers and in-flight requests. */
export function initializeResponsiveLayout(): () => void {
  void applyResponsivePolicy()
  window.addEventListener('resize', scheduleResponsivePolicy, { passive: true })
  window.addEventListener('orientationchange', scheduleResponsivePolicy, { passive: true })

  return () => {
    window.removeEventListener('resize', scheduleResponsivePolicy)
    window.removeEventListener('orientationchange', scheduleResponsivePolicy)
    if (resizeTimer !== undefined) clearTimeout(resizeTimer)
    resizeTimer = undefined
    // Invalidate any fetch/json response already in flight so it cannot mutate a disposed layout.
    activeRequest += 1
  }
}
