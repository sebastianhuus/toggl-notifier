package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"golang.org/x/oauth2"
	"toggl-notifier/googleauth"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	cfg, err := googleauth.Config()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	state, err := randomState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	url := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "select_account consent"),
	)
	http.Redirect(w, r, url, http.StatusFound)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
