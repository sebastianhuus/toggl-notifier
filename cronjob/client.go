package cronjob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"
)

const baseURL = "https://api.cron-job.org"

type Client struct {
	apiKey string
	http   *http.Client
}

func New() (*Client, error) {
	key := os.Getenv("CRONJOB_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("CRONJOB_API_KEY must be set")
	}
	return &Client{apiKey: key, http: http.DefaultClient}, nil
}

type jobSchedule struct {
	Timezone string `json:"timezone"`
	Months   []int  `json:"months"`
	Mdays    []int  `json:"mdays"`
	Hours    []int  `json:"hours"`
	Minutes  []int  `json:"minutes"`
	Wdays    []int  `json:"wdays"`
}

// Create schedules a one-time GET request to url at fireAt (Europe/Oslo).
// bearerToken is embedded in the job's Authorization header.
func (c *Client) Create(ctx context.Context, title, url, bearerToken string, fireAt time.Time) (int64, error) {
	oslo, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		return 0, err
	}
	t := fireAt.In(oslo)
	return c.create(ctx, title, url, bearerToken, jobSchedule{
		Timezone: "Europe/Oslo",
		Months:   []int{int(t.Month())},
		Mdays:    []int{t.Day()},
		Hours:    []int{t.Hour()},
		Minutes:  []int{t.Minute()},
		Wdays:    []int{-1},
	})
}

// CreateDaily schedules a recurring daily GET request to url at hour:minute (Europe/Oslo).
// bearerToken is embedded in the job's Authorization header.
func (c *Client) CreateDaily(ctx context.Context, title, url, bearerToken string, hour, minute int) (int64, error) {
	return c.create(ctx, title, url, bearerToken, jobSchedule{
		Timezone: "Europe/Oslo",
		Months:   []int{-1},
		Mdays:    []int{-1},
		Hours:    []int{hour},
		Minutes:  []int{minute},
		Wdays:    []int{-1},
	})
}

func (c *Client) create(ctx context.Context, title, url, bearerToken string, sched jobSchedule) (int64, error) {
	type extendedData struct {
		Headers map[string]string `json:"headers"`
	}
	type job struct {
		Title          string       `json:"title"`
		URL            string       `json:"url"`
		Enabled        bool         `json:"enabled"`
		RequestMethod  int          `json:"requestMethod"` // 0 = GET
		RequestTimeout int          `json:"requestTimeout"`
		Schedule       jobSchedule  `json:"schedule"`
		ExtendedData   extendedData `json:"extendedData"`
	}

	payload, err := json.Marshal(map[string]any{
		"job": job{
			Title:          title,
			URL:            url,
			Enabled:        true,
			RequestMethod:  0,
			RequestTimeout: 30,
			Schedule:       sched,
			ExtendedData:   extendedData{Headers: map[string]string{"Authorization": "Bearer " + bearerToken}},
		},
	})
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/jobs", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("cronjob create: %d %s", resp.StatusCode, b)
	}

	var out struct {
		JobID int64 `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.JobID, nil
}

func (c *Client) Delete(ctx context.Context, jobID int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/jobs/%d", baseURL, jobID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil // already deleted; desired state achieved
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cronjob delete %d: %d %s", jobID, resp.StatusCode, b)
	}
	return nil
}
