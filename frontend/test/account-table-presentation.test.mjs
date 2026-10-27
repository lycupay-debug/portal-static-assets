import assert from 'node:assert/strict'
import test from 'node:test'
import { buildTableColumnTemplate, tableCellPreview, tableCellValue } from '../src/utils/accountTablePresentation.ts'

test('cell formatting preserves zero values and supplies placeholders only for missing values', () => {
  assert.equal(tableCellValue({ count: 0 }, 'count'), '0')
  assert.equal(tableCellValue({ title: '' }, 'title'), '未命名内容')
  assert.equal(tableCellValue({}, 'status'), '—')
})

test('cell previews truncate after ten Unicode code points', () => {
  assert.equal(tableCellPreview('1234567890'), '1234567890')
  assert.equal(tableCellPreview('中文测试文本长度超过十个字符'), '中文测试文本长度超过…')
})

test('table layout fixes action column and caps columns when content exceeds viewport', () => {
  const columns = [{ key: 'title', label: '标题' }, { key: 'actions', label: '操作' }]
  const result = buildTableColumnTemplate(columns, [{ title: 'A very long title that exceeds ten chars', actions: 'open' }], 320, 900)
  assert.match(result, /minmax\(330px, max-content\)$/)
  assert.match(result, /72px/)
})

test('table layout expands natural columns and handles empty definitions', () => {
  assert.equal(buildTableColumnTemplate([], [], 600, 900), '100%')
  const result = buildTableColumnTemplate([{ key: 'title', label: '标题' }, { key: 'status', label: '状态' }], [{ title: '短标题', status: '正常' }], 1000, 900)
  const widths = result.split(' ').map(value => Number.parseInt(value, 10))
  assert.equal(widths.length, 2)
  assert.ok(widths.every(width => width >= 72))
  assert.ok(widths[0] + widths[1] <= 1000)
})
