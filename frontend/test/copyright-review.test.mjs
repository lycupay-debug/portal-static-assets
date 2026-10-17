import test from 'node:test'
import assert from 'node:assert/strict'
import { copyrightReviewIsViolation, copyrightReviewLabel } from '../src/utils/copyrightReview.ts'

test('copyright status labels distinguish waiting, in review and approval', () => {
  assert.equal(copyrightReviewLabel('pending'), '待审核')
  assert.equal(copyrightReviewLabel('in_review'), '审核中')
  assert.equal(copyrightReviewLabel('approved'), '已通过')
  assert.equal(copyrightReviewLabel(null), '待审核')
})

test('only explicitly approved assets pass; all terminal non-approved states are violations', () => {
  for (const status of ['blocked', 'replace_or_remove', 'approved_with_conditions', 'unknown', 'restricted']) {
    assert.equal(copyrightReviewLabel(status), '涉及违规')
    assert.equal(copyrightReviewIsViolation(status), true)
  }
  assert.equal(copyrightReviewIsViolation('pending'), false)
  assert.equal(copyrightReviewIsViolation('in_review'), false)
  assert.equal(copyrightReviewIsViolation('approved'), false)
})
