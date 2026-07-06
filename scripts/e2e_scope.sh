#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

ROOT="${E2E_SCOPE_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT"

module_path="$(go list -m)"
e2e_prefix="${module_path}/tests/cli_e2e"

all_live_packages() {
  go list ./tests/cli_e2e/... |
    grep -v "^${e2e_prefix}$" |
    grep -v '/demo$'
}

domain_exists() {
  local domain="$1"
  go list "./tests/cli_e2e/${domain}" >/dev/null 2>&1
}

append_unique_line() {
  local value="$1"
  local current="$2"
  if [ -z "$value" ]; then
    printf '%s' "$current"
    return
  fi
  if printf '%s\n' "$current" | grep -Fxq "$value"; then
    printf '%s' "$current"
    return
  fi
  if [ -z "$current" ]; then
    printf '%s' "$value"
  else
    printf '%s\n%s' "$current" "$value"
  fi
}

domains=""
force_full_reason=""
matched_relevant=false

add_domain() {
  local domain="$1"
  case "$domain" in
    doc|docs|lark-doc)
      domains="$(append_unique_line doc "$domains")"
      domains="$(append_unique_line docs "$domains")"
      ;;
    drive|lark-drive)
      domains="$(append_unique_line drive "$domains")"
      ;;
    sheets|lark-sheets)
      domains="$(append_unique_line sheets "$domains")"
      ;;
    wiki|lark-wiki)
      domains="$(append_unique_line wiki "$domains")"
      ;;
    base|lark-base)
      domains="$(append_unique_line base "$domains")"
      ;;
    im|lark-im)
      domains="$(append_unique_line im "$domains")"
      ;;
    vc|lark-vc)
      domains="$(append_unique_line vc "$domains")"
      ;;
    calendar|lark-calendar)
      domains="$(append_unique_line calendar "$domains")"
      ;;
    task|lark-task)
      domains="$(append_unique_line task "$domains")"
      ;;
    contact|lark-contact)
      domains="$(append_unique_line contact "$domains")"
      ;;
    mail|lark-mail)
      domains="$(append_unique_line mail "$domains")"
      ;;
    minutes|lark-minutes)
      domains="$(append_unique_line minutes "$domains")"
      ;;
    okr|lark-okr)
      domains="$(append_unique_line okr "$domains")"
      ;;
    slides|lark-slides)
      domains="$(append_unique_line slides "$domains")"
      ;;
    apps|lark-apps)
      domains="$(append_unique_line apps "$domains")"
      ;;
    markdown|lark-markdown)
      domains="$(append_unique_line markdown "$domains")"
      ;;
    note|lark-note)
      domains="$(append_unique_line note "$domains")"
      ;;
    event|lark-event)
      domains="$(append_unique_line event "$domains")"
      ;;
    config)
      domains="$(append_unique_line config "$domains")"
      ;;
    *)
      force_full_reason="unmapped domain path: ${domain}"
      ;;
  esac
}

mark_full() {
  if [ -z "$force_full_reason" ]; then
    force_full_reason="$1"
  fi
}

is_docs_only_path() {
  case "$1" in
    README*|CHANGELOG*|LICENSE|CLA.md|docs/*|.changeset/*|*.md|*.mdx|*.txt|*.rst)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

classify_path() {
  local path="${1#./}"
  [ -z "$path" ] && return

  case "$path" in
    tests/cli_e2e/*/*)
      local rest="${path#tests/cli_e2e/}"
      local domain="${rest%%/*}"
      if [ "$domain" = "demo" ]; then
        return
      fi
      if domain_exists "$domain"; then
        matched_relevant=true
        add_domain "$domain"
        return
      fi
      if is_docs_only_path "$path"; then
        return
      fi
      mark_full "unknown CLI E2E domain path: ${path}"
      return
      ;;
    tests/cli_e2e/*)
      mark_full "shared CLI E2E harness changed: ${path}"
      return
      ;;
    shortcuts/common/*|cmd/*|internal/*|pkg/*|extension/*|registry/*|go.mod|go.sum|Makefile|.github/workflows/*|scripts/*)
      mark_full "shared/runtime path changed: ${path}"
      return
      ;;
    shortcuts/*/*)
      local rest="${path#shortcuts/}"
      matched_relevant=true
      add_domain "${rest%%/*}"
      return
      ;;
    skills/lark-*/*)
      local rest="${path#skills/}"
      matched_relevant=true
      add_domain "${rest%%/*}"
      return
      ;;
  esac

  if is_docs_only_path "$path"; then
    return
  fi

  mark_full "unclassified path changed: ${path}"
}

changed_files() {
  if [ -n "${E2E_SCOPE_CHANGED_FILES:-}" ]; then
    cat "$E2E_SCOPE_CHANGED_FILES"
    return
  fi

  if [ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]; then
    return 1
  fi

  local base_ref="${GITHUB_BASE_REF:-main}"
  if git rev-parse --verify "origin/${base_ref}" >/dev/null 2>&1; then
    git diff --name-only "origin/${base_ref}...HEAD"
    return
  fi

  return 1
}

if ! files="$(changed_files)"; then
  mode="full"
  reason="non-pull_request run or unavailable diff"
  domains="all"
  packages="$(all_live_packages | paste -sd' ' -)"
else
  while IFS= read -r file; do
    classify_path "$file"
  done <<<"$files"

  if [ -n "$force_full_reason" ]; then
    mode="full"
    reason="$force_full_reason"
    domains="all"
    packages="$(all_live_packages | paste -sd' ' -)"
  elif [ -n "$domains" ] && [ "$matched_relevant" = true ]; then
    mode="subset"
    reason="business domain scoped changes"
    domains="$(printf '%s\n' "$domains" | sort -u)"
    packages="$(
      for domain in $domains; do
        if domain_exists "$domain"; then
          printf '%s/tests/cli_e2e/%s\n' "$module_path" "$domain"
        fi
      done | sort -u | paste -sd' ' -
    )"
  else
    mode="skip"
    reason="docs-only or no live CLI E2E impact"
    domains=""
    packages=""
  fi
fi

domains_line="$(printf '%s\n' "$domains" | paste -sd',' -)"

emit() {
  local key="$1"
  local value="$2"
  printf '%s=%s\n' "$key" "$value"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf '%s=%s\n' "$key" "$value" >>"$GITHUB_OUTPUT"
  fi
}

emit mode "$mode"
emit reason "$reason"
emit domains "$domains_line"
emit dry_packages "$packages"
emit live_packages "$packages"
