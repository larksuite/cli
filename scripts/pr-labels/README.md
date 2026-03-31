# PR Label Sync

This directory contains scripts and sample data for automatically classifying and labeling GitHub Pull Requests based on the files they modify.

## Files

- `index.js`: The main Node.js script. It fetches PR files, evaluates their risk level, calculates business impact, and uses GitHub APIs to add appropriate `size/*` and `domain/*` labels.
- `samples.json`: A collection of historical PRs used as test cases to verify the labeling logic (especially for regression testing the S/M/L thresholds).

## Features

### Size Labels (`size/*`)
The script evaluates the "effective" lines of code changed (ignoring tests, docs, and ci files) to classify the PR:
- **`size/S`**: Low-risk changes involving only docs, tests, CI workflows, or chores.
- **`size/M`**: Small-to-medium changes affecting a single business domain, with effective lines under 300.
- **`size/L`**: Large features (>= 300 lines), cross-domain changes, or any changes touching core architecture paths (like `cmd/`).
- **`size/XL`**: Architectural overhauls, extremely large PRs (>1200 lines), or sensitive refactors.

### Domain Tags (`domain/*`)
The script also identifies which business domains a PR touches to give reviewers an immediate sense of the impact scope. Currently tracked domains include:
- `domain/im`
- `domain/vc`
- `domain/ccm`
- `domain/base`
- `domain/mail`
- `domain/calendar`
- `domain/task`
- `domain/contact`

Minor modules like docs and tests are omitted to keep PR tags clean and focused on structural changes.

## Usage

### In GitHub Actions
This script is designed to run in CI workflows. It automatically reads the `GITHUB_EVENT_PATH` payload to get the PR context.

```bash
node scripts/pr-labels/index.js
```

### Local Dry Run
You can test the labeling logic against an existing GitHub PR without actually applying labels by using the `--dry-run` flag.

```bash
# Requires GITHUB_TOKEN environment variable or passing --token
node scripts/pr-labels/index.js --dry-run --repo larksuite/cli --pr-number 123
```

## Testing

A regression test suite is available in `test.js` which verifies the output of the classification logic against historical PRs configured in `samples.json`.

```bash
# Requires GITHUB_TOKEN environment variable to avoid rate limits
GITHUB_TOKEN=$(gh auth token) node scripts/pr-labels/test.js
```

This test suite also runs automatically in CI via `.github/workflows/pr-labels-test.yml` when changes are made to this directory.