/**
 * Responsibility: render a searchable, windowed single-select control using native DOM APIs.
 * Dependencies: the caller-provided option/value contract and the host element's Document.
 * Collaboration: the caller owns selected state and option data; this module emits selection intent
 * and owns only transient query, open, scroll, and rendered DOM state.
 * Boundaries: no framework dependency, API calls, global state, or HTML-string parsing. Labels are written
 * through textContent. Empty-string option values remain valid selections.
 * Cleanup: dispose removes document listeners and all rendered DOM; repeated disposal is harmless.
 */
export type VirtualizedSelectOption = { label: string; value: string }

export type VirtualizedSelectOptions = {
  label: string
  placeholder?: string
  disabled?: boolean
  options: VirtualizedSelectOption[]
  value: string | null
  onChange: (value: string) => void
}

type SelectState = {
  open: boolean
  query: string
  scrollTop: number
  options: VirtualizedSelectOption[]
  value: string | null
  disabled: boolean
}

const ROW_HEIGHT = 36
const VIEWPORT_HEIGHT = 216
const OVERSCAN = 2

export function mountVirtualizedSelect(root: HTMLElement, initial: VirtualizedSelectOptions): {
  update: (next: Partial<Pick<VirtualizedSelectOptions, 'options' | 'value' | 'disabled'>>) => void
  dispose: () => void
} {
  const document = root.ownerDocument
  const state: SelectState = {
    open: false,
    query: '',
    scrollTop: 0,
    options: initial.options,
    value: initial.value,
    disabled: initial.disabled ?? false,
  }
  let disposed = false

  const normalizedQuery = () => state.query.trim().toLocaleLowerCase()
  const filteredOptions = () => {
    const query = normalizedQuery()
    return state.options.filter(option => option.label.toLocaleLowerCase().includes(query))
  }

  function close() {
    if (!state.open) return
    state.open = false
    render()
  }

  function onDocumentMouseDown(event: MouseEvent) {
    if (!root.contains(event.target as Node)) close()
  }

  function onDocumentKeyDown(event: KeyboardEvent) {
    if (event.key === 'Escape') close()
  }

  function createButton(className: string, text: string, onClick: () => void): HTMLButtonElement {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = className
    button.textContent = text
    button.addEventListener('click', onClick)
    return button
  }

  function renderOptionList(viewport: HTMLElement) {
    const filtered = filteredOptions()
    const startIndex = Math.max(0, Math.floor(state.scrollTop / ROW_HEIGHT) - OVERSCAN)
    const endIndex = Math.min(filtered.length, startIndex + Math.ceil(VIEWPORT_HEIGHT / ROW_HEIGHT) + OVERSCAN * 2)
    const children: HTMLElement[] = []
    const topSpacer = document.createElement('div')
    topSpacer.style.height = `${startIndex * ROW_HEIGHT}px`
    children.push(topSpacer)
    for (const option of filtered.slice(startIndex, endIndex)) {
      const optionButton = createButton('', '', () => {
        initial.onChange(option.value)
        state.value = option.value
        state.open = false
        state.query = ''
        state.scrollTop = 0
        render()
      })
      optionButton.setAttribute('role', 'option')
      optionButton.setAttribute('aria-selected', String(option.value === state.value))
      optionButton.classList.toggle('selected', option.value === state.value)
      optionButton.style.height = `${ROW_HEIGHT}px`
      const optionLabel = document.createElement('span')
      optionLabel.textContent = option.label
      optionButton.append(optionLabel)
      if (option.value === state.value) {
        const check = document.createElement('i')
        check.className = 'bi bi-check2'
        check.setAttribute('aria-hidden', 'true')
        optionButton.append(check)
      }
      children.push(optionButton)
    }
    const bottomSpacer = document.createElement('div')
    bottomSpacer.style.height = `${Math.max(0, (filtered.length - endIndex) * ROW_HEIGHT)}px`
    children.push(bottomSpacer)
    if (!filtered.length) {
      const empty = document.createElement('p')
      empty.className = 'virtualized-select-empty'
      empty.textContent = '没有匹配的选项'
      children.push(empty)
    }
    viewport.replaceChildren(...children)
  }

  function render() {
    if (disposed) return
    const selected = state.options.find(option => option.value === state.value)
    const wrapper = document.createElement('div')
    wrapper.className = `virtualized-select${state.open ? ' is-open' : ''}${state.disabled ? ' is-disabled' : ''}`

    const trigger = createButton('virtualized-select-trigger', '', () => {
      if (state.disabled) return
      if (state.open) {
        state.open = false
      } else {
        state.open = true
        state.query = ''
        state.scrollTop = 0
      }
      render()
      if (state.open) root.querySelector<HTMLInputElement>('.virtualized-select-search')?.focus()
    })
    trigger.setAttribute('aria-label', initial.label)
    trigger.setAttribute('aria-expanded', String(state.open))
    trigger.disabled = state.disabled
    const selectedLabel = document.createElement('span')
    selectedLabel.textContent = selected?.label ?? initial.placeholder ?? '请选择'
    const chevron = document.createElement('i')
    chevron.className = 'virtualized-select-chevron bi bi-chevron-down'
    chevron.setAttribute('aria-hidden', 'true')
    trigger.append(selectedLabel, chevron)
    wrapper.append(trigger)

    if (state.open) {
      const popover = document.createElement('div')
      popover.className = 'virtualized-select-popover'
      popover.setAttribute('role', 'listbox')
      popover.setAttribute('aria-label', initial.label)
      const search = document.createElement('input')
      search.type = 'search'
      search.name = 'optionSearch'
      search.className = 'form-control virtualized-select-search'
      search.placeholder = '搜索选项…'
      search.setAttribute('aria-label', `搜索${initial.label}`)
      search.value = state.query
      search.addEventListener('input', () => {
        state.query = search.value
        state.scrollTop = 0
        render()
        const nextSearch = root.querySelector<HTMLInputElement>('.virtualized-select-search')
        nextSearch?.focus()
        if (nextSearch) nextSearch.setSelectionRange(state.query.length, state.query.length)
      })
      search.addEventListener('keydown', event => {
        event.stopPropagation()
        if (event.key === 'Escape') close()
      })
      popover.append(search)

      const viewport = document.createElement('div')
      viewport.className = 'virtualized-select-viewport'
      viewport.style.height = `${VIEWPORT_HEIGHT}px`
      viewport.addEventListener('scroll', () => {
        state.scrollTop = viewport.scrollTop
        renderOptionList(viewport)
      })
      renderOptionList(viewport)
      popover.append(viewport)
      wrapper.append(popover)
    }
    root.replaceChildren(wrapper)
  }

  document.addEventListener('mousedown', onDocumentMouseDown)
  document.addEventListener('keydown', onDocumentKeyDown)
  render()

  return {
    update(next) {
      if (disposed) return
      if (next.options) state.options = next.options
      if (next.value !== undefined) state.value = next.value
      if (next.disabled !== undefined) state.disabled = next.disabled
      if (state.disabled) state.open = false
      state.scrollTop = 0
      render()
    },
    dispose() {
      if (disposed) return
      disposed = true
      document.removeEventListener('mousedown', onDocumentMouseDown)
      document.removeEventListener('keydown', onDocumentKeyDown)
      root.replaceChildren()
    },
  }
}
