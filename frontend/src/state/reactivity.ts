/**
 * Responsibility: provide a small framework-independent reactive state layer for the portal application.
 * Dependencies: @preact/signals-core for dependency tracking; no DOM, Vue, scheduling globals or API access.
 * Collaboration: application controllers consume ReadonlyValue/MutableValue contracts; native views subscribe through
 * watch(), while derived values use computed(). The layer deliberately exposes only the subset the app requires.
 * Boundaries: plain objects/arrays receive deep mutation tracking; class instances, DOM nodes and platform objects
 * remain opaque. Watch callbacks are untracked, coalesced by default, and synchronously flushed only when requested.
 * Lifecycle: every watch() returns a disposer; application teardown must invoke all owned disposers.
 */
import { computed, effect, signal, untracked } from '@preact/signals-core'
import type { MutableValue, ReadonlyValue } from './valueContracts'

export type WatchSource<T> = ReadonlyValue<T> | (() => T)
export type WatchOptions = { deep?: boolean; flush?: 'sync' | 'pre' | 'post' }
type WatchValue<S> = S extends () => infer T ? T : S extends ReadonlyValue<infer T> ? T : never
type WatchTuple<S extends readonly WatchSource<any>[]> = { [K in keyof S]: WatchValue<S[K]> }

let activeWatchScope: Set<() => void> | null = null

/** Group watcher lifetimes under one application mount/dispose boundary. */
export function createWatchScope() {
  const disposers = new Set<() => void>()
  let disposed = false
  return {
    run<T>(callback: () => T): T {
      if (disposed) throw new Error('Cannot run a disposed watch scope')
      const previous = activeWatchScope
      activeWatchScope = disposers
      try { return callback() } finally { activeWatchScope = previous }
    },
    dispose() {
      if (disposed) return
      disposed = true
      for (const dispose of [...disposers]) dispose()
      disposers.clear()
    },
  }
}

function isTrackable(value: unknown): value is Record<PropertyKey, unknown> | unknown[] {
  if (value === null || typeof value !== 'object') return false
  const prototype = Object.getPrototypeOf(value)
  return Array.isArray(value) || prototype === Object.prototype || prototype === null
}

/** Create a signal-backed cell with ref-compatible `.value` semantics and deep plain-object tracking. */
export function ref<T>(initial: T): MutableValue<T> {
  const version = signal(0)
  const proxies = new WeakMap<object, object>()

  const wrap = <V>(value: V): V => {
    if (!isTrackable(value)) return value
    const target = value as object
    const cached = proxies.get(target)
    if (cached) return cached as V
    const proxy = new Proxy(target, {
      get(object, property, receiver) {
        // A deep watcher traverses the proxy and subscribes to this version signal.
        version.value
        return wrap(Reflect.get(object, property, receiver))
      },
      set(object, property, next, receiver) {
        const previous = Reflect.get(object, property, receiver)
        const success = Reflect.set(object, property, wrap(next), receiver)
        if (success && !Object.is(previous, next)) version.value += 1
        return success
      },
      deleteProperty(object, property) {
        const existed = Reflect.has(object, property)
        const success = Reflect.deleteProperty(object, property)
        if (success && existed) version.value += 1
        return success
      },
    })
    proxies.set(target, proxy)
    return proxy as V
  }

  const value = signal(wrap(initial))
  return {
    get value() { return value.value },
    set value(next: T) { value.value = wrap(next) },
  }
}

export { computed }

function readSource<T>(source: WatchSource<T>): T {
  return typeof source === 'function' ? (source as () => T)() : source.value
}

function traverse(value: unknown, seen = new WeakSet<object>()): void {
  if (!isTrackable(value)) return
  const object = value as object
  if (seen.has(object)) return
  seen.add(object)
  for (const key of Reflect.ownKeys(object)) traverse((value as Record<PropertyKey, unknown>)[key], seen)
}

/**
 * Observe a cell, derived getter or tuple of sources. Default/pre/post callbacks are coalesced to a microtask,
 * matching the application's previous batched watcher behavior. `flush: sync` is reserved for immediate side effects.
 */
export function watch<const S extends readonly WatchSource<any>[]>(source: S, callback: (value: WatchTuple<S>, oldValue: WatchTuple<S>) => void, options?: WatchOptions): () => void
export function watch<T>(source: WatchSource<T>, callback: (value: T, oldValue: T) => void, options?: WatchOptions): () => void
export function watch(source: WatchSource<any> | readonly WatchSource<any>[], callback: (value: any, oldValue: any) => void, options: WatchOptions = {}): () => void {
  const sources = (Array.isArray(source) ? source : [source]) as readonly WatchSource<unknown>[]
  let oldValues: unknown[] = []
  let initialized = false
  let disposed = false
  let queued = false
  let pendingValues: unknown[] | null = null

  const dispatch = (values: unknown[]) => {
    const previous = oldValues
    oldValues = values
    const currentResult = Array.isArray(source) ? values : values[0]
    const previousResult = Array.isArray(source) ? previous : previous[0]
    untracked(() => callback(currentResult, previousResult))
  }

  const disposeEffect = effect(() => {
    const values = sources.map(readSource)
    if (options.deep) for (const value of values) traverse(value)
    if (!initialized) {
      oldValues = values
      initialized = true
      return
    }
    const changed = options.deep === true || values.some((value, index) => !Object.is(value, oldValues[index]))
    if (!changed) return
    if (options.flush === 'sync') {
      dispatch(values)
      return
    }
    pendingValues = values
    if (queued) return
    queued = true
    queueMicrotask(() => {
      queued = false
      if (disposed || !pendingValues) return
      const next = pendingValues
      pendingValues = null
      dispatch(next)
    })
  })

  const stop = () => {
    if (disposed) return
    disposed = true
    pendingValues = null
    disposeEffect()
    activeWatchScope?.delete(stop)
  }
  activeWatchScope?.add(stop)
  return stop
}

/** Microtask scheduling boundary for code that must run after queued state observers. */
export function nextTick(): Promise<void> {
  return Promise.resolve()
}
