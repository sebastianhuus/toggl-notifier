package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"toggl-notifier/auth"
	"toggl-notifier/compare"
	"toggl-notifier/gcal"
	"toggl-notifier/gmailsend"
	"toggl-notifier/togglclient"
)

const defaultThresholdMinutes = 30

func Handler(w http.ResponseWriter, r *http.Request) {
	if !auth.Require(w, r) {
		return
	}

	notifyEmail := os.Getenv("NOTIFY_EMAIL")
	if notifyEmail == "" {
		writeErr(w, http.StatusInternalServerError, "NOTIFY_EMAIL is not set")
		return
	}
	thresholdMin := defaultThresholdMinutes
	if raw := os.Getenv("NOTIFY_THRESHOLD_MINUTES"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			writeErr(w, http.StatusInternalServerError, "NOTIFY_THRESHOLD_MINUTES must be a non-negative integer")
			return
		}
		thresholdMin = v
	}

	dryRun := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"

	day := time.Now()

	tc, err := togglclient.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries, err := tc.EntriesForDay(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "toggl: "+err.Error())
		return
	}

	gc, err := gcal.New(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := gc.EventsForDay(r.Context(), day)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "calendar: "+err.Error())
		return
	}

	report := compare.Run(
		day.Format("2006-01-02"),
		gcal.TotalSeconds(events),
		togglclient.TotalSeconds(entries),
		int64(thresholdMin)*60,
	)

	type result struct {
		Report  compare.Report `json:"report"`
		Events  int            `json:"events"`
		Entries int            `json:"entries"`
		DryRun  bool           `json:"dryRun"`
		Forced  bool           `json:"forced,omitempty"`
		Sent    bool           `json:"sent"`
		SentTo  string         `json:"sentTo,omitempty"`
		SendErr string         `json:"sendError,omitempty"`
	}

	res := result{Report: report, Events: len(events), Entries: len(entries), DryRun: dryRun, Forced: force}

	if (report.NeedsNotify || force) && !dryRun {
		mailer, err := gmailsend.New(r.Context())
		if err != nil {
			res.SendErr = err.Error()
		} else {
			subject := fmt.Sprintf("[toggl-notifier] Missing %s of tracking on %s",
				compare.FormatDuration(report.DeltaSeconds), report.Day)
			body := fmt.Sprintf(
				"Today (%s):\n"+
					"  Calendar (filtered):    %s across %d event(s)\n"+
					"  Toggl (project):        %s across %d entry/entries\n"+
					"  Gap (calendar − toggl): %s\n"+
					"  Threshold:              %s\n\n"+
					"Open Toggl: https://track.toggl.com/timer\n",
				report.Day,
				compare.FormatDuration(report.CalendarSeconds), len(events),
				compare.FormatDuration(report.TogglSeconds), len(entries),
				compare.FormatDuration(report.DeltaSeconds),
				compare.FormatDuration(report.ThresholdSeconds),
			)
			if err := mailer.Send(r.Context(), notifyEmail, subject, body); err != nil {
				res.SendErr = err.Error()
			} else {
				res.Sent = true
				res.SentTo = notifyEmail
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if res.SendErr != "" {
		w.WriteHeader(http.StatusBadGateway)
	}
	json.NewEncoder(w).Encode(res)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
