package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"toggl-notifier/auth"
	"toggl-notifier/gcal"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}
	client, err := gcal.New(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	day := time.Now()
	if d := r.URL.Query().Get("date"); d != "" {
		parsed, err := time.ParseInLocation("2006-01-02", d, time.Now().Location())
		if err != nil {
			writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		day = parsed
	}
	events, err := client.EventsForDay(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
