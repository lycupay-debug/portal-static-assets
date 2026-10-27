package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

func eventMetricsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		rows, err := db.QueryContext(ctx, `SELECT status, count(*)::int FROM portal.event_inbox GROUP BY status`)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		defer rows.Close()
		counts := map[string]int{"received": 0, "processing": 0, "completed": 0, "failed": 0}
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
				return
			}
			counts[status] = count
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		var dueDispatch, expiredProcessing int
		var oldestReceived sql.NullFloat64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE dispatch_state='pending' AND next_dispatch_at<=now())::int, count(*) FILTER(WHERE status='processing' AND processing_lease_until<now())::int, EXTRACT(EPOCH FROM now()-min(received_at) FILTER (WHERE status='received')) FROM portal.event_inbox`).Scan(&dueDispatch, &expiredProcessing, &oldestReceived); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(w, "# HELP personal_portal_events Current event inbox rows by business status.")
		fmt.Fprintln(w, "# TYPE personal_portal_events gauge")
		for _, status := range []string{"received", "processing", "completed", "failed"} {
			fmt.Fprintf(w, "personal_portal_events{status=%q} %d\n", status, counts[status])
		}
		var failedFileCleanupEvents int
		if err := db.QueryRowContext(ctx, `SELECT count(*)::int FROM portal.event_inbox WHERE event_type=$1 AND status='failed'`, fileCleanupEventType).Scan(&failedFileCleanupEvents); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		fmt.Fprintln(w, "# HELP personal_portal_file_cleanup_events_failed File cleanup events exhausted retries and require inspection.")
		fmt.Fprintln(w, "# TYPE personal_portal_file_cleanup_events_failed gauge")
		fmt.Fprintf(w, "personal_portal_file_cleanup_events_failed %d\n", failedFileCleanupEvents)
		fmt.Fprintln(w, "# HELP personal_portal_event_dispatch_due Events waiting for a dispatch attempt.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_dispatch_due gauge")
		fmt.Fprintf(w, "personal_portal_event_dispatch_due %d\n", dueDispatch)
		fmt.Fprintln(w, "# HELP personal_portal_event_processing_leases_expired Processing leases that have expired and can be recovered.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_processing_leases_expired gauge")
		fmt.Fprintf(w, "personal_portal_event_processing_leases_expired %d\n", expiredProcessing)
		fmt.Fprintln(w, "# HELP personal_portal_event_oldest_received_age_seconds Age of the oldest received event; zero if none exist.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_oldest_received_age_seconds gauge")
		if oldestReceived.Valid {
			fmt.Fprintf(w, "personal_portal_event_oldest_received_age_seconds %.3f\n", oldestReceived.Float64)
		} else {
			fmt.Fprintln(w, "personal_portal_event_oldest_received_age_seconds 0")
		}
	}
}
