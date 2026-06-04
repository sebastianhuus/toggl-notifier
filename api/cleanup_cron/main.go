package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata"

	"toggl-notifier/auth"
	"toggl-notifier/cronjob"
	"toggl-notifier/kv"
)

const localTZ = "Europe/Oslo"

func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}

	loc, err := time.LoadLocation(localTZ)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load tz: "+err.Error())
		return
	}

	now := time.Now().In(loc)
	dateKey := "cronjob:daily:" + now.Format("2006-01-02")

	kvc, err := kv.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "kv: "+err.Error())
		return
	}

	jobIDStr, err := kvc.Get(r.Context(), dateKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "kv get: "+err.Error())
		return
	}
	if jobIDStr == "" {
		writeJSON(w, http.StatusOK, map[string]string{"skipped": "no job scheduled today"})
		return
	}

	jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "parse job ID: "+err.Error())
		return
	}

	cj, err := cronjob.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cj.Delete(r.Context(), jobID); err != nil {
		writeErr(w, http.StatusBadGateway, "cronjob delete: "+err.Error())
		return
	}

	if err := kvc.Del(r.Context(), dateKey); err != nil {
		writeErr(w, http.StatusInternalServerError, "kv del: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"date":    now.Format("2006-01-02"),
		"jobID":   jobID,
		"cleaned": true,
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
