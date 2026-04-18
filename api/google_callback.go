package handler

import (
	"encoding/json"
	"html/template"
	"net/http"

	"toggl-notifier/googleauth"
)

var callbackTmpl = template.Must(template.New("callback").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Google OAuth — refresh token</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 720px; margin: 48px auto; padding: 0 16px; line-height: 1.5; }
code { display: block; padding: 12px; background: #f4f4f4; border-radius: 6px; word-break: break-all; white-space: pre-wrap; }
small { color: #666; }
</style>
</head>
<body>
<h1>Refresh token captured</h1>
<p>Add this to your Vercel project env as <strong>GOOGLE_REFRESH_TOKEN</strong>, then redeploy.</p>
<code>{{.RefreshToken}}</code>
<p><small>Access token expires: {{.Expiry}}</small></p>
</body>
</html>`))

func Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		writeErr(w, http.StatusBadRequest, "oauth error: "+e)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
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

	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", Path: "/", MaxAge: -1})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	callbackTmpl.Execute(w, struct {
		RefreshToken string
		Expiry       string
	}{
		RefreshToken: tok.RefreshToken,
		Expiry:       tok.Expiry.String(),
	})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
