package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"toggl-notifier/toggl"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	client, err := toggl.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	loc := time.Now().Location()
	day := time.Now()
	if d := r.URL.Query().Get("date"); d != "" {
		parsed, err := time.ParseInLocation("2006-01-02", d, loc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
		day = parsed
	}

	entries, err := client.EntriesForDay(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
