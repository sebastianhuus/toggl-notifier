package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"toggl-notifier/auth"
	"toggl-notifier/cronjob"
	"toggl-notifier/kv"
)


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

	kvc, err := kv.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "kv: "+err.Error())
		return
	}
	cj, err := cronjob.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	type jobResult struct {
		JobID   int64  `json:"jobID"`
		Created bool   `json:"created"`
		Skipped string `json:"skipped,omitempty"`
	}

	results := map[string]jobResult{}

	for _, job := range []struct {
		key    string
		title  string
		path   string
		hour   int
		minute int
	}{
		{kv.CronScheduleDay, "toggl-notifier schedule_day", "/api/schedule_day", 7, 0},
		{kv.CronCleanupCron, "toggl-notifier cleanup_cron", "/api/cleanup_cron", 17, 0},
	} {
		name := strings.TrimPrefix(job.path, "/api/")
		existing, err := kvc.Get(r.Context(), job.key)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "kv get: "+err.Error())
			return
		}
		if existing != "" {
			id, err := strconv.ParseInt(existing, 10, 64)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "corrupt kv for "+name+": "+err.Error())
				return
			}
			results[name] = jobResult{JobID: id, Skipped: "already configured"}
			continue
		}

		jobID, err := cj.CreateDaily(r.Context(), job.title, appURL+job.path, cronSecret, job.hour, job.minute)
		if err != nil {
			writeErr(w, http.StatusBadGateway, name+": "+err.Error())
			return
		}
		if err := kvc.Set(r.Context(), job.key, strconv.FormatInt(jobID, 10)); err != nil {
			writeErr(w, http.StatusInternalServerError, "kv set: "+err.Error())
			return
		}
		results[name] = jobResult{JobID: jobID, Created: true}
		time.Sleep(500 * time.Millisecond)
	}

	writeJSON(w, http.StatusOK, results)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
