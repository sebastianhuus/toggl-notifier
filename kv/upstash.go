package kv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	RefreshTokenKey    = "google:refresh_token"
	CronScheduleDay    = "cronjob:permanent:schedule_day"
	CronCleanupCron    = "cronjob:permanent:cleanup_cron"
)

type Client struct {
	url   string
	token string
	http  *http.Client
}

func New() (*Client, error) {
	url := os.Getenv("UPSTASH_REDIS_REST_URL")
	token := os.Getenv("UPSTASH_REDIS_REST_TOKEN")
	if url == "" || token == "" {
		return nil, fmt.Errorf("UPSTASH_REDIS_REST_URL and UPSTASH_REDIS_REST_TOKEN must be set")
	}
	return &Client{url: strings.TrimRight(url, "/"), token: token, http: http.DefaultClient}, nil
}

func (c *Client) Set(ctx context.Context, key, value string) error {
	_, err := c.do(ctx, "SET", key, value)
	return err
}

// SetNX sets key=value only if key does not already exist.
// Returns true if the key was set, false if it already existed.
func (c *Client) SetNX(ctx context.Context, key, value string) (bool, error) {
	res, err := c.do(ctx, "SET", key, value, "NX")
	if err != nil {
		return false, err
	}
	return res != nil, nil
}

func (c *Client) Del(ctx context.Context, key string) error {
	_, err := c.do(ctx, "DEL", key)
	return err
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	res, err := c.do(ctx, "GET", key)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	s, ok := res.(string)
	if !ok {
		return "", fmt.Errorf("upstash GET %s: unexpected type %T", key, res)
	}
	return s, nil
}

// LPush prepends value to the list at key. Returns the new list length.
func (c *Client) LPush(ctx context.Context, key, value string) error {
	_, err := c.do(ctx, "LPUSH", key, value)
	return err
}

// LRange returns elements of the list at key from index start to stop (inclusive).
// Use stop=-1 for the full list.
func (c *Client) LRange(ctx context.Context, key string, start, stop int) ([]string, error) {
	res, err := c.do(ctx, "LRANGE", key, strconv.Itoa(start), strconv.Itoa(stop))
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	items, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("upstash LRANGE %s: unexpected type %T", key, res)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("upstash LRANGE %s: element type %T", key, item)
		}
		out = append(out, s)
	}
	return out, nil
}

// LTrim trims the list at key to only contain elements from index start to stop (inclusive).
func (c *Client) LTrim(ctx context.Context, key string, start, stop int) error {
	_, err := c.do(ctx, "LTRIM", key, strconv.Itoa(start), strconv.Itoa(stop))
	return err
}

func (c *Client) do(ctx context.Context, cmd ...string) (any, error) {
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstash %s: %d %s", cmd[0], resp.StatusCode, b)
	}

	var out struct {
		Result any    `json:"result"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != "" {
		return nil, fmt.Errorf("upstash %s: %s", cmd[0], out.Error)
	}
	return out.Result, nil
}
