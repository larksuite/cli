# Note CLI E2E Coverage

## Metrics
- Denominator: 2 leaf commands
- Dry-run covered: 2
- Dry-run coverage: 100.0%
- Live covered: 0
- Live coverage: 0.0%

Live E2E is intentionally not counted yet because both commands require meeting-generated note artifacts; stable create/use/cleanup fixtures are not available in this test suite.

## Summary
- TestNoteDetailDryRun: dry-run coverage for `note +detail`; asserts the detail request method and `/open-apis/vc/v1/notes/{note_id}` URL without calling live APIs.
- TestNoteTranscriptDryRun: dry-run coverage for `note +transcript`; asserts the two-step request shape (`note detail` precheck, then `unified_note_transcript`), transcript query parameters, and that `--transcript-format` coexists with the global `--format` output flag.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| dry-run ✓ / live ✕ | note +detail | shortcut | note_dryrun_test.go::TestNoteDetailDryRun | `--note-id`; user identity | live note fixtures depend on meeting-generated artifacts |
| dry-run ✓ / live ✕ | note +transcript | shortcut | note_dryrun_test.go::TestNoteTranscriptDryRun | `--note-id`; `--transcript-format`; `--format json`; transcript API `format/page_size/locale` params | live unified-note fixtures depend on generated VC note artifacts |
