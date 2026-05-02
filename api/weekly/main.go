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
	defaultWeeklyThresholdMinutes = 30
	localTZ                       = "Europe/Oslo"
	weeklyKeyPrefix               = "notify:weekly:"
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
	thresholdMin := defaultWeeklyThresholdMinutes
	if raw := os.Getenv("WEEKLY_NOTIFY_THRESHOLD_MINUTES"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			writeErr(w, http.StatusInternalServerError, "WEEKLY_NOTIFY_THRESHOLD_MINUTES must be a non-negative integer")
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

	asOf := time.Now().In(loc)
	if raw := r.URL.Query().Get("as_of"); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, loc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "as_of must be YYYY-MM-DD")
			return
		}
		asOf = parsed
	}

	startDay := mostRecentMondayBefore(asOf)
	endDay := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	periodStart := startDay.Format("2006-01-02")
	periodEnd := endDay.Format("2006-01-02")
	isoYear, isoWeek := startDay.ISOWeek()
	isoLabel := fmt.Sprintf("%04d-W%02d", isoYear, isoWeek)

	tc, err := togglclient.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries, err := tc.EntriesForRange(r.Context(), startDay, endDay)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "toggl: "+err.Error())
		return
	}

	gc, err := gcal.New(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := gc.EventsForRange(r.Context(), startDay, endDay)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "calendar: "+err.Error())
		return
	}

	report := compare.RunWeekly(
		periodStart, periodEnd, isoLabel,
		gcal.TotalSeconds(events),
		togglclient.TotalSeconds(entries),
		int64(thresholdMin)*60,
	)

	type result struct {
		Report  compare.WeeklyReport `json:"report"`
		Events  int                  `json:"events"`
		Entries int                  `json:"entries"`
		DryRun  bool                 `json:"dryRun"`
		Forced  bool                 `json:"forced,omitempty"`
		Sent    bool                 `json:"sent"`
		SentTo  string               `json:"sentTo,omitempty"`
		Skipped string               `json:"skipped,omitempty"`
		SendErr string               `json:"sendError,omitempty"`
	}

	res := result{Report: report, Events: len(events), Entries: len(entries), DryRun: dryRun, Forced: force}

	if (report.NeedsNotify || force) && !dryRun {
		if !force {
			kvc, kerr := kv.New()
			if kerr != nil {
				res.SendErr = "kv: " + kerr.Error()
				writeJSON(w, http.StatusBadGateway, res)
				return
			}
			claimed, kerr := kvc.SetNX(r.Context(), weeklyKeyPrefix+isoLabel, time.Now().UTC().Format(time.RFC3339))
			if kerr != nil {
				res.SendErr = "kv: " + kerr.Error()
				writeJSON(w, http.StatusBadGateway, res)
				return
			}
			if !claimed {
				res.Skipped = "weekly report already sent for " + isoLabel
				writeJSON(w, http.StatusOK, res)
				return
			}
		}

		mailer, err := gmailsend.New(r.Context())
		if err != nil {
			res.SendErr = err.Error()
		} else {
			subject, body := buildEmail(report, len(events), len(entries))
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

// mostRecentMondayBefore returns 00:00 on the most recent Monday strictly before t
// (in t's location). If t is itself a Monday, returns the Monday 7 days earlier.
func mostRecentMondayBefore(t time.Time) time.Time {
	loc := t.Location()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	// time.Sunday=0, time.Monday=1, ... time.Saturday=6
	// Days to subtract so we land on the most recent Monday strictly before `day`:
	//   Mon → 7 (last Monday)
	//   Tue → 1, Wed → 2, ..., Sun → 6
	offset := (int(day.Weekday()) + 6) % 7
	if offset == 0 {
		offset = 7
	}
	return day.AddDate(0, 0, -offset)
}

func buildEmail(report compare.WeeklyReport, events, entries int) (string, string) {
	subject := fmt.Sprintf("[toggl-notifier] Weekly: missing %s for %s → %s",
		compare.FormatDuration(report.DeltaSeconds), report.PeriodStart, report.PeriodEnd)
	body := fmt.Sprintf(
		"Weekly report (%s, %s → %s):\n"+
			"  Calendar (filtered):    %s across %d event(s)\n"+
			"  Toggl (project):        %s across %d entry/entries\n"+
			"  Gap (calendar − toggl): %s\n"+
			"  Threshold:              %s\n\n"+
			"Open Toggl: https://track.toggl.com/timer\n",
		report.ISOWeek, report.PeriodStart, report.PeriodEnd,
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
