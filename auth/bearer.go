package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// Require enforces Bearer <CRON_SECRET>. Writes a 401/500 and returns false if
// the request should not proceed.
func Require(w http.ResponseWriter, r *http.Request) bool {
	secret := os.Getenv("CRON_SECRET")
	if secret == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CRON_SECRET is not set"})
		return false
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
		return false
	}
	token := strings.TrimPrefix(h, prefix)
	if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bearer token"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}
