export function copyrightReviewLabel(status?: string | null): string {
  if (status === 'approved') return '已通过'
  if (status === 'in_review') return '审核中'
  if (!status || status === 'pending') return '待审核'
  return '涉及违规'
}

export function copyrightReviewIsViolation(status?: string | null): boolean {
  return Boolean(status && status !== 'approved' && status !== 'pending' && status !== 'in_review')
}
