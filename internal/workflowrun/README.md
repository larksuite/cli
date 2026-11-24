# Workflow execution log mock preflight

Four read-only shortcuts collect runs, paginated node observations, button
iteration events, and durable exports. Production execution log APIs are not
implemented. All wire names, scopes, IDs, coverage fields, attribution modes,
and compound cursors are contract proposals pending T1–T14. No retention
period is asserted.

```sh
lark-cli base +workflow-run-list --base-token fixture --workflow-id wf1 --start-time 0 --end-time 172800000 --mock-fixture @internal/workflowrun/testdata/basic.json
lark-cli base +workflow-run-get --base-token fixture --workflow-id wf1 --run-id E1 --mock-fixture @internal/workflowrun/testdata/basic.json
lark-cli base +workflow-button-log-list --base-token fixture --workflow-id wf1 --run-id E1 --button-key message_btn_12345678 --mock-fixture @internal/workflowrun/testdata/basic.json
lark-cli base +workflow-run-export --base-token fixture --workflow-id wf1 --start-time 0 --end-time 172800000 --output workflow-export --mock-fixture @internal/workflowrun/testdata/basic.json --require-complete
lark-cli base +workflow-run-export --base-token fixture --resume workflow-export --mock-fixture @internal/workflowrun/testdata/basic.json --require-complete
```

The default mock uses an injected HTTP transport and never opens a socket.
`--mock-url` selects a local HTTP mock server at a literal loopback IP; this
client sends no credentials, uses no proxy, and rejects redirects. A mock server
receives GET requests on the three proposed paths, with a JSON `Query` in the
`query` query-string parameter. `--dry-run` previews these proposed requests
without issuing HTTP calls or writing files. The fixture supports six statuses,
Base aggregation capability, ordered pagination, payload exclusion, three
attribution modes, visibility time, and finite or indefinite faults.

Exports require the local FileIO provider: the portable FileIO contract lacks
fsync, directory walking, atomic rename and directory locking. Runtime resolves
and validates the root; the internal store rejects symlinks, bounds generated
entry names, and serializes writers with a directory lock. Each immutable JSON
batch retains its full page metadata and observations. The batch commit is
`.part -> file fsync -> rename -> parent fsync`, followed by the same protocol
for its checkpoint. Checkpoints retain SHA-256 and full batch metadata.

Resume replays every checkpoint, validates its shard hash and query/cursor
continuity, quarantines incomplete data and unverifiable orphan batches, and
adopts only continuous batches with explicit result and cursor metadata. The
merged NDJSON view is published only after every job for its plane terminates.
Runs and nodes use the newest `(fetched_at, checkpoint seq)` observation; button
events deduplicate by event ID. Immutable source observations remain intact.

The manifest freezes parameters, absolute instance-selection windows, selected
run IDs and button collection bounds. Click ranges start at each selected run's
start and end at that run's frozen `collected_at`, independently of selection
window end. Resume only fills this directory's gaps. New directories reevaluate
mutable filters and collect later clicks. There is no sync, refresh, frozen-ID
backfill or analysis command. Per-recipient delivery/read results are explicitly
unavailable.

`complete` requires all jobs to terminate, no pending work, guaranteed coverage
in every window, exact button cursor capability and a declared attribution mode,
including the separate legacy group. Missing coverage stays unknown. Expired
windows record uncovered intervals and are terminal, so resume does not retry
them. Permission and request failures remain pending.

Batch failures remain on stdout as `ok:false` with the full manifest/result.
Outcome exit codes are 7 for record/server truncation, 3 for permission denied,
4 for request failure/cancellation, and 1 for incomplete coverage when
`--require-complete` is requested. Parameter conflicts use the standard typed
validation error and exit 2. The JSON result carries stable reason codes even
when permissive collection exits 0 with partial coverage.

Tests: `go test ./internal/workflowrun ./shortcuts/base`. The table-driven
`TestMockAcceptance/AC01_*` through `AC20_*` covers the preflight acceptance
matrix; `TestHardKillResume` additionally exercises actual process termination
on both sides of checkpoint commit. Shortcut tests exercise dry-run requests,
mock HTTP, stdout envelopes, jq, record-budget resume, selectors and exit codes.
