package handler

import (
	"encoding/json"
	"html/template"
	"net/http"

	"toggl-notifier/googleauth"
	"toggl-notifier/kv"
)

var successTmpl = template.Must(template.New("ok").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Signed in</title>
<style>body{font-family:system-ui,sans-serif;max-width:640px;margin:48px auto;padding:0 16px;line-height:1.5}a{color:#0b5fff}</style>
</head>
<body>
<h1>Signed in</h1>
<p>Refresh token stored. You can close this tab.</p>
<p><a href="/api/calendar">Try /api/calendar</a></p>
</body></html>`))

func Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		writeErr(w, http.StatusBadRequest, "oauth error: "+e)
		return
	}
	code, state := q.Get("code"), q.Get("state")
	if code == "" || state == "" {
		writeErr(w, http.StatusBadRequest, "missing code or state")
		return
	}
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		writeErr(w, http.StatusBadRequest, "state mismatch — restart the flow at /api/google_auth")
		return
	}
	cfg, err := googleauth.Config()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	tok, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "token exchange failed: "+err.Error())
		return
	}
	if tok.RefreshToken == "" {
		writeErr(w, http.StatusInternalServerError, "no refresh token returned — revoke app access at https://myaccount.google.com/permissions then retry")
		return
	}
	store, err := kv.New()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := store.Set(r.Context(), kv.RefreshTokenKey, tok.RefreshToken); err != nil {
		writeErr(w, http.StatusBadGateway, "failed to persist refresh token: "+err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", Path: "/", MaxAge: -1})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	successTmpl.Execute(w, nil)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
