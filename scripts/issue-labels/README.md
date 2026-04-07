# Issue Labels

This script searches unlabeled GitHub issues in a repository and applies labels based on heuristics. It is intentionally a one-shot triage pass: once any label is added to an issue, that issue is out of scope for future scheduled runs.

It only covers two label dimensions:

- **Type**: `bug` / `enhancement` / `question` / `documentation` / `performance` / `security`
- **Domain**: `domain/<service>` (multi-select)

Related GitHub Actions workflow: `.github/workflows/issue-labels.yml`.

## Labeling Rules (Current)

### Type (single-select; write only when matched)

- Candidates: `bug`, `enhancement`, `question`, `documentation`, `performance`, `security`
- Type is written **only when keywords are matched** in title/body. If nothing matches, the script will not add or correct type labels.
- By default, the script **does not override existing type labels** to avoid reverting manual triage. Use `--override-type` if you really want the script to enforce the computed type.

### Domain (multi-select; add-only by default)

- Managed labels prerequisite: the standard type labels plus `domain/<service>` labels should exist in the repository. If a specific issue needs a managed label that is missing, the script prints a warning, skips that issue, and continues processing the rest.
- Label format: `domain/<service>` (e.g. `domain/base`, `domain/im`)
- Signals (strong → weak):
  1) Explicit `domain/<service>` in text
  2) Command mention: `lark-cli <service>` / `lark cli <service>` (maps `docs` → `doc`)
  3) Loose title match (careful; excludes English `im` to reduce false positives)
  4) A small set of conservative keyword heuristics as fallback
- By default, the script only adds missing domain labels and never removes existing ones.
- If you want stricter domain synchronization, use `--sync-domains`.
- Note: the current implementation only removes existing `domain/*` labels when it can positively match at least one domain for the issue, so this is not an exact-sync cleanup mode.

## Usage

### GitHub Actions (recommended)

The workflow supports both:

- `schedule` (hourly)
- `workflow_dispatch` (manual run)

Scheduled runs write labels by default. Manual runs default to dry-run unless `dry_run=false` is selected.

Only issues with no labels are scanned. This is intentional: the automation is meant to triage brand-new unlabeled issues once, not to continuously reconcile labels on previously triaged issues.

### Local dry-run

Provide a token to avoid anonymous rate limits:

```bash
GITHUB_TOKEN=$(gh auth token) \
  node scripts/issue-labels/index.js \
  --repo larksuite/cli \
  --max-issues 100 \
  --dry-run --json
```

### Common Flags

- `--dry-run`: Do not write labels, only print planned changes
- `--json`: JSON output (usually with `--dry-run`)
- `--max-issues <n>` / `--max-pages <n>`: Bound unlabeled-issue search size for each run
- `--sync-domains`: Stricter `domain/*` sync when at least one managed domain matches (may still leave stale labels if nothing matches)
- `--override-type`: Allow overriding existing type labels (use with caution)

## Regression Samples

`samples.json` is a regression dataset sampled from real issues in `larksuite/cli` (issue bodies are truncated).

Run tests:

```bash
node scripts/issue-labels/test.js
```
