package togglclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

const baseURL = "https://api.track.toggl.com/api/v9"

type TimeEntry struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Start       string  `json:"start"`
	Stop        *string `json:"stop,omitempty"`
	Duration    int64   `json:"duration"`
	ProjectID   *int64  `json:"project_id,omitempty"`
	WorkspaceID int64   `json:"workspace_id"`
}

func (e TimeEntry) ElapsedSeconds() int64 {
	if e.Duration >= 0 {
		return e.Duration
	}
	start, err := time.Parse(time.RFC3339, e.Start)
	if err != nil {
		return 0
	}
	return int64(time.Since(start).Seconds())
}

type Client struct {
	token       string
	workspaceID int64
	projectID   int64
	http        *http.Client
}

func New() (*Client, error) {
	token := os.Getenv("TOGGL_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TOGGL_API_TOKEN is not set")
	}
	wsRaw := os.Getenv("TOGGL_WORKSPACE_ID")
	if wsRaw == "" {
		return nil, fmt.Errorf("TOGGL_WORKSPACE_ID is not set")
	}
	workspaceID, err := strconv.ParseInt(wsRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TOGGL_WORKSPACE_ID must be an integer")
	}
	projRaw := os.Getenv("TOGGL_PROJECT_ID")
	if projRaw == "" {
		return nil, fmt.Errorf("TOGGL_PROJECT_ID is not set")
	}
	projectID, err := strconv.ParseInt(projRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("TOGGL_PROJECT_ID must be an integer")
	}
	return &Client{token: token, workspaceID: workspaceID, projectID: projectID, http: http.DefaultClient}, nil
}

func (c *Client) EntriesForDay(ctx context.Context, day time.Time) ([]TimeEntry, error) {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)

	params := url.Values{}
	params.Set("start_date", start.Format(time.RFC3339))
	params.Set("end_date", end.Format(time.RFC3339))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/me/time_entries?%s", baseURL, params.Encode()), nil)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(c.token + ":api_token"))
	req.Header.Set("Authorization", "Basic "+encoded)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("toggl auth failed; check TOGGL_API_TOKEN")
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("toggl %d: %s", resp.StatusCode, body)
	}

	var all []TimeEntry
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil, err
	}

	filtered := make([]TimeEntry, 0, len(all))
	for _, e := range all {
		if e.WorkspaceID != c.workspaceID {
			continue
		}
		if e.ProjectID == nil || *e.ProjectID != c.projectID {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered, nil
}

func TotalSeconds(entries []TimeEntry) int64 {
	var total int64
	for _, e := range entries {
		total += e.ElapsedSeconds()
	}
	return total
}
