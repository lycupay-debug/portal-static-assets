/**
 * Responsibility: render public article content, author metadata, interactions and sharing with native DOM.
 * Dependencies: caller-owned article/loading/auth state, pre-sanitized Markdown HTML and action callbacks.
 * Collaboration: article loader and action controller own API semantics; this view presents their state and intent.
 * Boundaries: API metadata is always assigned as text; only DOMPurify-rendered Markdown HTML enters innerHTML.
 */
type PublicArticle = {
  title: string; slug: string; contentType: string; tags?: string[] | null; summary?: string | null; publishedAt?: string | null
  authorName?: string | null; likeCount?: number; favoriteCount?: number; viewerLiked?: boolean; viewerFavorited?: boolean
}
type ArticleState = { article: PublicArticle | null; html: string; loading: boolean; error: string; busy: boolean; loggedIn: boolean; shareOpen: boolean; shareUrl: string }
type Interaction = 'like' | 'favorite'

function dateLabel(value?: string | null, fallback = '近期') {
  if (!value) return fallback
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? fallback : date.toLocaleDateString('zh-CN')
}

export function mountPublicArticleView(host: HTMLElement, options: {
  contentTypeLabels: Record<string, string>
  toggleInteraction: (type: Interaction) => unknown
  shareArticle: () => unknown
  copyShareUrl: () => unknown
  closeShare: () => unknown
  navigate: (target: string) => unknown
}) {
  const document = host.ownerDocument
  const state = document.createElement('section')
  state.className = 'public-content-state'
  const layout = document.createElement('section')
  layout.className = 'public-article-layout'
  const article = document.createElement('article')
  article.className = 'public-article'
  const header = document.createElement('header')
  header.className = 'public-article-route-header'
  header.setAttribute('aria-label', '文章信息')
  const type = document.createElement('span')
  type.className = 'public-article-route-type'
  const published = document.createElement('span')
  published.className = 'public-article-route-date'
  header.append(type, published)
  const title = document.createElement('h1')
  const summary = document.createElement('p')
  summary.className = 'public-article-summary'
  const body = document.createElement('div')
  body.className = 'public-article-body markdown-rendered'
  article.append(header, title, summary, body)

  const sidebar = document.createElement('aside')
  sidebar.className = 'public-article-sidebar'
  sidebar.setAttribute('aria-label', '作者信息与文章操作')
  const author = document.createElement('section')
  author.className = 'public-author-card'
  const avatar = document.createElement('div')
  avatar.className = 'public-author-avatar'
  avatar.setAttribute('aria-hidden', 'true')
  const authorHeading = document.createElement('div')
  authorHeading.className = 'public-author-heading'
  const authorLabel = document.createElement('span')
  authorLabel.textContent = '本文作者'
  const authorName = document.createElement('strong')
  authorHeading.append(authorLabel, authorName)
  const authorDescription = document.createElement('p')
  authorDescription.textContent = '持续整理经验、知识与实践记录。'
  const authorMeta = document.createElement('div')
  authorMeta.className = 'public-author-meta'
  author.append(avatar, authorHeading, authorDescription, authorMeta)

  const actions = document.createElement('section')
  actions.className = 'public-article-actions'
  actions.setAttribute('aria-label', '文章互动')
  const makeAction = (icon: string, label: string, callback: () => unknown) => {
    const button = document.createElement('button')
    button.type = 'button'
    button.className = 'public-article-action'
    const symbol = document.createElement('span')
    symbol.className = `public-action-symbol bi ${icon}`
    symbol.setAttribute('aria-hidden', 'true')
    const text = document.createElement('span')
    text.textContent = label
    const count = document.createElement('strong')
    button.append(symbol, text, count)
    button.addEventListener('click', callback)
    actions.append(button)
    return { button, text, count }
  }
  const like = makeAction('bi-heart-fill', '点赞', () => options.toggleInteraction('like'))
  const favorite = makeAction('bi-star-fill', '收藏', () => options.toggleInteraction('favorite'))
  const share = makeAction('bi-share', '转发 / 分享', options.shareArticle)
  const loginHint = document.createElement('p')
  loginHint.className = 'public-article-login-hint'
  loginHint.textContent = '登录后可点赞和收藏文章。'
  sidebar.append(author, actions, loginHint)
  layout.append(article, sidebar)

  const shareBackdrop = document.createElement('div')
  shareBackdrop.className = 'share-dialog-backdrop'
  const shareDialog = document.createElement('section')
  shareDialog.className = 'share-dialog'
  shareDialog.setAttribute('role', 'dialog')
  shareDialog.setAttribute('aria-modal', 'true')
  shareDialog.setAttribute('aria-labelledby', 'share-dialog-title')
  const close = document.createElement('button')
  close.type = 'button'
  close.className = 'share-dialog-close'
  close.setAttribute('aria-label', '关闭分享窗口')
  close.innerHTML = '<i class="bi bi-x-lg" aria-hidden="true"></i>'
  const eyebrow = document.createElement('span')
  eyebrow.className = 'share-dialog-eyebrow'
  eyebrow.textContent = 'SHARE ARTICLE'
  const shareTitle = document.createElement('h2')
  shareTitle.id = 'share-dialog-title'
  shareTitle.textContent = '分享这篇文章'
  const shareDescription = document.createElement('p')
  shareDescription.textContent = '复制下方链接，发送给需要阅读的人。'
  const shareLabel = document.createElement('label')
  shareLabel.className = 'share-dialog-label'
  shareLabel.htmlFor = 'share-dialog-url'
  shareLabel.textContent = '文章链接'
  const shareInput = document.createElement('input')
  shareInput.id = 'share-dialog-url'
  shareInput.className = 'form-control share-dialog-url'
  shareInput.type = 'text'
  shareInput.readOnly = true
  const shareActions = document.createElement('div')
  shareActions.className = 'share-dialog-actions'
  const cancel = document.createElement('button')
  cancel.type = 'button'
  cancel.className = 'share-dialog-cancel'
  cancel.textContent = '取消'
  const copy = document.createElement('button')
  copy.type = 'button'
  copy.className = 'share-dialog-copy'
  copy.textContent = '▢ 复制链接'
  shareActions.append(cancel, copy)
  shareDialog.append(close, eyebrow, shareTitle, shareDescription, shareLabel, shareInput, shareActions)
  shareBackdrop.append(shareDialog)
  host.replaceChildren(state, layout, shareBackdrop)

  const onClose = () => options.closeShare()
  const onBackdropClick = (event: MouseEvent) => { if (event.target === shareBackdrop) onClose() }
  const onEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
  const onSelect = () => shareInput.select()
  close.addEventListener('click', onClose)
  cancel.addEventListener('click', onClose)
  shareBackdrop.addEventListener('click', onBackdropClick)
  shareBackdrop.addEventListener('keydown', onEscape)
  shareInput.addEventListener('focus', onSelect)
  copy.addEventListener('click', options.copyShareUrl)

  function update(value: ArticleState) {
    state.replaceChildren()
    state.hidden = true
    layout.hidden = true
    if (value.loading) {
      state.textContent = '正在读取内容…'
      state.hidden = false
    } else if (!value.article) {
      const heading = document.createElement('h1')
      heading.textContent = '内容暂不可用'
      const message = document.createElement('p')
      message.textContent = value.error || '内容不存在或尚未公开。'
      const home = document.createElement('a')
      home.className = 'btn btn-dark button button-primary'
      home.href = '/'
      home.textContent = '返回首页'
      home.addEventListener('click', event => { event.preventDefault(); options.navigate('/') })
      state.append(heading, message, home)
      state.hidden = false
    } else {
      const item = value.article
      type.textContent = item.tags?.length ? item.tags.join(' · ') : options.contentTypeLabels[item.contentType] || item.contentType
      published.textContent = item.publishedAt ? dateLabel(item.publishedAt, '') : ''
      published.hidden = !item.publishedAt
      title.textContent = item.title
      summary.textContent = item.summary || ''
      summary.hidden = !item.summary
      // This is output from the existing DOMPurify-backed Markdown renderer, never raw API content.
      body.innerHTML = value.html
      authorName.textContent = item.authorName || '平台作者'
      avatar.textContent = (item.authorName || '作者').slice(0, 1)
      authorMeta.textContent = `发布于 ${dateLabel(item.publishedAt)}`
      like.button.classList.toggle('active', Boolean(item.viewerLiked))
      like.button.setAttribute('aria-pressed', String(Boolean(item.viewerLiked)))
      like.button.disabled = value.busy
      like.text.textContent = item.viewerLiked ? '已点赞' : '点赞'
      like.count.textContent = String(item.likeCount || 0)
      favorite.button.classList.toggle('active', Boolean(item.viewerFavorited))
      favorite.button.setAttribute('aria-pressed', String(Boolean(item.viewerFavorited)))
      favorite.button.disabled = value.busy
      favorite.text.textContent = item.viewerFavorited ? '已收藏' : '收藏'
      favorite.count.textContent = String(item.favoriteCount || 0)
      share.button.disabled = false
      share.count.textContent = ''
      loginHint.hidden = value.loggedIn
      layout.hidden = false
    }
    shareBackdrop.hidden = !value.shareOpen
    shareInput.value = value.shareUrl
  }

  return {
    update,
    dispose() {
      close.removeEventListener('click', onClose)
      cancel.removeEventListener('click', onClose)
      shareBackdrop.removeEventListener('click', onBackdropClick)
      shareBackdrop.removeEventListener('keydown', onEscape)
      shareInput.removeEventListener('focus', onSelect)
      copy.removeEventListener('click', options.copyShareUrl)
      host.replaceChildren()
    },
  }
}
