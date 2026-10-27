import assert from 'node:assert/strict'
import test from 'node:test'
import { educationResourceSearchRank } from '../src/utils/resourceSearch.ts'

test('resource ranking prioritizes exact filename and title matches', () => {
  const item = { filename: 'FreeBSD Handbook.pdf', title: 'FreeBSD 系统手册' }
  assert.equal(educationResourceSearchRank(item, 'FreeBSD Handbook.pdf'), 0)
  assert.equal(educationResourceSearchRank(item, 'freebsdhandbook.pdf'), 1)
  assert.equal(educationResourceSearchRank(item, 'FreeBSD 系统手册'), 2)
  assert.equal(educationResourceSearchRank(item, 'Handbook'), 3)
  assert.equal(educationResourceSearchRank(item, 'freebsd 系统'), 5)
})

test('resource ranking handles empty optional fields and normalized whitespace', () => {
  assert.equal(educationResourceSearchRank({}, 'anything'), 7)
  assert.equal(educationResourceSearchRank({ filename: '  学习 资料.pdf ' }, '学习资料.pdf'), 1)
})
