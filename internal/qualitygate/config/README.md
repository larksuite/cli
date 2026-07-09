# CLI Quality Gate

The quality gate protects machine-facing CLI contracts:

- command and flag naming
- Skills command references
- executable examples under `--dry-run`
- default-output facts for semantic review
- command boundary error contracts

Actions:

- `REJECT` fails CI.
- `LABEL` asks for taxonomy or compatibility review.
- `WARNING` is reviewer signal only.

Local run:

```bash
make quality-gate
```

`make quality-gate` first exports two local command snapshots:

- `command-manifest.json` covers hand-authored commands used by naming and default-output rules.
- `command-index.json` covers the full command surface, including generated service commands used by reference and dry-run validation.

CI uploads only `facts.json`; command snapshots are local inputs and are not published as workflow artifacts.

## Legacy Naming Allowlists

`internal/qualitygate/config/allowlists/legacy-commands.txt` and `internal/qualitygate/config/allowlists/legacy-flags.txt` are compatibility records for historical hand-authored public command and flag names that cannot be renamed immediately.

Each non-comment row must include owner, reason, and `added_at`:

```text
# command	owner	reason	added_at
drive +task_result	cli-owner	legacy public shortcut	2026-06-05

# command	flag	owner	reason	added_at
docs +whiteboard-update	input_format	cli-owner	legacy public flag	2026-06-05
```

Adding a new row requires approval from the matching CODEOWNERS or quality gate owner.

`legacy-commands.txt` only covers hand-authored legacy commands. Generated OpenAPI service commands are intentionally excluded from `command-manifest.json`; they are included in `command-index.json` only so command references can be checked against the real CLI surface.

## Semantic Blocker Policy

The semantic reviewer can propose findings, but the local gatekeeper recomputes whether each finding is reproducible from `facts.json`. A finding blocks only when all of these are true:

- runtime blocking is enabled with repository variable `SEMANTIC_REVIEW_BLOCK=true`;
- the category is listed in `internal/qualitygate/config/semantic/policy.json`;
- every evidence item is reproducible from facts;
- every evidence item matches at least one common rollout group;
- the finding is not covered by active `internal/qualitygate/config/semantic/waivers.txt` rows that share one `waiver_id`.

The initial blocking rollout is intentionally narrower than the full policy vocabulary. Today, `changed-only` blocks only `error_hint` and `skill_quality`; `default_output` and `naming` remain observe-first until a later rollout explicitly enables them.

| Category | Required evidence | Blocks when enabled by rollout |
|---|---|---|
| `error_hint` | `facts.errors[n]` | `required_hint=true` and `hint_action_count=0` |
| `default_output` | `facts.outputs[n]` | list command lacks a default limit or decision fields |
| `naming` | `facts.commands[n]` | new hand-authored command or flag conflicts with the naming contract; `legacy_naming=true` never blocks |
| `skill_quality` | `facts.skills[n]` | invalid command reference |

Evidence that is missing, out of range, the wrong fact kind, or not reproducible is downgraded to a warning.

The `skill_quality` category intentionally uses `facts.skills[n]` evidence. `facts.skill_quality[n]` is currently a document-statistics fact and is not v1 blocker evidence.

### Semantic Rollout Config

Long-lived policy is in JSON:

- `internal/qualitygate/config/semantic/policy.json` controls blockable categories and rollout groups.
- `internal/qualitygate/config/semantic/models.json` controls allowed model ids and allowed model API base URLs. It does not define a default model; semantic review is skipped unless both `ARK_API_KEY` and `ARK_MODEL` are configured.

Temporary compatibility waivers are in TSV:

```text
# waiver_id	category	fact_kind	source_file	line	command_path	owner	reason	added_at	expires_at
wiki-move-202606	skill_quality	skill	skills/lark-wiki/SKILL.md	30		wiki-owner	migration	2026-06-08	2026-07-15
```

`skill` and `error` waivers must target `source_file + line`. `command` and `output` waivers must target `command_path`. Multi-evidence findings require one waiver row per evidence item, and those rows must share the same `waiver_id`. Expired semantic waivers warn and no longer waive blockers.

## Command Error Contract Rollout

The blocking rule covers command boundary returns only:

- Cobra `RunE` / `Run` function literals.
- Functions directly referenced by `RunE` / `Run`.
- Shortcut `Validate` / `Execute` function literals.
- Functions directly referenced by shortcut `Validate` / `Execute`.

Helper/internal errors are collected as facts or warnings. They are not blocking unless the analyzer proves they are returned directly from a command boundary. Semantic scope fields such as `command_path` and `domain` are filled only when the analyzer can attribute the boundary to a concrete command.

Existing boundary bare errors live in `internal/qualitygate/config/allowlists/legacy-command-errors.txt` with owner, reason, and added_at. New boundary bare errors are rejected.

Useful commands:

```bash
go run -C lint . --changed-from origin/main ..
go run -C lint . --print-legacy-command-error-candidates ..
```

## Branch Protection

The intended required checks are:

- `results` for deterministic gates and existing CI.
- `semantic-review/result` custom check-run only after the semantic workflow is approved for required-check usage.

The semantic workflow starts in comment-only mode and publishes `semantic-review/observe`. Do not make `semantic-review/observe` required: GitHub treats `neutral` and `skipped` conclusions as successful required-check states.

Blocking mode publishes `semantic-review/result`. Do not add the `semantic-review` workflow job name as a required check.

Before `semantic-review/result` becomes required, facts must be regenerated or independently verified by trusted base code, and no other PR-executable workflow may be able to forge the same check name with `checks: write`.

## CI Rollout Test Plan

Use a temporary sandbox repository created from a fork or private test copy. Do not test required checks directly on the production default branch.

| Scenario | Expected result |
|---|---|
| normal branch PR | `results` runs quality gate and uploads `quality-gate-facts-<base_sha>-<head_sha>` |
| fork PR | deterministic gate runs without secrets; semantic workflow uses trusted `workflow_run` only |
| stale PR head after CI | semantic verifier rejects mismatched PR head SHA |
| missing artifact | comment-only publishes `semantic-review/observe=neutral`; blocking publishes `semantic-review/result=failure` |
| multiple artifacts | verifier rejects the run |
| tampered zip path traversal | verifier rejects before reading facts |
| symlink facts entry | verifier rejects before reading facts |
| missing `ARK_API_KEY` or `ARK_MODEL` | comment-only publishes `semantic-review/observe=neutral`; blocking publishes `semantic-review/result=failure` |
| model timeout | `internal/qualitygate/cmd/semantic-review` writes a degraded decision; comment-only publishes `semantic-review/observe=neutral`; blocking publishes `semantic-review/result=failure` |
| blocker fixture with `SEMANTIC_REVIEW_BLOCK=true` | custom check `semantic-review/result` is `failure` on PR head SHA |
| comment-only mode | custom check `semantic-review/observe` is `success` or `neutral`; observe-only findings do not publish a PR comment by default |

Rollout sequence:

1. Run deterministic `results` as required and semantic review in comment-only mode for one week.
2. Track false positives by category, rollout group, and owner.
3. Enable `SEMANTIC_REVIEW_BLOCK=true` only for `changed-only` rollout first.
4. Enable required custom check `semantic-review/result` only after trusted facts, check-name forgery review, merge-queue compatibility, named owners, and false-positive targets are satisfied.
5. Roll back by clearing `SEMANTIC_REVIEW_BLOCK`, removing rollout groups or waivers as needed, and removing `semantic-review/result` from required checks; do not remove `results`.
