# Slides CLI E2E Coverage

## Metrics
- Denominator: 3 leaf commands
- Covered: 2
- Coverage: 66.7%

## Summary
- TestSlides_CreateWorkflowAsUser: proves the user slides workflow through `create presentation with slide as user` and `get created presentation xml as user`; creates a fresh presentation, asserts returned IDs, then reads back the XML content to prove the title and slide body persisted.
- TestSlides_UpdateSlideWorkflowAsUser: proves `+update-slide` end to end on a two-page deck — restyle an element (one replace part), rewrite the same page (no-op, no request), add an element without an id (insert), drop an element (delete), and confirm a background change is refused with the page left untouched; then checks the control page and the deck order did not move. **This test is why the command works**: the first version of the command sent a single part covering the whole page, which HTTP stubs accepted and the real API rejects outright — `ReplacePart.block_id` is validated as a short ELEMENT id, so neither the page id (`p`-prefixed) nor the background fill id (`f`-prefixed) can be addressed. Stubs prove the request shape; only a live round trip proves the request is legal.
- TestSlidesUpdateSlideDryRunE2E / TestSlidesUpdateAliasDryRunE2E / TestSlidesUpdateSlideRejectsElementRootDryRunE2E: dry-run coverage for the read-then-write orchestration, the shared revision and slide_id on both calls, the hidden `slides +update` alias with the hidden `--token` / `--xml` spellings, and the refusal of a non-`<slide>` root before any request is built.
- **Known gap**: the live workflow test skips without a user token, so a CI run configured only with bot credentials leaves the load-bearing backend behavior unverified — exactly the blind spot that let the original design reach review. A CI user token, or a bot-identity variant of this workflow, would close it.
- Cleanup deletes the deck through `drive +delete`, which needs `space:document:delete` and `drive:drive.metadata:readonly`. For **stored credentials** the workflow probes that capability up front with a `--dry-run` delete (whose scope pre-flight reads the stored grants) and skips before creating anything when they are missing; an unexpected probe failure is fatal. For **environment tokens** (`TEST_USER_ACCESS_TOKEN`) no scope metadata exists and no API exposes a token's grants without exercising them, so the probe proves nothing there — the CI identity must be provisioned with the cleanup scopes, and a cleanup failure stays fatal and visible. A fully-scoped run creates, edits and deletes its own deck (verified green end to end).
- Blocked area: `slides +media-upload` is still uncovered because it needs a deterministic local image fixture plus XML follow-up proof that is separate from the base create/read workflow.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | slides +create | shortcut | slides_create_workflow_test.go::TestSlides_CreateWorkflowAsUser/create presentation with slide as user | `--title`; `--slides ["<slide ...>"]` | read back through raw slides API to prove persisted XML |
| ✓ | slides +update-slide | shortcut | slides_update_slide_workflow_test.go::TestSlides_UpdateSlideWorkflowAsUser | `--presentation`; `--slide-id`; `--content "<slide ...>"`; `--revision-id` | live run needs a user token carrying `slides:presentation:create` / `read` / `update` / `write_only`, plus `space:document:delete` for cleanup; the dry-run half needs no secrets |
| ✕ | slides +media-upload | shortcut |  | none | needs a stable local image fixture plus follow-up slide XML proof |
