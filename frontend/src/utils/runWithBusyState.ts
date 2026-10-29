/**
 * Responsibility: provide a small lifecycle primitive for UI operations with a shared busy indicator.
 * Dependencies: injected setter and an async operation; no Vue, DOM, or network dependencies.
 * Collaboration: callers own their state and error presentation; this helper guarantees cleanup around success,
 * synchronous throws, and rejected promises.
 * Boundaries: it does not swallow errors or serialize concurrent operations; callers must prevent conflicting work.
 */
export async function runWithBusyState<T>(setBusy: (busy: boolean) => void, operation: () => Promise<T>): Promise<T> {
  setBusy(true)
  try {
    return await operation()
  } finally {
    setBusy(false)
  }
}
