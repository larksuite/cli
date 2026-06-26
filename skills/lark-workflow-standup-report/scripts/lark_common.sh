#!/usr/bin/env bash
set -euo pipefail

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

json_count() {
  local file=$1
  local expr=$2
  jq -r "$expr | if type == \"array\" then length elif . == null then 0 else 1 end" "$file" 2>/dev/null || printf '0\n'
}

json_get() {
  local file=$1
  local expr=$2
  jq -r "$expr // empty" "$file" 2>/dev/null || true
}

run_lark_json() {
  local outfile=$1
  local timeout_seconds=$2
  shift 2
  mkdir -p "$(dirname "$outfile")"

  local tmp
  tmp="${outfile}.tmp"
  if command -v timeout >/dev/null 2>&1; then
    timeout "$timeout_seconds" lark-cli "$@" >"$tmp" 2>&1 || {
      local status=$?
      mv "$tmp" "$outfile"
      return "$status"
    }
  else
    lark-cli "$@" >"$tmp" 2>&1 || {
      local status=$?
      mv "$tmp" "$outfile"
      return "$status"
    }
  fi

  mv "$tmp" "$outfile"
  if jq -e '.ok == false' "$outfile" >/dev/null 2>&1; then
    return 1
  fi
}

is_auth_error_file() {
  local file=$1
  jq -e '
    (.error.message // "" | test("not logged in|auth login|keychain|permission|authorize"; "i")) or
    (.error.type // "" | test("auth|authentication"; "i"))
  ' "$file" >/dev/null 2>&1
}

redact_file() {
  local file=$1
  perl -0pi \
    -e 's/(Key:\s*)[A-Za-z0-9+\/=_-]{16,}/$1[REDACTED]/gi' \
    -e 's/(api[_ -]?key["'\''\s:=]+)[A-Za-z0-9+\/=_-]{12,}/$1[REDACTED]/gi' \
    -e 's/((?:access|refresh|tenant|user|app|authorization)[_-]?token["'\''\s:=]+)[A-Za-z0-9._+\/=_-]{16,}/$1[REDACTED]/gi' \
    -e 's/(secret["'\''\s:=]+)[A-Za-z0-9._+\/=_-]{12,}/$1[REDACTED]/gi' \
    -e 's/(password["'\''\s:=]+)[^\s,'\''"}]{6,}/$1[REDACTED]/gi' \
    -e 's/(private[-_ ]?key["'\''\s:=]+)[A-Za-z0-9._+\/=_-]{16,}/$1[REDACTED]/gi' \
    "$file" 2>/dev/null || true
}

redact_json_dir() {
  local dir=$1
  find "$dir" -maxdepth 1 -type f -name '*.json' -print0 2>/dev/null | while IFS= read -r -d '' file; do
    redact_file "$file"
  done
}

manifest_init() {
  local file=$1 date=$2 start=$3 end=$4 dry_run=${5:-false}
  jq -n \
    --arg date "$date" \
    --arg start "$start" \
    --arg end "$end" \
    --arg generated_at "$(date '+%Y-%m-%d %H:%M:%S %z')" \
    --argjson dry_run "$dry_run" \
    '{date:$date,start:$start,end:$end,generated_at:$generated_at,dry_run:$dry_run,files:{},counts:{},errors:[]}' >"$file"
}

manifest_set_file() {
  local manifest=$1 key=$2 value=$3
  jq --arg key "$key" --arg value "$value" '.files[$key]=$value' "$manifest" >"${manifest}.tmp"
  mv "${manifest}.tmp" "$manifest"
}

manifest_set_files_array() {
  local manifest=$1 key=$2
  shift 2
  jq --arg key "$key" --args '.files[$key]=$ARGS.positional' "$manifest" "$@" >"${manifest}.tmp"
  mv "${manifest}.tmp" "$manifest"
}

manifest_set_count() {
  local manifest=$1 key=$2 value=$3
  jq --arg key "$key" --argjson value "${value:-0}" '.counts[$key]=$value' "$manifest" >"${manifest}.tmp"
  mv "${manifest}.tmp" "$manifest"
}

manifest_set_value() {
  local manifest=$1 key=$2 value=$3
  jq --arg key "$key" --arg value "$value" '.[$key]=$value' "$manifest" >"${manifest}.tmp"
  mv "${manifest}.tmp" "$manifest"
}

manifest_add_error() {
  local manifest=$1 source=$2 message=$3
  jq --arg source "$source" --arg message "$message" '.errors += [{source:$source,message:$message}]' "$manifest" >"${manifest}.tmp"
  mv "${manifest}.tmp" "$manifest"
}

limit_text() {
  local max=${2:-240}
  python3 - "$max" <<'PY'
import re, sys
max_len = int(sys.argv[1])
text = sys.stdin.read()
text = re.sub(r"\s+", " ", text).strip()
print(text if len(text) <= max_len else text[:max_len] + "...")
PY
}

iso_to_epoch() {
  python3 - "$1" <<'PY'
from datetime import datetime
import sys
s=sys.argv[1]
if s.endswith('Z'):
    s=s[:-1] + '+00:00'
print(datetime.fromisoformat(s).timestamp())
PY
}
