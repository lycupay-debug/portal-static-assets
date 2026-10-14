import type Vditor from 'vditor'

export const VDITOR_CDN_PATH = '/vditor'

export const VDITOR_TOOLBAR: string[] = [
  'emoji', 'headings', 'bold', 'italic', 'strike', 'link', '|',
  'list', 'ordered-list', 'check', 'outdent', 'indent', '|',
  'quote', 'line', 'code', 'inline-code', 'insert-before', 'insert-after', '|',
  'table', 'undo', 'redo', '|',
  'both', 'edit-mode', 'preview', 'outline', 'fullscreen',
]

export function createVditorOptions(
  value: string,
  onInput: (markdown: string) => void,
): ConstructorParameters<typeof Vditor>[1] {
  return {
    value,
    lang: 'zh_CN',
    mode: 'wysiwyg',
    theme: 'classic',
    height: 'auto',
    minHeight: 440,
    placeholder: '输入正文，使用工具栏编辑 Markdown…',
    cdn: VDITOR_CDN_PATH,
    cache: { enable: false },
    toolbar: VDITOR_TOOLBAR,
    toolbarConfig: { pin: true, hide: false },
    resize: { enable: true, position: 'bottom' },
    link: { isOpen: false },
    upload: { url: '' },
    preview: {
      markdown: { autoSpace: false },
    },
    input: onInput,
  }
}
