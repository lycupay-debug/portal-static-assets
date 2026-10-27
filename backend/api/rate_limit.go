package main

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func rateLimitClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP := net.ParseIP(strings.Trim(host, "[]"))
	if remoteIP != nil && remoteIP.IsLoopback() {
		// Trust only Caddy's private header after it validates the peer against
		// Cloudflare's trusted-proxy ranges. Direct loopback callers can forge
		// X-Forwarded-For, so that generic header is intentionally ignored.
		forwarded := strings.TrimSpace(r.Header.Get("X-Portal-Client-IP"))
		if forwardedIP := net.ParseIP(forwarded); forwardedIP != nil {
			return forwardedIP.String()
		}
	}
	if remoteIP != nil {
		return remoteIP.String()
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func (a *portalAPI) rateLimited(w http.ResponseWriter, r *http.Request) bool {
	key := rateLimitClientIP(r) + ":" + r.URL.Path
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.rate[key]
	if !now.Before(v.until) {
		v = rateEntry{until: now.Add(15 * time.Minute)}
	}
	v.count++
	a.rate[key] = v
	if len(a.rate) > 5000 {
		for k, e := range a.rate {
			if now.After(e.until) {
				delete(a.rate, k)
			}
		}
	}
	if v.count > 12 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(v.until).Seconds()))))
		writeError(w, 429, "操作过于频繁，请稍后再试。", nil)
		return true
	}
	return false
}
