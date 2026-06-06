// Package errlog records handler errors to KV so they survive past the current
// Vercel log window. Entries are stored as a Redis list (newest first) and pruned
// to the last 24 h / 200 entries on every write.
package errlog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"toggl-notifier/kv"
)

const (
	LogKey  = "errors:log"
	maxAge  = 24 * time.Hour
	maxSize = 200
)

// Entry is a single logged error.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Handler   string `json:"handler"`
	Error     string `json:"error"`
}

// Log records an upstream error in KV and prunes stale entries.
// KV failures are printed to stderr for local debugging but never
// returned to callers — this must not affect the caller's response path.
func Log(ctx context.Context, handler, errMsg string) {
	kvc, err := kv.New()
	if err != nil {
		fmt.Printf("[errlog] kv.New failed: %v\n", err)
		return
	}
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Handler:   handler,
		Error:     errMsg,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if err := kvc.LPush(ctx, LogKey, string(b)); err != nil {
		fmt.Printf("[errlog] LPush failed: %v\n", err)
		return
	}
	prune(ctx, kvc)
}

// Read returns all current log entries, newest first.
func Read(ctx context.Context, kvc *kv.Client) ([]Entry, error) {
	all, err := kvc.LRange(ctx, LogKey, 0, -1)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(all))
	for _, raw := range all {
		var e Entry
		if json.Unmarshal([]byte(raw), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// prune removes entries older than maxAge, keeping at most maxSize entries.
// Because we always LPUSH, the list is ordered newest→oldest, so we find
// the cutoff index and LTRIM.
func prune(ctx context.Context, kvc *kv.Client) {
	all, err := kvc.LRange(ctx, LogKey, 0, -1)
	if err != nil || len(all) == 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	keep := 0
	for _, raw := range all {
		if keep >= maxSize {
			break
		}
		var e Entry
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			break // malformed → stop; everything after is also suspect
		}
		t, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil || !t.After(cutoff) {
			break // too old → everything after is also too old
		}
		keep++
	}
	if keep >= len(all) {
		return // nothing to prune
	}
	if keep == 0 {
		_ = kvc.Del(ctx, LogKey)
	} else {
		_ = kvc.LTrim(ctx, LogKey, 0, keep-1)
	}
}
