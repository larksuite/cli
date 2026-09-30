#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
#
# check-doc-tokens.sh
#
# Scans skill reference docs for token-like values that look realistic but are
# not using the required placeholder format (*_EXAMPLE_TOKEN or similar).
#
# Docs MUST use clearly fake placeholders, e.g.:
#   wikcn_EXAMPLE_TOKEN   doccn_EXAMPLE_TOKEN   <space_id>   your_token_here
#
# If this check fails, replace the realistic-looking value with a placeholder
# so gitleaks CI won't flag it as a real secret.
#
# Three rules run over every reference doc. Rule 1 alone used to be the whole
# check; measured against tests/fixtures/doc-tokens on 2026-08-31 it caught 3 of
# 6 realistic values and raised one false positive, so rules 2 and 3 were added
# and rule 1 was tightened.

set -euo pipefail

SKILLS_DIR="${1:-skills}"
ERRORS=0

# Markers that make a value unmistakably fake wherever they appear in it.
# Compared against a lowercased value, so the same stub is caught however these
# docs capitalise it -- a case-sensitive list let "page_token":"example_page_token"
# through. 550e8400-... is the RFC 4122 example UUID, which these docs use
# verbatim as a sample id.
PLACEHOLDER_LC='(example|_token|<|>|your_|_here|1234567890|550e8400-e29b-41d4-a716-446655440000)'

# A run of x's or y's is different: it stands in for a redacted tail, so it only
# makes the value fake if it is most of the value. Accepting it anywhere let a
# 29-character page_token in lark-im-reactions.md through on the four x's glued
# to its end -- gitleaks reported that one at entropy 4.6 while this check
# called the file clean. What is left after the runs are removed must be a short
# prefix such as `tbl`, `ou_` or `MAGOb`; anything longer is a real value
# wearing a placeholder as decoration.
REDACTION_RUN_MAX_PREFIX=8

# --- Rule 1: known Lark prefix followed by a realistic random part ----------
#
# `tbl`, `rec`, `vew`, `blk` and `boxcn` were missing from the prefix list, and
# it carried `tbln`, which the platform does not issue -- table ids are `tbl`
# plus the random part.
#
# The random part must now be at least 8 characters. The old bound was 4, which
# flagged `ou_12345` -- a value no reader would mistake for real. Every genuine
# id in these docs is far longer: open_ids are 32 hex characters.
PREFIXES='(wikcn|doccn|docx[a-z]|shtcn|bascn|fldcn|vewcn|obcn|flec|tbl|rec|vew|blk|boxcn|ou_|oc_|omm_|cli_)'
RULE1_RE="${PREFIXES}[A-Za-z0-9]{8,}"

# --- Rule 2: token-bearing context holding an opaque value -----------------
#
# Base tokens issued today carry no prefix at all. lark-base-app-block-data-config.md
# carried one such value -- 27 mixed-case alphanumerics, no prefix -- which rule 1
# cannot see at any threshold, while the legacy bascn-prefixed form in the same
# skill set was caught. Anchoring on the key or flag name instead of the value's
# shape is what makes a prefix-less token detectable without flagging ordinary
# prose.
#
# Only a quoted JSON value, or a value directly after a --*-token / --*-id flag,
# counts, and it must be at least 16 characters. Prose such as "pass the
# base_token you copied from the URL to --base-token." has neither shape.
RULE2_JSON='"[a-z_]*(token|_id)" *: *"[A-Za-z0-9_-]{16,}"'
RULE2_FLAG='--[a-z-]*(token|id) +[A-Za-z0-9_-]{16,}'

# --- Rule 3: partially masked real values ----------------------------------
#
# lark-base-dashboard-block-get-data.md carried a bascn-prefixed value with only
# its middle starred out, keeping a real head and a real tail. That is how a real
# value gets redacted -- nobody writes a placeholder that way. The asterisks also
# break rule 1's character class, so without this rule a masked token is strictly
# less visible than an unmasked one. A head-only stub, real prefix followed by a
# run of x's and nothing after, has no trailing fragment and is left alone.
RULE3_RE='[A-Za-z0-9]{4,}\*{3,}[A-Za-z0-9]{4,}'

# A grep that cannot search must not read as "found nothing". scan_rule runs
# inside a command substitution, so an `exit` there would only leave the
# subshell and the check would carry on reporting the file as clean. The
# subshell records the failure in this file instead, and the parent aborts on it
# after the loop.
FATAL_FLAG=$(mktemp)
trap 'rm -f "$FATAL_FLAG"' EXIT

