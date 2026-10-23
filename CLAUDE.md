# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Single-user Go service on Vercel. It compares Toggl Track time (one project) against Google Calendar blocks (one event color) and emails the user via Resend when tracked time falls short of planned time. See `readme.md` for the workflow; `.env.example` documents every env var.

## Commands

There are no tests and no linter config. Module is `toggl-notifier` (Go 1.26).

- `go build ./...` / `go vet ./...` — the only compile/static checks available.
- `vercel dev` — run the functions locally on port 3000 (reads `.env`; the OAuth redirect URI in `.env.example` assumes `localhost:3000`).
- `vercel --prod` — deploy. Env vars live in the Vercel project; `.env`, `.vercel` are gitignored.

Every endpoint except `google_auth` / `google_callback` requires `Authorization: Bearer $CRON_SECRET`. Handlers take query params for safe manual testing: `dry_run=1` (skip sending), `force=1` (bypass dedup), and `day=YYYY-MM-DD` / `offset_days=N` (check) or `as_of=YYYY-MM-DD` (weekly).

## Architecture

**One Vercel function per `api/<name>/main.go`**, each exposing `Handler` in `package handler`. `vercel.json` rewrites `/api/:endpoint` → `/api/:endpoint/main` and redirects `/` to `/api/google_auth`. Shared logic lives in top-level packages; handlers are thin and each redefines its own helpers (`writeJSON`, `writeErr`, `localTZ`) rather than sharing them.

**Auth/state flow.** `google_auth` → `google_callback` stores the Google OAuth refresh token in Upstash Redis (`kv.RefreshTokenKey`). `gcal` builds its token source from that stored token, so nothing works until someone has visited `/api/google_auth` once. `kv` is a minimal Upstash REST client (Get/Set/SetNX/Del/LPush/LRange/LTrim) and is the only persistence.

**Email** goes through `resendsend` (plain HTTP to the Resend API; needs `RESEND_API_KEY` and `RESEND_FROM`). Google OAuth is now calendar-read only.

**Event filtering is the core domain rule** (`gcal/client.go`): only primary-calendar events with `CALENDAR_COLOR_ID` (default `"3"`), organized by the user, with no other attendees, and with a timed (not all-day) start count as "planned time". `compare` turns calendar vs Toggl seconds into a report against a threshold.

**Two notification pipelines:**
- `check` (daily gap) and `weekly` (Mon–Sun gap for the prior period). Past-day reminders in `check` and weekly runs are deduped by claiming a KV key with `SetNX` (`notify:reminded:<date>`, `notify:weekly:<ISO-week>`) *before* sending.
- Dynamic start-of-day reminder, driven by cron-job.org (this service has no scheduler of its own; Vercel Cron is not used):
  1. `setup_crons` creates two permanent daily cron-job.org jobs (07:00 → `schedule_day`, 17:00 → `cleanup_cron`) and stores their IDs in KV (`kv.CronScheduleDay`, `kv.CronCleanupCron`). Idempotent; `?force=true` deletes and recreates.
  2. `schedule_day` takes the first calendar event of today and creates a one-shot cron-job.org job at start+15 min hitting `remind_start`; the job ID goes in KV under `cronjob:daily:<date>`.
  3. `remind_start` emails only if Toggl has zero entries today.
  4. `cleanup_cron` deletes that day's job and KV key (cron-job.org has a 100 API calls/day limit, so keep management calls minimal).
  The `cronjob` package embeds `CRON_SECRET` into each job's Authorization header, and `APP_URL` (no trailing slash) is used to build the callback URLs.

**Error log.** `errlog.Log` pushes upstream-failure entries to a KV list (24 h / 200 entries) readable via `/api/errors`. Only `check` and `weekly` call it; `schedule_day`, `remind_start` and `cleanup_cron` do not.

## Gotchas

- Timezone is hard-coded to `Europe/Oslo` in every handler and in `cronjob/client.go`, with `_ "time/tzdata"` imported because Vercel's runtime has no zoneinfo. Keep new handlers consistent with this.
- `togglclient` is the live Toggl client. `toggl/` is an unused older copy (nothing imports it).
- Toggl returns a negative duration for a running entry; `togglclient` already handles this by computing elapsed time.
- Version control in this checkout is GitButler (`gitbutler/workspace` branch); use the `but` skill instead of raw `git` write commands.
