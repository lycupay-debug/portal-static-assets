/**
 * Responsibility: expose a delayed reactive copy of a source ref for debounced user input.
 * Dependencies: Vue refs, watchers, and effect-scope cleanup; no application or network modules.
 * Collaboration: a component/composable passes its source ref and delay, then watches the returned ref.
 * Boundaries and side effects: owns one bounded timer; each source change cancels the previous timer,
 * and stopping the owning Vue effect scope cancels pending work. It does not mutate the source value.
 */
import { onScopeDispose, ref, watch, type Ref } from 'vue'

/** Keep the latest source value after it remains unchanged for `delayMs`. */
export function useDebouncedValue<T>(source: Readonly<Ref<T>>, delayMs: number): Ref<T> {
  if (!Number.isFinite(delayMs) || delayMs < 0) {
    throw new RangeError('Debounce delay must be a finite non-negative number.')
  }

  const debounced = ref(source.value) as Ref<T>
  let timer: ReturnType<typeof setTimeout> | undefined

  const stop = watch(source, (value) => {
    if (timer !== undefined) clearTimeout(timer)
    timer = setTimeout(() => {
      debounced.value = value
      timer = undefined
    }, delayMs)
  })

  onScopeDispose(() => {
    stop()
    if (timer !== undefined) clearTimeout(timer)
  })

  return debounced
}