# run_grep runs one grep and keeps its three outcomes apart. grep exits 0 when
# it matched, 1 when it searched and found nothing, and 2 when it could not
# search at all. Collapsing 1 and 2 turns an unreadable file into a silent pass.
#
# Every pattern is passed with -e: rule 2's pattern begins with "--", which grep
# would otherwise read as an option and reject with rc=2.
run_grep() {
  local rc=0 out
  out=$("$@") || rc=$?
  if [[ $rc -ge 2 ]]; then
    echo "check-doc-tokens: grep could not search (rc=$rc): $*" >&2
    echo "$rc" >>"$FATAL_FLAG"
    exit "$rc"
  fi
  if [[ $rc -eq 1 ]]; then
    return 1
  fi
  printf '%s' "$out"
  return 0
}

# scan_rule prints the matches of one regex in one file, minus placeholders.
#
# mixed=1 additionally requires the matched value to contain both a letter and a
# digit, which is what separates a real identifier from a word or a counter:
#   recommended                       `rec` + letters, no digit  -> dropped
#   ou_new_speaker_open_id            readable stub, no digit    -> dropped
#   --meeting-id 7628568141510692381  all digits, no letter      -> dropped
#   Qw3rQw3rQw3rQw3rQw3rQw3r          mixed, 24 chars            -> kept
#
# The test must run on the value alone. grep -n prefixes each match with its
# line number, so testing the whole line let every match through on the digits
# in "282:" -- which is why awk splits the prefix off first.
scan_rule() {
  local regex="$1" file="$2" mixed="$3" hits kept
  hits=$(run_grep grep -nEo -e "$regex" "$file") || return 0
  kept=$(awk -v placeholder="$PLACEHOLDER_LC" -v mixed="$mixed" \
             -v run_max="$REDACTION_RUN_MAX_PREFIX" '
    # A run of x'"'"'s or y'"'"'s only makes a value fake if removing every run leaves
    # nothing but a short prefix: tblxxxxxxxx -> tbl, MAGObxxxxx -> MAGOb.
    function redaction_stub(lv,   rest) {
      rest = lv
      gsub(/x{3,}/, "", rest)
      gsub(/y{3,}/, "", rest)
      return (rest != lv && length(rest) <= run_max)
    }
    {
      # grep -n prefixes each match with its line number. Both tests below must
      # see the value alone: testing the whole line let everything through on
      # the digits in "282:", and testing the whole match let the key name in
      # "base_token": "..." satisfy the _token placeholder rule on its own.
      m = substr($0, index($0, ":") + 1)
      if (match(m, /"[^"]*"$/)) {
        v = substr(m, RSTART + 1, RLENGTH - 2)   # "key": "value"  -> value
      } else {
        n = split(m, parts, /[ \t]+/)
        v = parts[n]                              # --flag value   -> value
      }
      lv = tolower(v)
      if (lv ~ placeholder) next
      if (redaction_stub(lv)) next
      if (mixed == "1" && !(v ~ /[0-9]/ && v ~ /[A-Za-z]/)) next
      print
    }' <<<"$hits")
  [[ -z "$kept" ]] && return 0
  printf '%s\n' "$kept"
}

while IFS= read -r -d '' file; do
  matches=$(
    scan_rule "$RULE1_RE" "$file" 1
    scan_rule "$RULE2_JSON" "$file" 1
    scan_rule "$RULE2_FLAG" "$file" 1
    # Rule 3 is exempt: the masked shape is specific enough on its own, and a
    # redacted value such as bascn***************Qw3rT need not carry a digit.
    scan_rule "$RULE3_RE" "$file" 0
  )
  nonblank=$(run_grep grep -vE -e '^[[:space:]]*$' <<<"$matches") || nonblank=""
  # A value can trip more than one rule -- an `ou_` id matches rule 1 on its
  # prefix and rule 2 on its JSON key -- so collapse to one report line per
  # source line, keyed on the line number grep -n prefixes.
  if [[ -n "$nonblank" ]]; then
    nonblank=$(sort -t: -k1,1n -u <<<"$nonblank")
  fi
  if [[ -n "$nonblank" ]]; then
    echo ""
    echo "❌  $file"
    echo "    Contains realistic-looking token values that may trigger gitleaks:"
    while IFS= read -r line; do
      echo "      $line"
    done <<< "$nonblank"
    echo "    → Replace with a placeholder, e.g.: wikcn_EXAMPLE_TOKEN, doccn_EXAMPLE_TOKEN"
    ERRORS=$((ERRORS + 1))
  fi
done < <(find "$SKILLS_DIR" -path "*/references/*.md" -print0)

if [[ -s "$FATAL_FLAG" ]]; then
  echo ""
  echo "❌  check-doc-tokens: at least one file could not be searched; results above are incomplete." >&2
  exit 2
fi

if [[ $ERRORS -gt 0 ]]; then
  echo ""
  echo "❌  check-doc-tokens: $ERRORS file(s) contain realistic token values in reference docs."
  echo "    Use _EXAMPLE_TOKEN placeholders to avoid false positives in gitleaks CI."
  exit 1
else
  echo "✅  check-doc-tokens: all reference docs use safe placeholder tokens."
fi
