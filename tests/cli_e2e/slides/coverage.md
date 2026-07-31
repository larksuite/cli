# Slides CLI E2E Coverage

## Metrics
- Denominator: 5 leaf commands
- Covered: 4
- Coverage: 80.0%

## Summary
- TestSlides_CreateWorkflowAsUser: proves the user slides workflow through `create presentation with slide as user` and `get created presentation xml as user`; creates a fresh presentation, asserts returned IDs, then reads back the XML content to prove the title and slide body persisted.
- TestSlidesAddSlideDryRunE2E / TestSlidesDeleteSlideDryRunE2E: pin the request shapes the unit tests cover, but through the real binary, which is the only layer that proves a full `<slide>` XML document survives flag parsing with its quotes and angle brackets intact. Delete additionally proves the shortcut runs without `--yes`, unlike the high-risk-write raw command.
- TestSlides_SlideAddDeleteWorkflowAsUser: live add/delete round trip on a throwaway presentation created and torn down per run. Asserts against a readback rather than the write's own response, so it proves what the request shape cannot: the returned `slide_id` addresses a real page, `--before-slide-id` positions the page between its neighbours instead of merely being forwarded, and the deleted page is gone while both neighbours survive.
- TestSlidesUpdateSlideDryRunE2E / TestSlidesUpdateAliasDryRunE2E / TestSlidesUpdateSlideRefuses*DryRunE2E: dry-run coverage for `+update-slide` through the built binary — one request carrying one `block_replace` part whose `block_id` is the PAGE id (the whole design; an element id there would replace one element and leave the rest of the page), the `slide` service alias / `+update` command alias / `--xml` spelling, and the two refusals that must not produce a request (a bare element root, and a root id naming a different page).
- TestSlides_HistoryWorkflow: opt-in live round-trip coverage for `+update-slide`; creates a presentation, updates its page in place, asserts the returned `slide_id`, the persisted marker, and that an element written back with its original id keeps that id, then reverts through slide history and self-cleans. It runs only when `LARK_SLIDES_HISTORY_E2E=1` and therefore does not yet exercise the default live lane.
- **Known gap**: the default live lane cannot exercise `+update-slide` until it carries a backend that accepts the page's own id as `block_id`; until then, dry-run is the default-CI proof and the opt-in history workflow is the real-API proof when explicitly enabled. Once rollout completes, add a dedicated live workflow covering restyle / no-op / insert / delete / background and asserting the control page and deck order did not move.
- Blocked area: `slides +media-upload` is still uncovered because it needs a deterministic local image fixture plus XML follow-up proof that is separate from the base create/read workflow.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | slides +create | shortcut | slides_create_workflow_test.go::TestSlides_CreateWorkflowAsUser/create presentation with slide as user | `--title`; `--slides ["<slide ...>"]` | read back through raw slides API to prove persisted XML |
| ✓ | slides +add-slide | shortcut | slides_slide_add_delete_dryrun_test.go::TestSlidesAddSlideDryRunE2E; slides_slide_add_delete_workflow_test.go::TestSlides_SlideAddDeleteWorkflowAsUser | `--slide "<slide ...>"`; `--before-slide-id`; `--revision-id` | live append and insert both verified by reading the deck back |
| ✓ | slides +delete-slide | shortcut | slides_slide_add_delete_dryrun_test.go::TestSlidesDeleteSlideDryRunE2E, TestSlidesDeleteSlideWikiDryRunE2E; slides_slide_add_delete_workflow_test.go::TestSlides_SlideAddDeleteWorkflowAsUser | `--slide-id`; `--revision-id`; wiki URL | live delete runs on a throwaway deck; readback proves the neighbours survive |
| ◐ | slides +update-slide | shortcut | slides_update_slide_dryrun_test.go::TestSlidesUpdateSlideDryRunE2E (+ alias / refusal cases); slides_history_workflow_test.go::TestSlides_HistoryWorkflow (opt-in live) | `--presentation`; `--slide-id`; `--content "<slide ...>"`; `--revision-id` | default CI is dry-run only; opt-in history workflow proves a live in-place update and persisted content |
| ✕ | slides +media-upload | shortcut |  | none | needs a stable local image fixture plus follow-up slide XML proof |
