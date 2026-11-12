# OKR CLI E2E Coverage

## Metrics

- Denominator: 47 leaf commands, enumerated from the compiled CLI help.
- Covered: 24 commands (dry-run coverage included).
- Coverage: 51.1%. Live execution requires the fixtures below.

## Summary

- `TestOKR_DraftFlagsDryRun`: default and explicit draft/published flags for reads, O/KR creation, batch creation, patch, indicators, and KR weights.
- `TestOKR_PatchDraftScoreRejectedDryRun`: typed validation errors for O/KR scores in draft mode. Existing score dry-runs explicitly use published mode.
- `TestOKR_DraftActionsDryRun`: POST paths for publishing and discarding an O draft.
- `TestOKR_DraftLifecycleLive`: bot creates a draft-only O/KR, verifies published-view absence, publishes, edits both drafts, verifies isolation, discards, and edits published content. Deletes the created O during cleanup.
- Existing `TestOKR_CreateLive*`, `TestOKR_BatchCreateLive`, `TestOKR_ReorderLive`, and `TestOKR_WeightLive` retain published creation/read semantics using explicit flags; they require user credentials and `OKR_TEST_CYCLE_ID`.
- `TestOKR_CycleListDryRun*`, `TestOKR_CycleDetailDryRun*`, `TestOKR_Progress*DryRun*`, and `TestOKR_UploadImageDryRun*`: read, pagination, rich-text, and progress request shapes.
- `TestOKR_Comment*DryRun*` and `TestOKR_CommentLifecycleLive`: comment request shapes and create/read/update/delete lifecycle.
- Live blockers: the draft lifecycle requires bot credentials with OKR read/write access to `OKR_TEST_DRAFT_CYCLE_ID` and deployed draft APIs. This separate fixture keeps existing published live tests runnable before draft APIs are deployed. Existing weight/reorder tests need a dedicated test cycle and enabled weight settings. Live indicator edits need a configured numeric indicator; alignment tests need multiple owners with overlapping cycles. Permission filtering and these prerequisite-heavy live paths remain unproven here.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | `okr +batch-create` | shortcut | okr_shortcuts_test.go::TestOKR_BatchCreateDryRun | O/KR request shape; draft flags where supported |  |
| ✓ | `okr +comment-create` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-delete` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-detail` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-get` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-list` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-patch` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-reopen` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +comment-solve` | shortcut | okr_comments_test.go::TestOKR_CommentLifecycleLive | comment lifecycle; deterministic dry-run |  |
| ✓ | `okr +create` | shortcut | okr_shortcuts_test.go::TestOKR_CreateDryRun | O/KR request shape; draft flags where supported |  |
| ✓ | `okr +cycle-detail` | shortcut | okr_drafts_test.go::TestOKR_DraftFlagsDryRun | default draft; explicit true/false |  |
| ✓ | `okr +cycle-list` | shortcut | okr_cycle_list_test.go::TestOKR_CycleListDryRun | user, time range, pagination |  |
| ✓ | `okr +drop-draft` | shortcut | okr_drafts_test.go::TestOKR_DraftLifecycleLive | publish and discard; O/KR read-after-write |  |
| ✓ | `okr +indicator-update` | shortcut | okr_drafts_test.go::TestOKR_DraftFlagsDryRun | default draft; explicit true/false; score rejection (+patch) |  |
| ✓ | `okr +patch` | shortcut | okr_drafts_test.go::TestOKR_DraftFlagsDryRun | default draft; explicit true/false; score rejection (+patch) |  |
| ✓ | `okr +progress-create` | shortcut | okr_progress_test.go::TestOKR_ProgressCreateDryRun | request shape; style and target variants |  |
| ✓ | `okr +progress-delete` | shortcut | okr_progress_test.go::TestOKR_ProgressDeleteDryRun | request shape; style and target variants |  |
| ✓ | `okr +progress-get` | shortcut | okr_progress_test.go::TestOKR_ProgressGetDryRun | request shape; style and target variants |  |
| ✓ | `okr +progress-list` | shortcut | okr_progress_test.go::TestOKR_ProgressListDryRun | request shape; style and target variants |  |
| ✓ | `okr +progress-update` | shortcut | okr_progress_test.go::TestOKR_ProgressUpdateDryRun | request shape; style and target variants |  |
| ✓ | `okr +publish-draft` | shortcut | okr_drafts_test.go::TestOKR_DraftLifecycleLive | publish and discard; O/KR read-after-write |  |
| ✓ | `okr +reorder` | shortcut | okr_shortcuts_test.go::TestOKR_ReorderDryRun | O/KR request shape; draft flags where supported |  |
| ✓ | `okr +upload-image` | shortcut | okr_progress_test.go::TestOKR_UploadImageDryRun | request shape; style and target variants |  |
| ✓ | `okr +weight` | shortcut | okr_shortcuts_test.go::TestOKR_WeightDryRun | O/KR request shape; draft flags where supported |  |
| ✕ | `okr alignments delete` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr alignments get` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr categories list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr cycle.objectives create` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr cycle.objectives list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr cycles list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr cycles objectives_position` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr cycles objectives_weight` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr indicators patch` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr key_result.indicators list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr key_results delete` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr key_results get` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr key_results patch` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objective.alignments create` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objective.alignments list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objective.indicators list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objective.key_results create` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objective.key_results list` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objectives delete` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objectives get` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objectives key_results_position` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objectives key_results_weight` | api |  | none | No primary API workflow; cleanup calls do not count. |
| ✕ | `okr objectives patch` | api |  | none | No primary API workflow; cleanup calls do not count. |
