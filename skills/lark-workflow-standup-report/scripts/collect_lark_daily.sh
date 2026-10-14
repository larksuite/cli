#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lark_common.sh
. "$SCRIPT_DIR/lark_common.sh"

DATE=""
START=""
END=""
OUT_DIR=""
REQUEST_TIMEOUT_SECONDS=120
IM_REQUEST_TIMEOUT_SECONDS=45
IM_CHUNK_HOURS=1
IM_MIN_CHUNK_MINUTES=60
IM_MAX_FAILURES_PER_SEARCH=4
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --date|-Date) DATE=$2; shift 2 ;;
    --start|-Start) START=$2; shift 2 ;;
    --end|-End) END=$2; shift 2 ;;
    --out-dir|-OutDir) OUT_DIR=$2; shift 2 ;;
    --request-timeout-seconds|-RequestTimeoutSeconds) REQUEST_TIMEOUT_SECONDS=$2; shift 2 ;;
    --im-request-timeout-seconds|-ImRequestTimeoutSeconds) IM_REQUEST_TIMEOUT_SECONDS=$2; shift 2 ;;
    --im-chunk-hours|-ImChunkHours) IM_CHUNK_HOURS=$2; shift 2 ;;
    --im-min-chunk-minutes|-ImMinChunkMinutes) IM_MIN_CHUNK_MINUTES=$2; shift 2 ;;
    --im-max-failures-per-search|-ImMaxFailuresPerSearch) IM_MAX_FAILURES_PER_SEARCH=$2; shift 2 ;;
    --dry-run|-DryRun) DRY_RUN=true; shift ;;
    *) die "Unknown argument: $1" ;;
  esac
done

[[ -n "$DATE" && -n "$START" && -n "$END" && -n "$OUT_DIR" ]] || die "Required: --date --start --end --out-dir"
need_cmd jq
need_cmd lark-cli
need_cmd python3

mkdir -p "$OUT_DIR"
MANIFEST="$OUT_DIR/source_manifest.json"
manifest_init "$MANIFEST" "$DATE" "$START" "$END" "$DRY_RUN"

if [[ "$DRY_RUN" == true ]]; then
  jq '.plan=[
    "calendar +agenda",
    "vc +search with pagination",
    "contact +get-user",
    "im +messages-search all with pagination",
    "im +messages-search sender=current_user with pagination",
    "docs +search with pagination",
    "redact raw json files",
    "write source_manifest.json"
  ]' "$MANIFEST" >"${MANIFEST}.tmp"
  mv "${MANIFEST}.tmp" "$MANIFEST"
  printf '%s\n' "$MANIFEST"
  exit 0
fi

MEETING_IDS_FILE="$OUT_DIR/.meeting_ids"
MINUTE_TOKENS_FILE="$OUT_DIR/.minute_tokens"
IM_FAILURES_DIR="$OUT_DIR/.im_failures"
: >"$MEETING_IDS_FILE"
: >"$MINUTE_TOKENS_FILE"
mkdir -p "$IM_FAILURES_DIR"

add_unique_line() {
  local file=$1 value=$2
  [[ -n "$value" ]] || return 0
  grep -Fxq "$value" "$file" 2>/dev/null || printf '%s\n' "$value" >>"$file"
}

extract_minute_tokens_from_text() {
  grep -Eo 'https?://[^[:space:]"'\'')]+/minutes/[^/?#[:space:]"'\'')]+' 2>/dev/null | sed -E 's#.*\/minutes\/([^/?#]+).*#\1#' || true
}

try_lark() {
  local source=$1 outfile=$2 timeout_seconds=$3
  shift 3
  if ! run_lark_json "$outfile" "$timeout_seconds" "$@"; then
    if is_auth_error_file "$outfile"; then
      manifest_add_error "$MANIFEST" "$source" "$(jq -r '.error.message // "auth error"' "$outfile" 2>/dev/null)"
      printf '%s\n' "$MANIFEST"
      exit 1
    fi
    manifest_add_error "$MANIFEST" "$source" "$(cat "$outfile" | limit_text 360)"
    return 1
  fi
}

calendar_file="$OUT_DIR/calendar_agenda_$DATE.json"
if try_lark calendar "$calendar_file" "$REQUEST_TIMEOUT_SECONDS" calendar +agenda --as user --start "$START" --end "$END" --format json; then
  manifest_set_file "$MANIFEST" calendar "$calendar_file"
  manifest_set_count "$MANIFEST" calendar "$(json_count "$calendar_file" '.data')"
fi

