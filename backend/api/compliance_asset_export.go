package main

// Compliance asset export owns the read-only CSV representation and streaming response headers.

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
)

func (a *portalAPI) exportAssetsCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT a.asset_id::text,a.asset_type,a.asset_name,a.asset_path,a.source_url,a.creator_or_rightsholder,a.ownership_basis,coalesce(l.spdx_id,''),a.permission_status,a.use_scope,a.distribution_status,a.review_status,a.risk_level,a.sha256,coalesce(a.file_size_bytes,0),a.published_or_active,a.notes FROM portal.compliance_assets a LEFT JOIN portal.compliance_license_catalog l ON l.license_id=a.license_id ORDER BY a.risk_level,a.asset_type,a.asset_name`)
	if err != nil {
		a.dbError(w, err, "无法导出资产台账。")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="copyright-asset-ledger.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte("\xef\xbb\xbf")); err != nil {
		log.Printf("compliance_asset_export_failed stage=bom_write")
		return
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"asset_id", "asset_type", "asset_name", "asset_path", "source_url", "rightsholder", "ownership_basis", "license_spdx", "permission_status", "use_scope", "distribution_status", "review_status", "risk_level", "sha256", "file_size_bytes", "active", "notes"}); err != nil {
		log.Printf("compliance_asset_export_failed stage=header_write")
		return
	}
	for rows.Next() {
		var record []string
		var id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, notes string
		var size int64
		var active bool
		if err = rows.Scan(&id, &typ, &name, &path, &source, &rightsholder, &ownership, &license, &permission, &scope, &distribution, &status, &risk, &sha, &size, &active, &notes); err != nil {
			log.Printf("compliance_asset_export_failed stage=row_scan")
			return
		}
		record = []string{id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, strconv.FormatInt(size, 10), strconv.FormatBool(active), notes}
		if err = cw.Write(record); err != nil {
			log.Printf("compliance_asset_export_failed stage=row_write")
			return
		}
	}
	if err = rows.Err(); err != nil {
		log.Printf("compliance_asset_export_failed stage=row_iteration")
		return
	}
	cw.Flush()
	if err = cw.Error(); err != nil {
		log.Printf("compliance_asset_export_failed stage=flush")
	}
}
