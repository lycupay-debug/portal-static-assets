SELECT jsonb_pretty(jsonb_build_object(
  'generated_at', now(),
  'mode', 'report_only',
  'destructive_actions_enabled', false,
  'legal_hold_model_available', false,
  'categories', jsonb_build_object(
    'expired_sessions', jsonb_build_object(
      'count', (SELECT count(*) FROM portal.portal_auth_sessions WHERE expires_at <= now()),
      'oldest_expired_at', (SELECT min(expires_at) FROM portal.portal_auth_sessions WHERE expires_at <= now())
    ),
    'soft_deleted_content', jsonb_build_object(
      'count', (SELECT count(*) FROM portal.portal_content WHERE deleted_at IS NOT NULL),
      'oldest_deleted_at', (SELECT min(deleted_at) FROM portal.portal_content WHERE deleted_at IS NOT NULL)
    ),
    'soft_deleted_resources', jsonb_build_object(
      'count', (SELECT count(*) FROM portal.teaching_resources WHERE deleted_at IS NOT NULL),
      'oldest_deleted_at', (SELECT min(deleted_at) FROM portal.teaching_resources WHERE deleted_at IS NOT NULL)
    ),
    'uploaded_images_not_referenced_by_live_content', jsonb_build_object(
      'count', (
        SELECT count(*) FROM portal.uploaded_images i
        WHERE NOT EXISTS (
          SELECT 1 FROM portal.portal_content c
          WHERE c.deleted_at IS NULL AND position('/uploads/' || i.filename in c.body) > 0
        )
      )
    )
  ),
  'policy_gaps', jsonb_build_array(
    'No approved retention schedule is configured for soft-deleted content or teaching resources.',
    'Expired sessions are candidates for cleanup based on the existing session expiry contract; this report does not delete them.',
    'Unreferenced upload records/files require ownership and access review before deletion.'
  )
));
