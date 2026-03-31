# Skill Format Check

This directory contains a script to validate the format of `SKILL.md` files located in the `../../skills` directory.

## Purpose

The `index.js` script ensures that all `SKILL.md` files conform to the standard template defined in `skill-template/skill-template.md`. Specifically, it checks that the YAML frontmatter includes the following fields:
- `name` (required)
- `description` (required)
- `metadata` (outputs a warning if missing, does not fail the build)

> **Note:** The `lark-shared` skill is explicitly excluded from these format checks.

## Usage

This script is executed automatically via GitHub Actions (`.github/workflows/skill-format-check.yml`) on pull requests and pushes that modify the `skills/` directory.

To run the check manually from the root of the repository, execute:

```bash
node scripts/skill-format-check/index.js
```

You can also specify a custom target directory as the first argument:

```bash
node scripts/skill-format-check/index.js ./path/to/my/skills
```

## Testing

This tool comes with a quick validation script to ensure it correctly identifies good and bad skill formats. To run the tests, execute:

```bash
./scripts/skill-format-check/test.sh
```
