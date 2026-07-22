# Calendar CLI E2E Coverage

## Metrics
- Denominator: 28 leaf commands
- Covered: 13
- Coverage: 46.4%

## Summary
- TestCalendar_ViewAgenda: proves the user shortcut `calendar +agenda`; key `t.Run(...)` proof points are `view today agenda as user`, `view agenda with date range as user`, and `view agenda with pretty format as user`.
- TestCalendar_PersonalEventWorkflowAsUser: proves a self-contained user event workflow across `calendar calendars primary`, `calendar +create`, `calendar events get`, and `calendar +agenda`; key `t.Run(...)` proof points are `get primary calendar as user`, `create personal event with shortcut as user`, `get created event as user`, and `find created event in agenda as user`.
- TestCalendar_RSVPWorkflowAsUser: proves the user shortcuts `calendar +freebusy` and `calendar +rsvp`; key `t.Run(...)` proof points are `query freebusy as user`, `reply tentative as user`, `verify tentative freebusy as user`, `reply accept as user`, and `verify accepted freebusy as user`.
- TestCalendar_SearchEventDryRunAsBot: proves `calendar +search-event` accepts bot identity and produces the expected `search_event` request path without live credentials.
- TestCalendar_SearchEventWorkflowAsBot: proves a self-contained bot workflow across `calendar +create`, `calendar +search-event`, and `calendar events delete`; key `t.Run(...)` proof points are `create searchable event as bot` and `find created event with shortcut as bot`.
- TestCalendar_UpdateDryRun: proves `calendar +update` request construction as bot, including event mutation and attendee add/remove calls.
- TestCalendar_CreateEvent: proves `calendar +create`, `calendar events get`, and `calendar events delete`; key `t.Run(...)` proof points are `create event with shortcut as bot`, `verify event created as bot`, and `delete event as bot`.
- TestCalendar_ManageCalendar: proves `calendar calendars primary`, `calendar calendars create`, `calendar calendars get`, and `calendar calendars patch`; key `t.Run(...)` proof points are `get primary calendar as bot`, `create calendar as bot`, `get created calendar as bot`, and `update calendar as bot`.
- Cleanup note: `calendar calendars delete` is part of the calendar lifecycle workflow and is counted as covered because the workflow proves the full shared-calendar lifecycle.
- Blocked area: direct `event.attendees *` APIs, `calendar calendars search`, `calendar events create|instance_view|patch|search_event|share_info`, `calendar freebusys list`, and shortcuts `calendar +get` / `calendar +meeting` / `calendar +room-find` / `calendar +suggestion` still need deterministic workflows; the planning shortcuts currently depend on live tenant availability and room inventory, so they remain uncovered.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | calendar +agenda | shortcut | calendar_view_agenda_test.go::TestCalendar_ViewAgenda; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser/find created event in agenda as user | default today; `--start`; `--end`; `--format pretty` | user identity readback plus general agenda view |
| ✓ | calendar +create | shortcut | calendar_create_event_test.go::TestCalendar_CreateEvent/create event with shortcut as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser/create personal event with shortcut as user | `--summary`; `--start`; `--end`; `--calendar-id`; `--description` | bot and user workflow coverage |
| ✓ | calendar +freebusy | shortcut | calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/query freebusy as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/verify tentative freebusy as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/verify accepted freebusy as user | default current user; `--start`; `--end` | user identity flow |
| ✕ | calendar +get | shortcut |  | none | no direct shortcut workflow yet |
| ✕ | calendar +meeting | shortcut |  | none | requires an event with linked meeting artifacts |
| ✕ | calendar +room-find | shortcut |  | none | no deterministic self-contained workflow yet; output depends on live room inventory |
| ✓ | calendar +rsvp | shortcut | calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/reply tentative as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/reply accept as user | `--calendar-id`; `--event-id`; `--rsvp-status` | user reply flow |
| ✓ | calendar +search-event | shortcut | calendar_search_event_workflow_test.go::TestCalendar_SearchEventDryRunAsBot; calendar_search_event_workflow_test.go::TestCalendar_SearchEventWorkflowAsBot/find created event with shortcut as bot | bot identity; `--calendar-id`; `--query`; `--start`; `--end`; `--dry-run` | bot-owned event search plus dry-run request contract |
| ✕ | calendar +suggestion | shortcut |  | none | no deterministic self-contained workflow yet; output depends on live availability suggestions |
| ✓ | calendar +update | shortcut | calendar_update_dryrun_test.go::TestCalendar_UpdateDryRun | event fields plus attendee add/remove under bot identity | dry-run request contract |
| ✓ | calendar calendars create | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/create calendar as bot | `summary`; `description` in `--data` | |
| ✓ | calendar calendars delete | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/delete calendar as bot | `calendar_id` in `--params` | |
| ✓ | calendar calendars get | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/get created calendar as bot; calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/verify updated calendar as bot | `calendar_id` in `--params` | |
| ✕ | calendar calendars list | api |  | none | removed from the live workflow because tenant history made list latency non-deterministic |
| ✓ | calendar calendars patch | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/update calendar as bot | `calendar_id` in `--params`; `summary` in `--data` | |
| ✓ | calendar calendars primary | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/get primary calendar as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser/get primary calendar as user | none | bot and user primary calendar lookup |
| ✕ | calendar calendars search | api |  | none | no search workflow yet |
| ✕ | calendar events create | api |  | none | only covered indirectly through `calendar +create` |
| ✓ | calendar events delete | api | calendar_create_event_test.go::TestCalendar_CreateEvent/delete event as bot | `calendar_id`; `event_id` in `--params` | |
| ✓ | calendar events get | api | calendar_create_event_test.go::TestCalendar_CreateEvent/verify event created as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser/get created event as user | `calendar_id`; `event_id` in `--params` | bot and user read-after-write coverage |
| ✕ | calendar events instance_view | api |  | none | `+agenda` is indirect orchestration, not direct API coverage |
| ✕ | calendar events patch | api |  | none | no direct event-update workflow yet |
| ✕ | calendar events search_event | api |  | none | only covered indirectly through `calendar +search-event` |
| ✕ | calendar events share_info | api |  | none | no event sharing workflow yet |
| ✕ | calendar freebusys list | api |  | none | no direct freebusy API workflow yet |
| ✕ | calendar event.attendees batch_delete | api |  | none | requires an isolated attendee lifecycle workflow |
| ✕ | calendar event.attendees create | api |  | none | requires an isolated attendee lifecycle workflow |
| ✕ | calendar event.attendees list | api |  | none | requires an isolated attendee lifecycle workflow |
