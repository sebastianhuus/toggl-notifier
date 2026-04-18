package googleauth

import (
	"fmt"
	"os"

	"golang.org/x/oauth2"
	googleendpoints "golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	gmail "google.golang.org/api/gmail/v1"
)

func Config() (*oauth2.Config, error) {
	id := os.Getenv("GOOGLE_CLIENT_ID")
	secret := os.Getenv("GOOGLE_CLIENT_SECRET")
	redirect := os.Getenv("GOOGLE_REDIRECT_URI")
	if id == "" || secret == "" || redirect == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, and GOOGLE_REDIRECT_URI must all be set")
	}
	return &oauth2.Config{
		ClientID:     id,
		ClientSecret: secret,
		RedirectURL:  redirect,
		Scopes: []string{
			calendar.CalendarEventsReadonlyScope,
			gmail.GmailSendScope,
		},
		Endpoint: googleendpoints.Endpoint,
	}, nil
}
