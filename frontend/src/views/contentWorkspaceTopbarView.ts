/**
 * Responsibility: render the content-workspace identity/status bar using native DOM.
 * Dependencies: current title/busy snapshot and one route-exit callback.
 * Collaboration: portalApplication.ts owns route and save state; the editor and settings views own their independent DOM trees.
 * Boundaries: no persistence or API calls; title is inserted as text, and the return action is explicit.
 * Lifecycle: update changes only the status text and disabled state; dispose clears the host.
 */
export function mountContentWorkspaceTopbarView(
  host: HTMLElement,
  options: { getState: () => { title: string; busy: boolean }; returnToList: () => void; cancel: () => void },
) {
  const document = host.ownerDocument
  const brand = document.createElement('a'); brand.className = 'workspace-brand'; brand.href = '/content-management'
  const mark = document.createElement('span'); mark.className = 'workspace-brand-mark'; mark.textContent = 'R.'
  const brandCopy = document.createElement('span'); const title = document.createElement('strong'); title.textContent = '内容工作台'
  const subtitle = document.createElement('small'); brandCopy.append(title, subtitle); brand.append(mark, brandCopy)
  brand.addEventListener('click', event => { event.preventDefault(); options.returnToList() })
  const name = document.createElement('div'); name.className = 'workspace-document-name'
  const dot = document.createElement('span'); dot.className = 'workspace-status-dot'
  const documentTitle = document.createElement('span')
  const status = document.createElement('em'); status.textContent = '草稿'
  name.append(dot, documentTitle, status)
  const back = document.createElement('button'); back.type = 'button'; back.className = 'btn content-secondary-button workspace-return'
  const icon = document.createElement('i'); icon.className = 'bi bi-arrow-left'; icon.setAttribute('aria-hidden', 'true')
  back.append(icon, document.createTextNode(' 返回列表')); back.addEventListener('click', options.cancel)
  host.replaceChildren(brand, name, back)
  function update(state: { title: string; busy: boolean }) {
    documentTitle.textContent = state.title.trim() || '未命名内容'
    back.disabled = state.busy
  }
  update(options.getState())
  return { update, dispose() { host.replaceChildren() } }
}
