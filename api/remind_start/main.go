package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	"toggl-notifier/auth"
	"toggl-notifier/gmailsend"
	"toggl-notifier/togglclient"
)

const localTZ = "Europe/Oslo"

func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}

	notifyEmail := os.Getenv("NOTIFY_EMAIL")
	if notifyEmail == "" {
		writeErr(w, http.StatusInternalServerError, "NOTIFY_EMAIL is not set")
		return
	}

	loc, err := time.LoadLocation(localTZ)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load tz: "+err.Error())
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"

	now := time.Now().In(loc)

	tc, err := togglclient.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries, err := tc.EntriesForDay(r.Context(), now)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "toggl: "+err.Error())
		return
	}

	type result struct {
		Date    string `json:"date"`
		Entries int    `json:"entries"`
		DryRun  bool   `json:"dryRun"`
		Sent    bool   `json:"sent"`
		SentTo  string `json:"sentTo,omitempty"`
		Skipped string `json:"skipped,omitempty"`
		SendErr string `json:"sendError,omitempty"`
	}

	res := result{
		Date:    now.Format("2006-01-02"),
		Entries: len(entries),
		DryRun:  dryRun,
	}

	if len(entries) > 0 {
		res.Skipped = "tracking already started"
		writeJSON(w, http.StatusOK, res)
		return
	}

	// No entries — notify unless dry run
	if !dryRun {
		mailer, err := gmailsend.New(r.Context())
		if err != nil {
			res.SendErr = err.Error()
		} else {
			subject, body := buildEmail(now)
			if err := mailer.Send(r.Context(), notifyEmail, subject, body); err != nil {
				res.SendErr = err.Error()
			} else {
				res.Sent = true
				res.SentTo = notifyEmail
			}
		}
	}

	status := http.StatusOK
	if res.SendErr != "" {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, res)
}

func buildEmail(t time.Time) (string, string) {
	subject := fmt.Sprintf("[toggl-notifier] Reminder: start tracking (%s)", t.Format("2006-01-02"))
	body := fmt.Sprintf(
		"No Toggl entries found for today (%s).\n\n"+
			"You have a calendar event scheduled — remember to start tracking!\n\n"+
			"Open Toggl: https://track.toggl.com/timer\n",
		t.Format("2006-01-02"),
	)
	return subject, body
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