vc_files=()
vc_page=1
vc_page_token=""
vc_has_more=true
while [[ "$vc_has_more" == true && $vc_page -le 20 ]]; do
  vc_file="$OUT_DIR/vc_search_${DATE}_page${vc_page}.json"
  args=(vc +search --as user --start "$START" --end "$END" --format json --page-size 30)
  [[ -n "$vc_page_token" ]] && args+=(--page-token "$vc_page_token")
  if ! try_lark vc "$vc_file" "$REQUEST_TIMEOUT_SECONDS" "${args[@]}"; then
    break
  fi
  vc_files+=("$vc_file")
  jq -r '.data.items[]? | .id // empty' "$vc_file" | while IFS= read -r id; do add_unique_line "$MEETING_IDS_FILE" "$id"; done
  jq -r '.. | strings? // empty' "$vc_file" | extract_minute_tokens_from_text | while IFS= read -r token; do add_unique_line "$MINUTE_TOKENS_FILE" "$token"; done
  vc_has_more="$(jq -r '.data.has_more // false' "$vc_file")"
  vc_page_token="$(jq -r '.data.page_token // empty' "$vc_file")"
  vc_page=$((vc_page + 1))
done
if [[ ${#vc_files[@]} -gt 0 ]]; then
  manifest_set_files_array "$MANIFEST" vc "${vc_files[@]}"
  vc_count=0
  for f in "${vc_files[@]}"; do vc_count=$((vc_count + $(json_count "$f" '.data.items'))); done
  manifest_set_count "$MANIFEST" vc "$vc_count"
fi

detail_files=()
detail_index=1
while IFS= read -r meeting_id; do
  [[ -n "$meeting_id" ]] || continue
  detail_file="$OUT_DIR/vc_meeting_${DATE}_detail_${detail_index}.json"
  params=$(jq -nc --arg meeting_id "$meeting_id" '{meeting_id:$meeting_id,with_participants:true}')
  if try_lark vc_meeting_details "$detail_file" "$REQUEST_TIMEOUT_SECONDS" vc meeting get --as user --params "$params"; then
    detail_files+=("$detail_file")
    jq -r '[.data.note_doc_token?, .data.verbatim_doc_token?, .data.minute_token?, .data.url?, .data.meeting_url?] | .[]? // empty' "$detail_file" |
      extract_minute_tokens_from_text | while IFS= read -r token; do add_unique_line "$MINUTE_TOKENS_FILE" "$token"; done
    minute_token="$(jq -r '.data.minute_token // empty' "$detail_file")"
    add_unique_line "$MINUTE_TOKENS_FILE" "$minute_token"
  fi
  detail_index=$((detail_index + 1))
done <"$MEETING_IDS_FILE"
if [[ ${#detail_files[@]} -gt 0 ]]; then
  manifest_set_files_array "$MANIFEST" vc_meeting_details "${detail_files[@]}"
  manifest_set_count "$MANIFEST" vc_meeting_details "${#detail_files[@]}"
fi

notes_files=()
if [[ -s "$MEETING_IDS_FILE" ]]; then
  chunk_index=1
  while IFS= read -r chunk; do
    [[ -n "$chunk" ]] || continue
    notes_file="$OUT_DIR/vc_notes_${DATE}_meeting_chunk${chunk_index}.json"
    if try_lark vc_notes "$notes_file" "$REQUEST_TIMEOUT_SECONDS" vc +notes --as user --meeting-ids "$chunk" --format json; then
      notes_files+=("$notes_file")
    fi
    chunk_index=$((chunk_index + 1))
  done < <(paste -sd, "$MEETING_IDS_FILE" | fold -s -w 3000)
fi
if [[ ${#notes_files[@]} -gt 0 ]]; then
  manifest_set_files_array "$MANIFEST" vc_notes "${notes_files[@]}"
  manifest_set_count "$MANIFEST" vc_notes "${#notes_files[@]}"
fi

minute_files=()
if [[ -s "$MINUTE_TOKENS_FILE" ]]; then
  chunk_index=1
  while IFS= read -r chunk; do
    [[ -n "$chunk" ]] || continue
    minute_file="$OUT_DIR/vc_notes_${DATE}_minute_chunk${chunk_index}.json"
    if try_lark vc_minutes "$minute_file" "$REQUEST_TIMEOUT_SECONDS" vc +notes --as user --minute-tokens "$chunk" --format json; then
      minute_files+=("$minute_file")
    fi
    chunk_index=$((chunk_index + 1))
  done < <(paste -sd, "$MINUTE_TOKENS_FILE" | fold -s -w 3000)
fi
if [[ ${#minute_files[@]} -gt 0 ]]; then
  manifest_set_files_array "$MANIFEST" vc_minutes "${minute_files[@]}"
  manifest_set_count "$MANIFEST" vc_minutes "${#minute_files[@]}"
fi

current_user_open_id=""
current_user_file="$OUT_DIR/current_user_$DATE.json"
if try_lark current_user "$current_user_file" "$REQUEST_TIMEOUT_SECONDS" contact +get-user --as user --format json; then
  manifest_set_file "$MANIFEST" current_user "$current_user_file"
  current_user_open_id="$(jq -r '.data.user.open_id // .data.open_id // empty' "$current_user_file")"
  current_user_name="$(jq -r '.data.user.name // .data.name // empty' "$current_user_file")"
  manifest_set_value "$MANIFEST" current_user_open_id "$current_user_open_id"
  manifest_set_value "$MANIFEST" current_user_name "$current_user_name"
fi

format_iso_offset() {
  python3 - "$1" <<'PY'
from datetime import datetime
import sys
s=sys.argv[1]
if s.endswith('Z'): s=s[:-1]+'+00:00'
print(datetime.fromisoformat(s).isoformat(timespec='seconds'))
PY
}

add_im_failure() {
  local prefix=$1 message=$2
  local file="$IM_FAILURES_DIR/$prefix"
  local count=0
  [[ -f "$file" ]] && count="$(cat "$file" 2>/dev/null || printf '0')"
  count=$((count + 1))
  printf '%s\n' "$count" >"$file"
  manifest_add_error "$MANIFEST" "$prefix" "$message"
}

im_failure_limit_reached() {
  local prefix=$1
  local file="$IM_FAILURES_DIR/$prefix"
  local count=0
  [[ -f "$file" ]] && count="$(cat "$file" 2>/dev/null || printf '0')"
  [[ "$count" -ge "$IM_MAX_FAILURES_PER_SEARCH" ]]
}

collect_im_range_pages() {
  local prefix=$1 sender=$2 range_start=$3 range_end=$4 chunk_label=$5
  local page=1 page_token="" has_more=true files=()
  while [[ "$has_more" == true && $page -le 50 ]]; do
    im_failure_limit_reached "$prefix" && break
    local file="$OUT_DIR/${prefix}_${chunk_label}_page${page}.json"
    local args=(im +messages-search --as user --start "$(format_iso_offset "$range_start")" --end "$(format_iso_offset "$range_end")" --page-size 50 --format json)
    [[ -n "$sender" ]] && args+=(--sender "$sender")
    [[ -n "$page_token" ]] && args+=(--page-token "$page_token")
    if run_lark_json "$file" "$IM_REQUEST_TIMEOUT_SECONDS" "${args[@]}"; then
      files+=("$file")
      has_more="$(jq -r '.data.has_more // false' "$file")"
      page_token="$(jq -r '.data.page_token // empty' "$file")"
      page=$((page + 1))
      continue
    fi
    if is_auth_error_file "$file"; then
      manifest_add_error "$MANIFEST" "$prefix" "$(jq -r '.error.message // "auth error"' "$file" 2>/dev/null)"
      printf '%s\n' "$MANIFEST"
      exit 1
    fi
    local start_epoch end_epoch minutes
    start_epoch=$(iso_to_epoch "$range_start")
    end_epoch=$(iso_to_epoch "$range_end")
    minutes=$(python3 - "$start_epoch" "$end_epoch" <<'PY'
import sys
print((float(sys.argv[2])-float(sys.argv[1]))/60)
PY
)
    if [[ $page -eq 1 ]] && python3 - "$minutes" "$IM_MIN_CHUNK_MINUTES" <<'PY'
import sys
sys.exit(0 if float(sys.argv[1]) > float(sys.argv[2]) else 1)
PY
    then
      local mid
      mid=$(python3 - "$start_epoch" "$end_epoch" <<'PY'
from datetime import datetime, timezone
import sys
mid=(float(sys.argv[1])+float(sys.argv[2]))/2
print(datetime.fromtimestamp(mid, timezone.utc).astimezone().isoformat(timespec='seconds'))
PY
)
      collect_im_range_pages "$prefix" "$sender" "$range_start" "$mid" "${chunk_label}a"
      collect_im_range_pages "$prefix" "$sender" "$mid" "$range_end" "${chunk_label}b"
      return
    fi
    add_im_failure "$prefix" "IM search failed for $chunk_label page $page ($(format_iso_offset "$range_start") ~ $(format_iso_offset "$range_end")): $(cat "$file" | limit_text 360)"
    break
  done
  printf '%s\n' "${files[@]}"
}

collect_im_pages() {
  local prefix=$1 sender=${2:-}
  local cursor="$START" chunk_index=1 files=()
  local range_end_epoch cursor_epoch chunk_end
  range_end_epoch=$(iso_to_epoch "$END")
  while :; do
    cursor_epoch=$(iso_to_epoch "$cursor")
    python3 - "$cursor_epoch" "$range_end_epoch" <<'PY' || break
import sys
sys.exit(0 if float(sys.argv[1]) < float(sys.argv[2]) else 1)
PY
    if im_failure_limit_reached "$prefix"; then
      manifest_add_error "$MANIFEST" "$prefix" "Stopped IM search after $IM_MAX_FAILURES_PER_SEARCH failures; remaining time range was skipped."
      break
    fi
    chunk_end=$(python3 - "$cursor" "$END" "$IM_CHUNK_HOURS" <<'PY'
from datetime import datetime, timedelta
import sys
cur=sys.argv[1]; end=sys.argv[2]; hours=int(sys.argv[3])
def parse(s):
    if s.endswith('Z'): s=s[:-1]+'+00:00'
    return datetime.fromisoformat(s)
c=parse(cur); e=parse(end)
n=min(c+timedelta(hours=max(1,hours)), e)
print(n.isoformat(timespec='seconds'))
PY
)
    label=$(printf 'chunk%02d' "$chunk_index")
    while IFS= read -r f; do [[ -n "$f" ]] && files+=("$f"); done < <(collect_im_range_pages "$prefix" "$sender" "$cursor" "$chunk_end" "$label")
    cursor="$chunk_end"
    chunk_index=$((chunk_index + 1))
  done
  printf '%s\n' "${files[@]}"
}

if [[ -n "$current_user_open_id" ]]; then
  self_files=()
  while IFS= read -r f; do [[ -n "$f" ]] && self_files+=("$f"); done < <(collect_im_pages "im_messages_self_$DATE" "$current_user_open_id")
  manifest_set_files_array "$MANIFEST" im_self "${self_files[@]}"
  self_count=0
  details_missing=false
  for f in "${self_files[@]}"; do
    self_count=$((self_count + $(json_count "$f" '.data.messages // .data.message_ids // .data.items')))
    jq -e '((.data.message_ids? != null) and (.data.messages? == null)) or ((.data.note // "") | test("failed to fetch message details"))' "$f" >/dev/null 2>&1 && details_missing=true
  done
  manifest_set_count "$MANIFEST" im_self "$self_count"
  [[ "$details_missing" == true ]] && manifest_add_error "$MANIFEST" im_self_details "IM search returned message IDs only; message content enrichment is unavailable."
else
  manifest_add_error "$MANIFEST" im_self "Skipped because current user open_id was unavailable."
fi

all_files=()
while IFS= read -r f; do [[ -n "$f" ]] && all_files+=("$f"); done < <(collect_im_pages "im_messages_all_$DATE")
manifest_set_files_array "$MANIFEST" im_all "${all_files[@]}"
all_count=0
details_missing=false
for f in "${all_files[@]}"; do
  all_count=$((all_count + $(json_count "$f" '.data.messages // .data.message_ids // .data.items')))
  jq -e '((.data.message_ids? != null) and (.data.messages? == null)) or ((.data.note // "") | test("failed to fetch message details"))' "$f" >/dev/null 2>&1 && details_missing=true
done
manifest_set_count "$MANIFEST" im_all "$all_count"
[[ "$details_missing" == true ]] && manifest_add_error "$MANIFEST" im_all_details "IM search returned message IDs only; message content enrichment is unavailable."

doc_files=()
doc_page=1
doc_page_token=""
doc_has_more=true
doc_filter=$(jq -nc --arg start "$START" --arg end "$END" '{open_time:{start:$start,end:$end}}')
while [[ "$doc_has_more" == true && $doc_page -le 10 ]]; do
  doc_file="$OUT_DIR/docs_search_${DATE}_page${doc_page}.json"
  args=(docs +search --as user --query "" --filter "$doc_filter" --page-size 20 --format json)
  [[ -n "$doc_page_token" ]] && args+=(--page-token "$doc_page_token")
  if ! try_lark docs "$doc_file" "$REQUEST_TIMEOUT_SECONDS" "${args[@]}"; then
    break
  fi
  doc_files+=("$doc_file")
  doc_has_more="$(jq -r '.data.has_more // false' "$doc_file")"
  doc_page_token="$(jq -r '.data.page_token // empty' "$doc_file")"
  doc_page=$((doc_page + 1))
done
if [[ ${#doc_files[@]} -gt 0 ]]; then
  manifest_set_files_array "$MANIFEST" docs "${doc_files[@]}"
  doc_count=0
  for f in "${doc_files[@]}"; do doc_count=$((doc_count + $(json_count "$f" '.data.results'))); done
  manifest_set_count "$MANIFEST" docs "$doc_count"
fi

redact_json_dir "$OUT_DIR"
rm -f "$MEETING_IDS_FILE" "$MINUTE_TOKENS_FILE"
rm -rf "$IM_FAILURES_DIR"
printf '%s\n' "$MANIFEST"
