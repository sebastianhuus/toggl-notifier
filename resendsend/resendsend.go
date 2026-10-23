package resendsend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const baseURL = "https://api.resend.com"

type Client struct {
	apiKey string
	from   string
	http   *http.Client
}

func New() (*Client, error) {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("RESEND_API_KEY is not set")
	}
	from := os.Getenv("RESEND_FROM")
	if from == "" {
		return nil, fmt.Errorf("RESEND_FROM is not set")
	}
	return &Client{apiKey: key, from: from, http: http.DefaultClient}, nil
}

func (c *Client) Send(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(map[string]any{
		"from":    c.from,
		"to":      []string{to},
		"subject": subject,
		"text":    body,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend send: %d %s", resp.StatusCode, b)
	}
	return nil
}
