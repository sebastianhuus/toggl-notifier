package gcal

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/oauth2"
	calendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"toggl-notifier/googleauth"
	"toggl-notifier/kv"
)

type Event struct {
	ID       string    `json:"id"`
	Summary  string    `json:"summary"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	ColorID  string    `json:"colorId"`
	HTMLLink string    `json:"htmlLink,omitempty"`
}

func (e Event) DurationSeconds() int64 {
	return int64(e.End.Sub(e.Start).Seconds())
}

type Client struct {
	svc     *calendar.Service
	colorID string
}

func New(ctx context.Context) (*Client, error) {
	cfg, err := googleauth.Config()
	if err != nil {
		return nil, err
	}
	store, err := kv.New()
	if err != nil {
		return nil, err
	}
	refresh, err := store.Get(ctx, kv.RefreshTokenKey)
	if err != nil {
		return nil, fmt.Errorf("read refresh token: %w", err)
	}
	if refresh == "" {
		return nil, fmt.Errorf("no refresh token stored — visit /api/google_auth to sign in")
	}
	colorID := os.Getenv("CALENDAR_COLOR_ID")
	if colorID == "" {
		colorID = "3"
	}
	src := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	svc, err := calendar.NewService(ctx, option.WithTokenSource(src))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc, colorID: colorID}, nil
}

func (c *Client) EventsForDay(ctx context.Context, day time.Time) ([]Event, error) {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	return c.eventsBetween(ctx, start, end)
}

// EventsForRange returns events from 00:00 on startDay through 24:00 on endDay
// (endDay inclusive). Both bounds use endDay's location.
func (c *Client) EventsForRange(ctx context.Context, startDay, endDay time.Time) ([]Event, error) {
	loc := endDay.Location()
	start := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, loc)
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)
	return c.eventsBetween(ctx, start, end)
}

func (c *Client) eventsBetween(ctx context.Context, start, end time.Time) ([]Event, error) {
	resp, err := c.svc.Events.List("primary").
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(end.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(resp.Items))
	for _, e := range resp.Items {
		if e.ColorId != c.colorID {
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
		startT, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err != nil {
			continue
		}
		endT, err := time.Parse(time.RFC3339, e.End.DateTime)
		if err != nil {
			continue
		}
		events = append(events, Event{
			ID: e.Id, Summary: e.Summary,
			Start: startT, End: endT,
			ColorID: e.ColorId, HTMLLink: e.HtmlLink,
		})
	}
	return events, nil
}

func TotalSeconds(events []Event) int64 {
	var total int64
	for _, e := range events {
		total += e.DurationSeconds()
	}
	return total
}

func hasOtherAttendees(attendees []*calendar.EventAttendee) bool {
	for _, a := range attendees {
		if !a.Self {
			return true
		}
	}
	return false
}
