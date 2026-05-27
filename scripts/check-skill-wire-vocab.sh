#!/usr/bin/env bash
# §12.3: forward rule — any errs/ wire-shape change MUST be paired
# with a skills/ grep sweep in the same PR.
set -euo pipefail
PATTERN='"type"\s*:\s*"(auth_error|api_error|infra_error|missing_scope|command_denied|external_provider)"'
if git grep -E "$PATTERN" skills/ >/dev/null 2>&1; then
  echo "[WIRE-VOCAB-DRIFT] skills/ contains legacy wire strings — see spec §12.3" >&2
  git grep -nE "$PATTERN" skills/ >&2
  exit 1
fi
echo "skill wire-vocab clean."
