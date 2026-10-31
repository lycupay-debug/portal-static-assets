package main

// Shared compliance audit helper captures before/after state in the caller transaction for asset, license, legal-document, evidence, and user-role changes.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

func (a *portalAPI) logComplianceChangeTx(ctx context.Context, tx *sql.Tx, r *http.Request, u *portalUser, action, objectType, objectID string, before map[string]any) error {
	copyState := func(state map[string]any) map[string]any {
		copy := map[string]any{}
		for key, value := range state {
			copy[key] = value
		}
		return copy
	}
	beforeJSON, err := json.Marshal(copyState(before))
	if err != nil {
		return err
	}
	queries := map[string]string{
		"asset":          "SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x",
		"license":        "SELECT to_jsonb(x) FROM (SELECT license_id,spdx_id,license_name,license_version,license_family,canonical_url,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status,reviewed_at,notes,updated_at FROM portal.compliance_license_catalog WHERE license_id=$1) x",
		"legal_document": "SELECT to_jsonb(x) FROM (SELECT document_id,slug,title,document_type,jurisdiction,status,version,effective_at,last_legal_review_at,missing_information,approved_by,updated_at FROM portal.compliance_legal_documents WHERE document_id=$1) x",
		"asset_evidence": "SELECT to_jsonb(x) FROM (SELECT evidence_id,asset_id,evidence_type,evidence_uri,sha256,verified,verified_by,verified_at,captured_at,notes FROM portal.compliance_asset_evidence WHERE evidence_id=$1) x",
		"portal_user":    "SELECT to_jsonb(x) FROM (SELECT user_id::text AS id,email,display_name AS displayName,is_admin AS isAdmin,is_copyright_reviewer AS isCopyrightReviewer FROM portal.portal_users WHERE user_id=$1::uuid) x",
	}
	query := queries[objectType]
	if query == "" {
		return fmt.Errorf("unsupported compliance audit object type %q", objectType)
	}
	after, err := oneJSON(ctx, tx, query, objectID)
	if err != nil {
		return fmt.Errorf("read post-change compliance state: %w", err)
	}
	after = copyState(after)
	after["actor_display_name"] = u.DisplayName
	after["request_path"] = r.URL.Path
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb)", u.DisplayName, action, objectType, objectID, string(beforeJSON), string(afterJSON)); err != nil {
		return fmt.Errorf("write compliance audit record: %w", err)
	}
	return nil
}
