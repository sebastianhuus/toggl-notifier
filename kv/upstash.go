package kv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const RefreshTokenKey = "google:refresh_token"

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
	return &Client{
		url:   strings.TrimRight(url, "/"),
		token: token,
		http:  http.DefaultClient,
	}, nil
}

func (c *Client) Set(ctx context.Context, key, value string) error {
	_, err := c.do(ctx, "SET", key, value)
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
		return "", fmt.Errorf("upstash GET %s: unexpected value type %T", key, res)
	}
	return s, nil
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
