package gmailsend

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/oauth2"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"toggl-notifier/googleauth"
	"toggl-notifier/kv"
)

type Client struct {
	svc *gmail.Service
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
	src := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	svc, err := gmail.NewService(ctx, option.WithTokenSource(src))
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

func (c *Client) Send(ctx context.Context, to, subject, body string) error {
	var buf strings.Builder
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	fmt.Fprintf(&buf, "\r\n%s", body)

	raw := base64.RawURLEncoding.EncodeToString([]byte(buf.String()))
	_, err := c.svc.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Context(ctx).Do()
	return err
}
