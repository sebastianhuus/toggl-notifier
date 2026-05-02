# toggl-notifier

Compares Toggl Track data for today with Google Calendar and sends an email if you've logged less hours than planned. 

The project is quite tailored but essentially allows the following workflow:
1. User logs in with Google account, grants access to calendar read, Gmail send
2. User schedules workday with particular color `CALENDAR_COLOR_ID` in Google Calendar
3. When `/api/check` endpoint hit (typically by cronjob), calculate sum difference between total time logged in a particular Toggl Track project today vs total time planned for the configured calendar event color `CALENDAR_COLOR_ID` today. 
   - If difference is greater than `NOTIFY_THRESHOLD_MINUTES` set in .env, an email is sent to `NOTIFY_EMAIL`  
   - Otherwise do nothing
4. When `/api/weekly` is hit (typically by a cronjob on Saturday), the same comparison is run over the period from the most recent Monday strictly before today through yesterday. So a Saturday cron checks Mon–Fri; a Monday cron checks the prior Mon–Sun.
   - If the gap exceeds `WEEKLY_NOTIFY_THRESHOLD_MINUTES` (defaults to 30), an email is sent to `NOTIFY_EMAIL`.
   - Dedup is keyed on ISO year-week so the same week never gets two notifications.
   - Pass `?as_of=YYYY-MM-DD` to simulate a different "today" for testing; `?dry_run=1` to skip sending; `?force=1` to bypass dedup.

Based on usefulness, might make this into a PWA you can install on iOS to allow the use of notifications without needing a full-fledged iOS app.