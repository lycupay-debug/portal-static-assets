package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string, errorsList []string) {
	body := map[string]any{"error": message}
	if errorsList != nil {
		body["errors"] = errorsList
	}
	writeJSON(w, status, body)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, max int64, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("request must contain one JSON value")
	}
	return nil
}
func runeLen(s string) int { return utf8.RuneCountInString(s) }

func (a *portalAPI) dbError(w http.ResponseWriter, err error, message string) {
	fmt.Fprintf(os.Stderr, "portal_api_error operation=%q error=%v\n", message, err)
	if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
		writeError(w, 409, "别名或唯一字段已被使用，请检查后重试。", nil)
		return
	}
	writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
}
