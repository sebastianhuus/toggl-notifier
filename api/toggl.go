package handler

import (
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

const togglBase = "https://api.track.toggl.com/api/v9"

type TimeEntry struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Start       string  `json:"start"`
	Stop        *string `json:"stop,omitempty"`
	Duration    int64   `json:"duration"`
	ProjectID   *int64  `json:"project_id,omitempty"`
	WorkspaceID int64   `json:"workspace_id"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("TOGGL_API_TOKEN")
	if token == "" {
		writeErr(w, http.StatusInternalServerError, "TOGGL_API_TOKEN is not set")
		return
	}
	wsRaw := os.Getenv("TOGGL_WORKSPACE_ID")
	if wsRaw == "" {
		writeErr(w, http.StatusInternalServerError, "TOGGL_WORKSPACE_ID is not set")
		return
	}
	workspaceID, err := strconv.ParseInt(wsRaw, 10, 64)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TOGGL_WORKSPACE_ID must be an integer")
		return
	}
	projRaw := os.Getenv("TOGGL_PROJECT_ID")
	if projRaw == "" {
		writeErr(w, http.StatusInternalServerError, "TOGGL_PROJECT_ID is not set")
		return
	}
	projectID, err := strconv.ParseInt(projRaw, 10, 64)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "TOGGL_PROJECT_ID must be an integer")
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

	params := url.Values{}
	params.Set("start_date", start.Format(time.RFC3339))
	params.Set("end_date", end.Format(time.RFC3339))
	reqURL := fmt.Sprintf("%s/me/time_entries?%s", togglBase, params.Encode())

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, reqURL, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Authorization", basicAuth(token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		writeErr(w, http.StatusUnauthorized, "toggl authentication failed; check TOGGL_API_TOKEN")
		return
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("toggl %d: %s", resp.StatusCode, body))
		return
	}

	var all []TimeEntry
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries := make([]TimeEntry, 0, len(all))
	for _, e := range all {
		if e.WorkspaceID != workspaceID {
			continue
		}
		if e.ProjectID == nil || *e.ProjectID != projectID {
			continue
		}
		entries = append(entries, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func basicAuth(token string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(token + ":api_token"))
	return "Basic " + encoded
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
