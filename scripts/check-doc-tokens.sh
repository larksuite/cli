#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
#
# check-doc-tokens.sh
#
# Scans skill reference docs for token-like values that look realistic but
# are not using the required placeholder format (*_EXAMPLE_TOKEN or similar).
#
# Real token patterns (Lark API) often look like:
#   wikcnXXXXXXXXX  doccnXXXXXXX  shtcnXXX  fldcnXXX  ou_XXXX  cli_XXXX
#
# Docs MUST use clearly fake placeholders, e.g.:
#   wikcn_EXAMPLE_TOKEN   doccn_EXAMPLE_TOKEN   <space_id>   your_token_here
#
# If this check fails, replace the realistic-looking value with a placeholder
# like `wikcn_EXAMPLE_TOKEN` so gitleaks CI won't flag it as a real secret.

set -euo pipefail

SKILLS_DIR="${1:-skills}"
ERRORS=0

# Patterns that indicate a realistic-looking Lark token value inside a string.
# Matches JSON-style: "field": "token_value" or markdown backtick spans.
# Token prefixes used by Lark Open Platform:
#   wikcn  doccn  docx  shtcn  bascn  fldcn  vewcn  tbln  ou_  cli_  obcn  flec
#
# Excluded (clearly fake):
#   - Values ending with EXAMPLE_TOKEN  (e.g. wikcn_EXAMPLE_TOKEN)
#   - Values that are all uppercase X   (e.g. bascnXXXXXXXX)
#   - Values containing only X/_/<>     (e.g. <your_token>)
REALISTIC_TOKEN_RE='"(wikcn|doccn|docx[a-z]|shtcn|bascn|fldcn|vewcn|tbln|obcn|flec|ou_|cli_)[A-Za-z0-9]{6,}"|`(wikcn|doccn|docx[a-z]|shtcn|bascn|fldcn|vewcn|tbln|obcn|flec|ou_|cli_)[A-Za-z0-9]{6,}`'
PLACEHOLDER_RE='(EXAMPLE|_TOKEN|XXXX|xxxx|<|>|your_|_here)'

while IFS= read -r -d '' file; do
  # grep returns exit 1 when no match — use || true to avoid set -e killing us
  # Then filter out values that are clearly placeholders (EXAMPLE, XXXX, etc.)
  matches=$(grep -nEo "$REALISTIC_TOKEN_RE" "$file" 2>/dev/null | grep -vE "$PLACEHOLDER_RE" || true)
  if [[ -n "$matches" ]]; then
    echo ""
    echo "❌  $file"
    echo "    Contains realistic-looking token values that may trigger gitleaks:"
    while IFS= read -r line; do
      echo "      $line"
    done <<< "$matches"
    echo "    → Replace with a placeholder, e.g.: wikcn_EXAMPLE_TOKEN, doccn_EXAMPLE_TOKEN"
    ERRORS=$((ERRORS + 1))
  fi
done < <(find "$SKILLS_DIR" -path "*/references/*.md" -print0)

if [[ $ERRORS -gt 0 ]]; then
  echo ""
  echo "❌  check-doc-tokens: $ERRORS file(s) contain realistic token values in reference docs."
  echo "    Use _EXAMPLE_TOKEN placeholders to avoid false positives in gitleaks CI."
  exit 1
else
  echo "✅  check-doc-tokens: all reference docs use safe placeholder tokens."
fi
