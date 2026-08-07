<!-- knowledge-role: route-entry; knowledge-contract: lark-cli.local-page@1; source-of-truth: current-checkout -->

# Lark CLI Tool Development Entry

This page is the same-checkout navigation entry for Lark CLI development. It
identifies where to verify current behavior; it does not replace source code,
tests, generated metadata, or service-side contracts.

## Development Boundary

- `cmd/` owns command registration and top-level CLI behavior.
- `shortcuts/` owns handwritten workflow commands. Start with
  `shortcuts/register.go`, then inspect the domain module and its tests.
- `internal/registry/meta_data_default.json` is the checked-in metadata snapshot
  consumed by raw API commands. Trace updates through `scripts/fetch_meta.py`;
  do not hand-edit generated metadata as the source of truth.
- `skills/` owns agent-facing operation guidance. A skill reference documents
  the accepted CLI payload, not whether a backend deployment supports it.

## Base Field Shortcuts

For `base +field-create` or `base +field-update`, verify:

1. Command flags and validation in `shortcuts/base/field_create.go` or
   `shortcuts/base/field_update.go`.
2. Request construction and execution in `shortcuts/base/field_ops.go`.
3. Field JSON shape in
   `skills/lark-base/references/lark-base-field-json.md`.
4. Dry-run and behavior tests for the affected field type.

The CLI payload, service-side Agent schema, and backend field model are
different contracts. A field type present in one layer does not prove that the
other layers accept or deploy it.

## Candidate Route Use

The central `lark-cli.tool-dev` Route is a candidate. Use it only for
governance, qualification, or an explicitly selected real-demand trial. During
such a trial, map this checkout as `larksuite-cli` and stop if a required Page,
Source, or local Page contract is unavailable.
