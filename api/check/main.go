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
	"toggl-notifier/compare"
	"toggl-notifier/gcal"
	"toggl-notifier/gmailsend"
	"toggl-notifier/kv"
	"toggl-notifier/togglclient"
)

const (
	defaultThresholdMinutes = 30
	localTZ                 = "Europe/Oslo"
	reminderKeyPrefix       = "notify:reminded:"
)

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

	loc, err := time.LoadLocation(localTZ)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "load tz: "+err.Error())
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"

	now := time.Now().In(loc)
	day := now
	if dayParam := r.URL.Query().Get("day"); dayParam != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dayParam, loc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
			return
		}
		day = parsed
	} else if offsetParam := r.URL.Query().Get("offset_days"); offsetParam != "" {
		n, err := strconv.Atoi(offsetParam)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "offset_days must be an integer")
			return
		}
		day = now.AddDate(0, 0, n)
	}

	dateKey := day.Format("2006-01-02")
	isReminder := dateKey != now.Format("2006-01-02")

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
		dateKey,
		gcal.TotalSeconds(events),
		togglclient.TotalSeconds(entries),
		int64(thresholdMin)*60,
	)

	type result struct {
		Report   compare.Report `json:"report"`
		Events   int            `json:"events"`
		Entries  int            `json:"entries"`
		DryRun   bool           `json:"dryRun"`
		Forced   bool           `json:"forced,omitempty"`
		Reminder bool           `json:"reminder,omitempty"`
		Sent     bool           `json:"sent"`
		SentTo   string         `json:"sentTo,omitempty"`
		Skipped  string         `json:"skipped,omitempty"`
		SendErr  string         `json:"sendError,omitempty"`
	}

	res := result{Report: report, Events: len(events), Entries: len(entries), DryRun: dryRun, Forced: force, Reminder: isReminder}

	if (report.NeedsNotify || force) && !dryRun {
		// Reminder mode: claim the date in KV before sending so we never email twice
		// for the same past day. `force` bypasses dedup for ad-hoc testing.
		if isReminder && !force {
			kvc, kerr := kv.New()
			if kerr != nil {
				res.SendErr = "kv: " + kerr.Error()
				writeJSON(w, http.StatusBadGateway, res)
				return
			}
			claimed, kerr := kvc.SetNX(r.Context(), reminderKeyPrefix+dateKey, time.Now().UTC().Format(time.RFC3339))
			if kerr != nil {
				res.SendErr = "kv: " + kerr.Error()
				writeJSON(w, http.StatusBadGateway, res)
				return
			}
			if !claimed {
				res.Skipped = "reminder already sent for " + dateKey
				writeJSON(w, http.StatusOK, res)
				return
			}
		}

		mailer, err := gmailsend.New(r.Context())
		if err != nil {
			res.SendErr = err.Error()
		} else {
			subject, body := buildEmail(report, len(events), len(entries), isReminder)
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

func buildEmail(report compare.Report, events, entries int, isReminder bool) (string, string) {
	subject := fmt.Sprintf("[toggl-notifier] Missing %s of tracking on %s",
		compare.FormatDuration(report.DeltaSeconds), report.Day)
	header := "Today"
	if isReminder {
		subject = fmt.Sprintf("[toggl-notifier] Reminder: missing %s of tracking on %s",
			compare.FormatDuration(report.DeltaSeconds), report.Day)
		header = "Reminder for"
	}
	body := fmt.Sprintf(
		"%s (%s):\n"+
			"  Calendar (filtered):    %s across %d event(s)\n"+
			"  Toggl (project):        %s across %d entry/entries\n"+
			"  Gap (calendar − toggl): %s\n"+
			"  Threshold:              %s\n\n"+
			"Open Toggl: https://track.toggl.com/timer\n",
		header, report.Day,
		compare.FormatDuration(report.CalendarSeconds), events,
		compare.FormatDuration(report.TogglSeconds), entries,
		compare.FormatDuration(report.DeltaSeconds),
		compare.FormatDuration(report.ThresholdSeconds),
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
