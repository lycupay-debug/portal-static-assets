package main

// Resource API entry point: authenticates requests, resolves resource routes,
// and delegates collection, item lifecycle, and update work to focused handlers.

import (
	"net/http"
	"strings"
)

// resources authenticates once, resolves the resource route, and delegates each
// operation to the handler that owns that request business responsibility.
func (a *portalAPI) resources(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	if path == "resources" {
		switch r.Method {
		case http.MethodGet:
			a.listResources(w, r, u)
		case http.MethodPost:
			a.createResource(w, r, u)
		default:
			writeError(w, http.StatusNotFound, "未找到资源接口。", nil)
		}
		return
	}
	if path == "resources/trash" && r.Method == http.MethodGet {
		a.listResourceTrash(w, r, u)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 || !portalUUID.MatchString(parts[1]) {
		writeError(w, http.StatusNotFound, "未找到资源接口。", nil)
		return
	}
	id := parts[1]
	action := ""
	if len(parts) > 2 {
		action = parts[2]
	}
	switch {
	case action == "preview" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		a.serveResourceMP3Preview(w, r, id, false, u.ID)
	case action == "download" && r.Method == http.MethodGet:
		a.downloadResource(w, r, u, id)
	case action == "restore" && r.Method == http.MethodPost:
		a.restoreResource(w, r, u, id)
	case action == "permanent" && r.Method == http.MethodDelete:
		a.permanentlyDeleteResource(w, r, u, id)
	case action != "":
		writeError(w, http.StatusNotFound, "未找到资源接口。", nil)
	case r.Method == http.MethodDelete:
		a.trashResource(w, r, u, id)
	case r.Method == http.MethodPatch:
		a.patchResource(w, r, u, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式。", nil)
	}
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// eventMetricsHandler exposes database-derived Prometheus gauges. It is an
// internal endpoint; the public Caddy gateway explicitly blocks /api/metrics.
