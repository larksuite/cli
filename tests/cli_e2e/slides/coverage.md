# Slides CLI E2E Coverage

## Metrics
- Denominator: 4 leaf commands
- Covered: 3
- Coverage: 75.0%

## Summary
- TestSlides_CreateWorkflowAsUser: proves the user slides workflow through `create presentation with slide as user` and `get created presentation xml as user`; creates a fresh presentation, asserts returned IDs, then reads back the XML content to prove the title and slide body persisted.
- TestSlidesAddSlideDryRunE2E / TestSlidesDeleteSlideDryRunE2E: pin the request shapes the unit tests cover, but through the real binary, which is the only layer that proves a full `<slide>` XML document survives flag parsing with its quotes and angle brackets intact. Delete additionally proves the shortcut runs without `--yes`, unlike the high-risk-write raw command.
- TestSlides_SlideAddDeleteWorkflowAsUser: live add/delete round trip on a throwaway presentation created and torn down per run. Asserts against a readback rather than the write's own response, so it proves what the request shape cannot: the returned `slide_id` addresses a real page, `--before-slide-id` positions the page between its neighbours instead of merely being forwarded, and the deleted page is gone while both neighbours survive.
- Blocked area: `slides +media-upload` is still uncovered because it needs a deterministic local image fixture plus XML follow-up proof that is separate from the base create/read workflow.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | slides +create | shortcut | slides_create_workflow_test.go::TestSlides_CreateWorkflowAsUser/create presentation with slide as user | `--title`; `--slides ["<slide ...>"]` | read back through raw slides API to prove persisted XML |
| ✓ | slides +add-slide | shortcut | slides_slide_add_delete_dryrun_test.go::TestSlidesAddSlideDryRunE2E; slides_slide_add_delete_workflow_test.go::TestSlides_SlideAddDeleteWorkflowAsUser | `--slide "<slide ...>"`; `--before-slide-id`; `--revision-id` | live append and insert both verified by reading the deck back |
| ✓ | slides +delete-slide | shortcut | slides_slide_add_delete_dryrun_test.go::TestSlidesDeleteSlideDryRunE2E, TestSlidesDeleteSlideWikiDryRunE2E; slides_slide_add_delete_workflow_test.go::TestSlides_SlideAddDeleteWorkflowAsUser | `--slide-id`; `--revision-id`; wiki URL | live delete runs on a throwaway deck; readback proves the neighbours survive |
| ✕ | slides +media-upload | shortcut |  | none | needs a stable local image fixture plus follow-up slide XML proof |
