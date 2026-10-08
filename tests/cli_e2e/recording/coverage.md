# Recording CLI E2E Coverage

## Metrics
- Denominator: 1 leaf command (`recording --help`).
- Covered: 1 command has dry-run and read-only live assertions.
- Coverage: 100% command coverage; live execution requires a user token with `vc:record:readonly`.

## Summary
- `TestRecordingListDryRun`: standalone registration, exact millisecond conversion, cursor forwarding, user identity, bot rejection, and help.
- `TestRecordingListLive`: fetch up to two pages; assert envelope, string identifiers, integer artifact types, and resume when more data exists. Empty accounts are valid; no remote state is created.
- Nonempty artifact fixtures, inclusive date bounds, capped/exhausted pagination, malformed responses and second-page API failures are covered by `shortcuts/recording/list_test.go`. The CLI cannot create recording fixtures, so the live test does not require preexisting recordings or assert their content.

## Command Table
| Status | Cmd | Type | Testcase | Key parameter shapes | Notes |
| --- | --- | --- | --- | --- | --- |
| ✓ | recording +list | shortcut | TestRecordingListDryRun; TestRecordingListLive | dates; page-token; page-all; user/bot | Read-only; live requires user authorization. |
