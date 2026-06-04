package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
	_ "time/tzdata"

	"toggl-notifier/auth"
	"toggl-notifier/cronjob"
	"toggl-notifier/gcal"
	"toggl-notifier/kv"
)

const localTZ = "Europe/Oslo"

func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}

	appURL := os.Getenv("APP_URL")
	if appURL == "" {
		writeErr(w, http.StatusInternalServerError, "APP_URL is not set")
		return
	}
	cronSecret := os.Getenv("CRON_SECRET")
	if cronSecret == "" {
		writeErr(w, http.StatusInternalServerError, "CRON_SECRET is not set")
		return
	}

	loc, err := time.LoadLocation(localTZ)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load tz: "+err.Error())
		return
	}

	now := time.Now().In(loc)
	dateKey := "cronjob:daily:" + now.Format("2006-01-02")

	gc, err := gcal.New(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "gcal: "+err.Error())
		return
	}
	events, err := gc.EventsForDay(r.Context(), now)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "gcal: "+err.Error())
		return
	}
	if len(events) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"skipped": "no calendar events today"})
		return
	}

	fireAt := events[0].Start.Add(15 * time.Minute)

	cj, err := cronjob.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	title := fmt.Sprintf("toggl-notifier remind_start %s", now.Format("2006-01-02"))
	jobID, err := cj.Create(r.Context(), title, appURL+"/api/remind_start", cronSecret, fireAt)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "cronjob create: "+err.Error())
		return
	}

	kvc, err := kv.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "kv: "+err.Error())
		return
	}
	if err := kvc.Set(r.Context(), dateKey, strconv.FormatInt(jobID, 10)); err != nil {
		writeErr(w, http.StatusInternalServerError, "kv set: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"date":   now.Format("2006-01-02"),
		"jobID":  jobID,
		"fireAt": fireAt.Format(time.RFC3339),
		"event":  events[0].Summary,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
