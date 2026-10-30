/**
 * Responsibility: cache one asynchronous loader initialization while allowing retry after failure.
 * Dependencies: a caller-provided promise factory; this helper has no framework or browser dependency.
 * Collaboration: portalApplication.ts uses it for lazily imported list-loader modules, sharing concurrent initialization.
 * Boundaries: successful values remain cached; rejected initialization is not cached, while operation errors
 * occurring after initialization belong to the loaded module and do not reset this cache.
 */
export function createRetryableLazyLoader<T>(load: () => Promise<T>): () => Promise<T> {
  let cached: Promise<T> | undefined

  return () => {
    if (cached) return cached
    const pending = Promise.resolve().then(load).catch((error: unknown) => {
      if (cached === pending) cached = undefined
      throw error
    })
    cached = pending
    return pending
  }
}
