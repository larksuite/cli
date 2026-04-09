# Calendar CLI E2E Coverage

## Metrics

- Denominator: 22 leaf commands
- Covered: 11
- Coverage: 50.0%

## Summary

- TestCalendar_CreateEvent: proves `calendar calendars primary`, `calendar +create`, `calendar events get`, and `calendar events delete`; key proof points are `get primary calendar`, `create event with shortcut`, `verify event created`, and `delete event`.
- TestCalendar_ManageCalendar: proves `calendar calendars list`, `calendar calendars create`, `calendar calendars patch`, `calendar calendars search`, and `calendar calendars delete`; key proof points are `create calendar`, `update calendar`, `search calendars`, and `delete calendar`.
- TestCalendar_ViewAgenda: proves `calendar +agenda` with the default range and an explicit `--start/--end` range.
- TestCalendar_FindMeetingTime: proves `calendar +suggestion` for basic scheduling and timezone-aware scheduling.
- Blocked area: `calendar +freebusy` and attendee-based `calendar +suggestion` coverage require stable real `open_id` fixtures for target users.
- Blocked area: `calendar +rsvp` is user-only and is skipped in the current bot-oriented suite.
- Gap pattern: direct `event.attendees *`, `events create/patch/search/instance_view`, `calendars get`, and `freebusys list` APIs still lack deterministic workflows.

## Command Table

| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | calendar +agenda | shortcut | calendar_view_agenda_test.go::TestCalendar_ViewAgenda/view today agenda; calendar_view_agenda_test.go::TestCalendar_ViewAgenda/view agenda with date range | default invocation; `--start --end` | |
| ✓ | calendar +create | shortcut | calendar_create_event_test.go::TestCalendar_CreateEvent/create event with shortcut | `--summary --start --end --calendar-id --description` | |
| ✕ | calendar +freebusy | shortcut |  | none | skipped in `calendar_query_freebusy_test.go`; requires real user `open_id` fixtures |
| ✕ | calendar +rsvp | shortcut |  | none | skipped in `calendar_reply_invite_test.go`; user-only workflow |
| ✓ | calendar +suggestion | shortcut | calendar_find_meeting_time_test.go::TestCalendar_FindMeetingTime/find available meeting times; calendar_find_meeting_time_test.go::TestCalendar_FindMeetingTime/find meeting times with timezone | `--start --end --duration-minutes`; optional `--timezone` | attendee-based case is skipped |
| ✓ | calendar calendars create | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/create calendar | request body with `summary` and `description` | |
| ✓ | calendar calendars delete | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/delete calendar | `calendar_id` in `--params` | |
| ✕ | calendar calendars get | api |  | none | no dedicated direct get workflow yet |
| ✓ | calendar calendars list | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/list calendars | no required params | |
| ✓ | calendar calendars patch | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/update calendar | `calendar_id` in `--params`; request body with updated fields | |
| ✓ | calendar calendars primary | api | calendar_create_event_test.go::TestCalendar_CreateEvent/get primary calendar; calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/get primary calendar | no required params | |
| ✓ | calendar calendars search | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/search calendars | query params with search keyword | |
| ✕ | calendar event.attendees batch_delete | api |  | none | requires deterministic attendee fixtures created in the same workflow |
| ✕ | calendar event.attendees create | api |  | none | requires deterministic attendee fixtures created in the same workflow |
| ✕ | calendar event.attendees list | api |  | none | requires deterministic attendee fixtures created in the same workflow |
| ✕ | calendar events create | api |  | none | only covered indirectly through `calendar +create` |
| ✓ | calendar events delete | api | calendar_create_event_test.go::TestCalendar_CreateEvent/delete event | `calendar_id` and `event_id` in `--params` | |
| ✓ | calendar events get | api | calendar_create_event_test.go::TestCalendar_CreateEvent/verify event created | `calendar_id` and `event_id` in `--params` | |
| ✕ | calendar events instance_view | api |  | none | no recurring-event fixture yet |
| ✕ | calendar events patch | api |  | none | no direct event-update workflow yet |
| ✕ | calendar events search | api |  | none | no direct event-search workflow yet |
| ✕ | calendar freebusys list | api |  | none | requires stable real user fixtures and direct free/busy assertions |
