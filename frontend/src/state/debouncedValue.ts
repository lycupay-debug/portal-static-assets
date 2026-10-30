/**
 * Responsibility: provide a framework-independent delayed mirror of a source value.
 * Dependencies: caller-supplied value factory and subscription adapter; no global timer or lifecycle assumptions.
 * Collaboration: Vue may supply watch/ref adapters during migration; native state can supply subscribe/cell adapters.
 * Boundaries: source values are never mutated; dispose cancels pending work and detaches the source listener.
 */
import type { MutableValue, ReadonlyValue } from './valueContracts'

type StateFactory = <T>(initial: T) => MutableValue<T>
type Subscribe<T> = (source: ReadonlyValue<T>, listener: (value: T) => void) => () => void

export function createDebouncedValue<T>(
  source: ReadonlyValue<T>,
  delayMs: number,
  dependencies: { createValue: StateFactory; subscribe: Subscribe<T> },
) {
  if (!Number.isFinite(delayMs) || delayMs < 0) throw new RangeError('Debounce delay must be a finite non-negative number.')
  const value = dependencies.createValue(source.value)
  let timer: ReturnType<typeof setTimeout> | undefined
  const unsubscribe = dependencies.subscribe(source, next => {
    if (timer !== undefined) clearTimeout(timer)
    timer = setTimeout(() => { value.value = next; timer = undefined }, delayMs)
  })
  function dispose() {
    unsubscribe()
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
  }
  return { value, dispose }
}
