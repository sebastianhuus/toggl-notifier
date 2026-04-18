package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"toggl-notifier/internal/googleauth"
)

type CalendarEvent struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Start    string `json:"start"`
	End      string `json:"end"`
	ColorID  string `json:"colorId"`
	HTMLLink string `json:"htmlLink,omitempty"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	refresh := os.Getenv("GOOGLE_REFRESH_TOKEN")
	if refresh == "" {
		writeErr(w, http.StatusInternalServerError, "GOOGLE_REFRESH_TOKEN is not set — visit /api/google_auth to obtain one")
		return
	}
	colorID := os.Getenv("CALENDAR_COLOR_ID")
	if colorID == "" {
		colorID = "3"
	}

	cfg, err := googleauth.Config()
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
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	tokSource := cfg.TokenSource(r.Context(), &oauth2.Token{RefreshToken: refresh})
	svc, err := calendar.NewService(r.Context(), option.WithTokenSource(tokSource))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	events, err := svc.Events.List("primary").
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(r.Context()).
		Do()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	filtered := make([]CalendarEvent, 0, len(events.Items))
	for _, e := range events.Items {
		if e.ColorId != colorID {
			continue
		}
		if e.Organizer == nil || !e.Organizer.Self {
			continue
		}
		if hasOtherAttendees(e.Attendees) {
			continue
		}
		if e.Start == nil || e.Start.DateTime == "" {
			continue
		}
		filtered = append(filtered, CalendarEvent{
			ID:       e.Id,
			Summary:  e.Summary,
			Start:    e.Start.DateTime,
			End:      e.End.DateTime,
			ColorID:  e.ColorId,
			HTMLLink: e.HtmlLink,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func hasOtherAttendees(attendees []*calendar.EventAttendee) bool {
	for _, a := range attendees {
		if !a.Self {
			return true
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
