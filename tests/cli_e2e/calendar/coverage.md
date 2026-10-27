# Calendar CLI E2E Coverage

## Metrics
- Denominator: 36 leaf commands (18 shortcuts + 18 API commands)
- Covered: 12
- Coverage: 33.3%

## Summary
- TestCalendar_ViewAgenda: proves the user shortcut `calendar +agenda`; key `t.Run(...)` proof points are `view today agenda as user`, `view agenda with date range as user`, and `view agenda with pretty format as user`.
- TestCalendar_PersonalEventWorkflowAsUser_AATCompatible: proves a self-contained user event workflow across `calendar calendars primary`, `calendar +create`, `calendar events get`, and `calendar +agenda`; key `t.Run(...)` proof points are `get primary calendar as user`, `create personal event with shortcut as user`, `get created event as user`, and `find created event in agenda as user`.
- TestCalendar_RSVPWorkflowAsUser: proves the user shortcuts `calendar +freebusy` and `calendar +rsvp`; key `t.Run(...)` proof points are `query freebusy as user`, `reply tentative as user`, `verify tentative freebusy as user`, `reply accept as user`, and `verify accepted freebusy as user`.
- TestCalendar_CreateEvent: proves `calendar +create`, `calendar events get`, and `calendar events delete`; key `t.Run(...)` proof points are `create event with shortcut as bot`, `verify event created as bot`, and `delete event as bot`.
- TestCalendar_UpdateEventWorkflow: proves `calendar +update`; key proof points are updating the created event as bot and reading back its title, description, and time changes. The attendee parameter is sent but attendee membership is not asserted.
- TestCalendar_ManageCalendar: proves `calendar calendars primary`, `calendar calendars create`, `calendar calendars get`, and `calendar calendars patch`; key `t.Run(...)` proof points are `get primary calendar as bot`, `create calendar as bot`, `get created calendar as bot`, and `update calendar as bot`.
- TestCalendar_SubscriptionDryRun and TestCalendar_SubscriptionRequiresCalendarID prove the compiled `+subscribe` / `+unsubscribe` request method, ordinary target path, absent body/query, and required target without credentials. TestCalendar_SubscriptionGuidanceEmbedded verifies help and shipped reference reachability. Path-segment encoding for accepted IDs containing spaces and slashes is covered by the unit test TestCalendarSubscriptionSuccess.
- TestCalendar_SubscriptionWorkflow is a self-contained bot workflow: create a public shared calendar, unsubscribe its initial owner subscription, subscribe, read the calendar back, unsubscribe, assert a repeated unsubscribe returns 191007, then delete in cleanup. The local run on 2026-10-26 skipped because the environment provides only a user credential. Live coverage remains unconfirmed; the fixture needs bot calendar create/read/delete/subscribe permissions and bot capability. Unit HTTP mocks cover both identities, unsafe input, identity fallback, typed errors and causes; real user subscription and tenant-dependent permission/limit failures remain unverified.
- Cleanup note: `calendar calendars delete` is part of the calendar lifecycle workflow and is counted as covered because the workflow proves the full shared-calendar lifecycle.
- Blocked area: unproven commands are listed below; planning shortcuts depend on live tenant availability and room inventory. Dry-run-only proof does not count toward live coverage.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | calendar +agenda | shortcut | calendar_view_agenda_test.go::TestCalendar_ViewAgenda; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser_AATCompatible/find created event in agenda as user | default today; `--start`; `--end`; `--format pretty` | user identity readback plus general agenda view |
| ✓ | calendar +create | shortcut | calendar_create_event_test.go::TestCalendar_CreateEvent/create event with shortcut as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser_AATCompatible/create personal event with shortcut as user | `--summary`; `--start`; `--end`; `--calendar-id`; `--description` | bot and user workflow coverage |
| ✕ | calendar +delete | shortcut | | `--event-id`; `--apply-to` | no self-contained live shortcut deletion workflow |
| ✓ | calendar +freebusy | shortcut | calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/query freebusy as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/verify tentative freebusy as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/verify accepted freebusy as user | default current user; `--start`; `--end` | user identity flow |
| ✕ | calendar +get | shortcut | | `--event-id` | no self-contained live shortcut read workflow |
| ✕ | calendar +join-event | shortcut | | `--token` | requires an isolated share token workflow |
| ✕ | calendar +list-attendees | shortcut | | `--event-id`; `--type` | no live shortcut attendee workflow |
| ✕ | calendar +meeting | shortcut | | `--event-ids` | requires a completed video meeting |
| ✕ | calendar +meeting-chat-create | shortcut | | `--event-id` | requires an eligible event and meeting-chat lifecycle |
| ✕ | calendar +meeting-chat-get | shortcut | | `--event-id` | requires a meeting-chat lifecycle |
| ✕ | calendar +room-find | shortcut |  | none | no deterministic self-contained workflow yet; output depends on live room inventory |
| ✓ | calendar +rsvp | shortcut | calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/reply tentative as user; calendar_rsvp_workflow_test.go::TestCalendar_RSVPWorkflowAsUser/reply accept as user | `--calendar-id`; `--event-id`; `--rsvp-status` | user reply flow |
| ✕ | calendar +search-event | shortcut | | `--query`; `--start`; `--end` | no self-contained live search workflow |
| ✕ | calendar +subscribe | shortcut | calendar_subscription_workflow_test.go::TestCalendar_SubscriptionWorkflow/subscribe to shared calendar as bot | required `--calendar-id`; explicit bot identity | workflow added; live run skipped without bot credentials; dry-run and unit proof passed |
| ✕ | calendar +suggestion | shortcut |  | none | no deterministic self-contained workflow yet; output depends on live availability suggestions |
| ✕ | calendar +transfer | shortcut | | `--event-id`; `--to-user-id`; `--yes` | dry-run covered; no cleanable live transfer workflow |
| ✕ | calendar +unsubscribe | shortcut | calendar_subscription_workflow_test.go::TestCalendar_SubscriptionWorkflow/unsubscribe shared calendar as bot; calendar_subscription_workflow_test.go::TestCalendar_SubscriptionWorkflow/repeat unsubscribe reports persisted state | required `--calendar-id`; preserves error 191007 | workflow added; live run skipped without bot credentials; dry-run and unit proof passed |
| ✓ | calendar +update | shortcut | calendar_update_event_test.go::TestCalendar_UpdateEventWorkflow | `--summary`; `--description`; `--start`; `--end` | read-after-write fields verified; attendee membership unverified |
| ✓ | calendar calendars create | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/create calendar as bot | `summary`; `description` in `--data` | |
| ✓ | calendar calendars delete | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/delete calendar as bot | `calendar_id` in `--params` | |
| ✓ | calendar calendars get | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/get created calendar as bot; calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/verify updated calendar as bot | `calendar_id` in `--params` | |
| ✕ | calendar calendars list | api |  | none | removed from the live workflow because tenant history made list latency non-deterministic |
| ✓ | calendar calendars patch | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/update calendar as bot | `calendar_id` in `--params`; `summary` in `--data` | |
| ✓ | calendar calendars primary | api | calendar_manage_calendar_test.go::TestCalendar_ManageCalendar/get primary calendar as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser_AATCompatible/get primary calendar as user | none | bot and user primary calendar lookup |
| ✕ | calendar calendars search | api |  | none | no search workflow yet |
| ✕ | calendar events create | api |  | none | only covered indirectly through `calendar +create` |
| ✓ | calendar events delete | api | calendar_create_event_test.go::TestCalendar_CreateEvent/delete event as bot | `calendar_id`; `event_id` in `--params` | |
| ✓ | calendar events get | api | calendar_create_event_test.go::TestCalendar_CreateEvent/verify event created as bot; calendar_personal_event_workflow_test.go::TestCalendar_PersonalEventWorkflowAsUser_AATCompatible/get created event as user | `calendar_id`; `event_id` in `--params` | bot and user read-after-write coverage |
| ✕ | calendar events instance_view | api |  | none | `+agenda` is indirect orchestration, not direct API coverage |
| ✕ | calendar events patch | api |  | none | no direct event-update workflow yet |
| ✕ | calendar events search_event | api |  | none | no direct search workflow yet |
| ✕ | calendar events share_info | api | | none | no isolated share-link workflow yet |
| ✕ | calendar freebusys list | api |  | none | no direct freebusy API workflow yet |
| ✕ | calendar event.attendees batch_delete | api |  | none | requires an isolated attendee lifecycle workflow |
| ✕ | calendar event.attendees create | api |  | none | requires an isolated attendee lifecycle workflow |
| ✕ | calendar event.attendees list | api |  | none | requires an isolated attendee lifecycle workflow |
