package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// wantsJSON checks if the client wants JSON response.
func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

// writeRecord writes a single record in plain text or JSON format.
func writeRecord(w http.ResponseWriter, r *http.Request, fields map[string]interface{}) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fields)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var parts []string
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, formatValue(v)))
	}
	fmt.Fprintf(w, "%s\n", strings.Join(parts, " "))
}

// writeRecords writes multiple records in plain text or JSON format.
func writeRecords(w http.ResponseWriter, r *http.Request, records []map[string]interface{}) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(records)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, fields := range records {
		var parts []string
		for k, v := range fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, formatValue(v)))
		}
		fmt.Fprintf(w, "%s\n", strings.Join(parts, " "))
	}
}

// writeError writes an error response with a hint.
func writeError(w http.ResponseWriter, r *http.Request, status int, msg, hint string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{
			"error": msg,
			"hint":  hint,
		})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, "error: %s | hint: %s\n", msg, hint)
}

// writeOK writes a simple success message.
func writeOK(w http.ResponseWriter, r *http.Request, msg string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": msg})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "ok: %s\n", msg)
}

// formatValue formats a value for plain text output.
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
