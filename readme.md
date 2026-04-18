# toggl-notifier

Compares Toggl Track data for today with Google Calendar and sends an email if you've logged less hours than planned. 

The project is quite tailored but essentially allows the following workflow:
1. User logs in with Google account, grants access to calendar read, Gmail send
2. User schedules workday with particular color `CALENDAR_COLOR_ID` in Google Calendar
3. When `/api/check` endpoint hit (typically by cronjob), calculate sum difference between total time logged in a particular Toggl Track project today vs total time planned for the configured calendar event color `CALENDAR_COLOR_ID` today. 
   - If difference is greater than `NOTIFY_THRESHOLD_MINUTES` set in .env, an email is sent to `NOTIFY_EMAIL`  
   - Otherwise do nothing

Based on usefulness, might make this into a PWA you can install on iOS to allow the use of notifications without needing a full-fledged iOS app.