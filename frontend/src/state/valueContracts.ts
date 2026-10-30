/**
 * Responsibility: define the minimal mutable/read-only value contracts shared by UI adapters and framework-free workflows.
 * Dependencies: none; these interfaces intentionally describe shape rather than a Vue implementation.
 * Collaboration: signal-backed state cells, native observable cells and deterministic test doubles can all satisfy these contracts.
 * Boundaries: this file provides no scheduling, dependency tracking or mutation policy; callers own those semantics.
 */
export interface ReadonlyValue<T> {
  readonly value: T
}

export interface MutableValue<T> extends ReadonlyValue<T> {
  value: T
}
