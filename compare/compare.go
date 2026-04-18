package compare

import "fmt"

type Report struct {
	Day              string `json:"day"`
	CalendarSeconds  int64  `json:"calendarSeconds"`
	TogglSeconds     int64  `json:"togglSeconds"`
	DeltaSeconds     int64  `json:"deltaSeconds"`
	ThresholdSeconds int64  `json:"thresholdSeconds"`
	NeedsNotify      bool   `json:"needsNotify"`
}

func Run(day string, calendarSeconds, togglSeconds, thresholdSeconds int64) Report {
	delta := calendarSeconds - togglSeconds
	return Report{
		Day:              day,
		CalendarSeconds:  calendarSeconds,
		TogglSeconds:     togglSeconds,
		DeltaSeconds:     delta,
		ThresholdSeconds: thresholdSeconds,
		NeedsNotify:      delta > thresholdSeconds,
	}
}

func FormatDuration(seconds int64) string {
	if seconds < 0 {
		return "-" + FormatDuration(-seconds)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
