# Contact CLI E2E Coverage

## Metrics
- Denominator: 2 leaf commands
- Covered: 1
- Coverage: 50.0%

## Summary
- TestContact_GetUser_BotWorkflow: proves `contact +get-user` for bot path with `--user-id`; key proof points are discovery of a real `open_id` via raw API and `get-user-by-id-as-bot` equality assertion on returned `open_id`.
- Blocked area: `contact +search-user` is currently modeled in a user-only workflow and skipped in bot-only CI.

## Command Table
| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | contact +get-user | shortcut | contact_shortcut_test.go::TestContact_GetUser_BotWorkflow/get-user-by-id-as-bot | `--user-id` | |
| ✕ | contact +search-user | shortcut |  | none | current workflow requires `--as user` and deterministic user-login fixtures |

