# Contact CLI E2E Coverage

## Metrics
- Denominator: 3 leaf commands
- Covered: 2
- Coverage: 66.7%

## Summary
- TestContact_LookupWorkflowAsUser: proves the user lookup workflow through `get self as user` and `get self by open id as user`; reads the current user first and round-trips the returned `open_id` back into `+get-user`.
- TestContact_LookupWorkflowAsBot: proves bot lookup through `discover user via api as bot` and `get user by open id as bot`; the raw API discovery step is fixture setup only and does not affect the domain denominator.
- TestContactSearchBotWorkflowAsUser: proves live bot search as user; validates `bots[]`, `has_more`, and a non-empty bot `open_id`.
- Blocked area: `contact +search-user` did not reliably return the current user in UAT even when queried with self-derived identifiers, so it remains uncovered rather than being counted from a flaky tenant-dependent assertion.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | contact +get-user | shortcut | contact_lookup_workflow_test.go::TestContact_LookupWorkflowAsUser/get self as user; contact_lookup_workflow_test.go::TestContact_LookupWorkflowAsUser/get self by open id as user; contact_lookup_workflow_test.go::TestContact_LookupWorkflowAsBot/get user by open id as bot | self lookup; `--user-id <open_id>` | |
| ✓ | contact +search-bot | shortcut | contact_search_bot_workflow_test.go::TestContactSearchBotWorkflowAsUser | `--query <keyword>`; `--format json`; user identity | |
| ✕ | contact +search-user | shortcut |  | none | UAT did not reliably return the current user for self-derived queries, so stable write-after-read style proof is not available |
