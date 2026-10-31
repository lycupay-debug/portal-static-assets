package main

// Content API entry point: authenticates requests and dispatches collection routes
// or delegates item routes to the owner-scoped content lifecycle handler.
import (
	"net/http"
)

func (a *portalAPI) content(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	switch {
	case r.URL.Path == "/content" && r.Method == http.MethodGet:
		a.listContent(w, r, u)
	case r.URL.Path == "/content/trash" && r.Method == http.MethodGet:
		a.listContentTrash(w, r, u)
	case r.URL.Path == "/content" && r.Method == http.MethodPost:
		a.createContent(w, r, u)
	default:
		a.contentByID(w, r, u)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
