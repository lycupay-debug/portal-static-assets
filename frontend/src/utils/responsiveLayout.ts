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
  if (resizeTimer) clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => void applyResponsivePolicy(), 160)
}

export function initializeResponsiveLayout(): void {
  void applyResponsivePolicy()
  window.addEventListener('resize', scheduleResponsivePolicy, { passive: true })
  window.addEventListener('orientationchange', scheduleResponsivePolicy, { passive: true })
}
