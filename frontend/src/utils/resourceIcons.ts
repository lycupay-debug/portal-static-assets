/** Local Bootstrap Icons mapping shared by public and workspace resource lists. */
export const RESOURCE_FILE_ICON_MAP: Readonly<Record<string, string>> = Object.freeze({
  doc: 'bi-file-earmark-word', docx: 'bi-file-earmark-word', odt: 'bi-file-earmark-word', rtf: 'bi-file-earmark-word',
  xls: 'bi-file-earmark-excel', xlsx: 'bi-file-earmark-excel', ods: 'bi-file-earmark-excel', csv: 'bi-file-earmark-excel',
  ppt: 'bi-file-earmark-ppt', pptx: 'bi-file-earmark-ppt', odp: 'bi-file-earmark-ppt', key: 'bi-file-earmark-ppt',
  pdf: 'bi-file-earmark-pdf',
  png: 'bi-file-earmark-image', jpg: 'bi-file-earmark-image', jpeg: 'bi-file-earmark-image', gif: 'bi-file-earmark-image',
  webp: 'bi-file-earmark-image', svg: 'bi-file-earmark-image', bmp: 'bi-file-earmark-image', tif: 'bi-file-earmark-image', tiff: 'bi-file-earmark-image',
  mp4: 'bi-file-earmark-play', mov: 'bi-file-earmark-play', avi: 'bi-file-earmark-play', mkv: 'bi-file-earmark-play',
  webm: 'bi-file-earmark-play', mp3: 'bi-file-earmark-play', wav: 'bi-file-earmark-play', m4a: 'bi-file-earmark-play',
  zip: 'bi-file-earmark-zip', rar: 'bi-file-earmark-zip', '7z': 'bi-file-earmark-zip', tar: 'bi-file-earmark-zip', gz: 'bi-file-earmark-zip',
  txt: 'bi-file-earmark-text', md: 'bi-file-earmark-text', markdown: 'bi-file-earmark-text', log: 'bi-file-earmark-text',
  html: 'bi-link-45deg', htm: 'bi-link-45deg', url: 'bi-link-45deg',
})

export function resourceFileIcon(filename?: string | null, url?: string | null): string {
  const source = (filename || url || '').split(/[?#]/, 1).join('').toLowerCase()
  const extension = source.includes('.') ? source.split('.').pop() || '' : ''
  if (url && !extension) return 'bi-link-45deg'
  return RESOURCE_FILE_ICON_MAP[extension] || (url ? 'bi-link-45deg' : 'bi-file-earmark')
}
