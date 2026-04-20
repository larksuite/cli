# Slides CLI E2E Coverage

## Metrics
- Denominator: 2 leaf commands
- Covered: 1
- Coverage: 50.0%

## Summary
- TestSlides_CreateWorkflowAsUser: proves the user slides workflow through `create presentation with slide as user` and `get created presentation xml as user`; creates a fresh presentation, asserts returned IDs, then reads back the XML content to prove the title and slide body persisted.
- Blocked area: `slides +media-upload` is still uncovered because it needs a deterministic local image fixture plus XML follow-up proof that is separate from the base create/read workflow.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | slides +create | shortcut | slides_create_workflow_test.go::TestSlides_CreateWorkflowAsUser/create presentation with slide as user | `--title`; `--slides ["<slide ...>"]` | read back through raw slides API to prove persisted XML |
| ✕ | slides +media-upload | shortcut |  | none | needs a stable local image fixture plus follow-up slide XML proof |
