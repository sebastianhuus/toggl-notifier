package handler

import (
	"encoding/json"
	"net/http"

	"toggl-notifier/auth"
	"toggl-notifier/errlog"
	"toggl-notifier/kv"
)

// Handler returns the error log, newest entry first.
// Protected by the same CRON_SECRET bearer token as all other endpoints.
func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}
	kvc, err := kv.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "kv: "+err.Error())
		return
	}
	entries, err := errlog.Read(r.Context(), kvc)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "kv: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
