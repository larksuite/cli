#!/usr/bin/env bash
set -euo pipefail

IDENTITY="user"
OUTPUT_DIR=""
TARGET_TYPE=""

declare -a TMP_FILES=()
declare -A SEEN_FOLDERS=()

cleanup() {
  local f
  for f in "${TMP_FILES[@]:-}"; do
    if [[ -n "$f" && -e "$f" ]]; then
      rm -f "$f"
    fi
  done
}
trap cleanup EXIT

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[lark-backup] $*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

json_tmp() {
  local f
  f="$(mktemp)"
  TMP_FILES+=("$f")
  printf '%s\n' "$f"
}

require_flag_value() {
  local flag="$1"
  local value="${2:-}"
  if [[ -z "$value" || "$value" == --* ]]; then
    fail "${flag} requires a value"
  fi
}

usage() {
  cat <<'EOF'
Usage:
  lark-backup.sh auto   --target <url-or-token> [--type <kind>] [--output-dir <dir>] [--as user|bot]
  lark-backup.sh folder --folder-token <token> [--output-dir <dir>] [--as user|bot]
  lark-backup.sh base   --base-token <token> [--output-dir <dir>] [--as user|bot]
  lark-backup.sh doc    --token <token> --doc-type <docx|doc> [--output-dir <dir>] [--as user|bot]
  lark-backup.sh sheet  --token <token> [--output-dir <dir>] [--as user|bot]
  lark-backup.sh file   --token <token> [--output-dir <dir>] [--as user|bot]

Examples:
  lark-backup.sh auto --target "https://xxx.feishu.cn/wiki/xxxx" --output-dir ./backup/wiki
  lark-backup.sh folder --folder-token "fldxxxx" --output-dir ./backup/drive
  lark-backup.sh base --base-token "app_xxx" --output-dir ./backup/base
EOF
}

sanitize_name() {
  local raw="${1:-}"
  local cleaned
  cleaned="$(printf '%s' "$raw" | tr '/\n\r\t' '____' | sed -E 's/[[:space:]]+/_/g; s/[^[:alnum:]_.-]+/_/g; s/_+/_/g; s/^[_ .-]+//; s/[_ .-]+$//')"
  if [[ -z "$cleaned" ]]; then
    cleaned="item"
  fi
  printf '%s\n' "$cleaned"
}

prepare_output_dir() {
  local requested="${1:-}"
  local dir
  if [[ -z "$requested" ]]; then
    dir="./lark-backup-$(date +%Y%m%d-%H%M%S)"
  else
    dir="$requested"
    if [[ -d "$dir" ]] && [[ -n "$(find "$dir" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]]; then
      dir="${dir%/}-$(date +%Y%m%d-%H%M%S)"
    fi
  fi
  mkdir -p "$dir"
  printf '%s\n' "$dir"
}

run_shortcut_json() {
  local out="$1"
  local err_out
  shift
  err_out="$(json_tmp)"
  if ! "$@" >"$out" 2>"$err_out"; then
    [[ -s "$out" ]] && cat "$out" >&2
    [[ -s "$err_out" ]] && cat "$err_out" >&2
    fail "shortcut command failed: $*"
  fi
  jq -e '.ok == true' "$out" >/dev/null || {
    cat "$out" >&2
    fail "shortcut command failed: $*"
  }
}

run_service_json() {
  local out="$1"
  local err_out
  shift
  err_out="$(json_tmp)"
  if ! "$@" >"$out" 2>"$err_out"; then
    [[ -s "$out" ]] && cat "$out" >&2
    [[ -s "$err_out" ]] && cat "$err_out" >&2
    fail "service command failed: $*"
  fi
  jq -e '.code == 0' "$out" >/dev/null || {
    cat "$out" >&2
    fail "service command failed: $*"
  }
}

extract_token() {
  local target="$1"
  local clean="${target%%\?*}"
  local result=""
  clean="${clean%%#*}"
  if [[ "$clean" == http://* || "$clean" == https://* ]]; then
    if [[ "$clean" =~ /drive/folder/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /drive/file/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /wiki/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /docx/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /doc/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /sheets/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    elif [[ "$clean" =~ /base/([^/]+)/?$ ]]; then
      result="${BASH_REMATCH[1]}"
    else
      fail "cannot extract token from URL: $target"
    fi
  else
    result="${clean%/}"
  fi
  [[ -n "$result" ]] || fail "cannot extract token from: $target"
  printf '%s\n' "$result"
}

infer_type() {
  local target="$1"
  local explicit="${2:-}"
  local clean
  clean="${target%%\?*}"
  clean="${clean%%#*}"

  if [[ -n "$explicit" ]]; then
    printf '%s\n' "$explicit"
    return
  fi

  case "$clean" in
    http://*|https://*)
      case "$clean" in
        */drive/folder/*) echo "folder" ;;
        */drive/file/*) echo "file" ;;
        */wiki/*) echo "wiki" ;;
        */docx/*) echo "docx" ;;
        */doc/*) echo "doc" ;;
        */sheets/*) echo "sheet" ;;
        */base/*) echo "bitable" ;;
        *) fail "cannot infer target type from URL: $target; pass --type" ;;
      esac
      ;;
    *)
      case "$clean" in
        # Documented token prefixes per Lark/Feishu API
        fld*) echo "folder" ;;  # fldcn... for folders
        dox*) echo "docx" ;;    # doxcn... for new documents
        sht*) echo "sheet" ;;   # shtcn... for sheets
        box*) echo "file" ;;    # boxcn... for files
        # Legacy/undocumented prefixes - require explicit --type
        # Removed: wikcn*, doccn*, app_*, bascn* to avoid misclassification
        *) fail "cannot infer target type from token: $target; pass --type" ;;
      esac
      ;;
  esac
}

write_json_copy() {
  local src="$1"
  local dest="$2"
  cp "$src" "$dest"
}

backup_doc() {
  local token="$1"
  local doc_type="$2"
  local outdir="$3"
  local meta_json fetch_json

  mkdir -p "$outdir"
  log "backup ${doc_type}: ${token}"

  meta_json="$(json_tmp)"
  run_service_json "$meta_json" \
    lark-cli drive metas batch_query --as "$IDENTITY" \
    --data "$(jq -cn --arg token "$token" --arg doc_type "$doc_type" '{request_docs:[{doc_token:$token,doc_type:$doc_type}],with_url:true}')"
  write_json_copy "$meta_json" "$outdir/meta.json"

  if [[ "$doc_type" == "docx" ]]; then
    fetch_json="$(json_tmp)"
    run_shortcut_json "$fetch_json" \
      lark-cli docs +fetch --as "$IDENTITY" --doc "$token" --format json
    write_json_copy "$fetch_json" "$outdir/fetch.json"
    if jq -e '.data.markdown | type == "string" and length > 0' "$fetch_json" >/dev/null; then
      jq -r '.data.markdown' "$fetch_json" >"$outdir/content.md"
    else
      log "docs +fetch returned no markdown; skipping content.md for ${token}"
    fi

    run_shortcut_json "$outdir/export-markdown.json" \
      lark-cli drive +export --as "$IDENTITY" \
      --token "$token" --doc-type docx --file-extension markdown --output-dir "$outdir"
    run_shortcut_json "$outdir/export-pdf.json" \
      lark-cli drive +export --as "$IDENTITY" \
      --token "$token" --doc-type docx --file-extension pdf --output-dir "$outdir"
    return
  fi

  run_shortcut_json "$outdir/export-docx.json" \
    lark-cli drive +export --as "$IDENTITY" \
    --token "$token" --doc-type doc --file-extension docx --output-dir "$outdir"
  run_shortcut_json "$outdir/export-pdf.json" \
    lark-cli drive +export --as "$IDENTITY" \
    --token "$token" --doc-type doc --file-extension pdf --output-dir "$outdir"
}

backup_sheet() {
  local token="$1"
  local outdir="$2"
  local info_json
  local idx=0

  mkdir -p "$outdir/csv"
  log "backup sheet: ${token}"

  info_json="$(json_tmp)"
  run_shortcut_json "$info_json" \
    lark-cli sheets +info --as "$IDENTITY" --spreadsheet-token "$token"
  write_json_copy "$info_json" "$outdir/info.json"

  run_shortcut_json "$outdir/export-xlsx.json" \
    lark-cli sheets +export --as "$IDENTITY" \
    --spreadsheet-token "$token" --file-extension xlsx --output-path "$outdir/workbook.xlsx"

  while IFS=$'\t' read -r sheet_id title; do
    [[ -z "$sheet_id" ]] && continue
    idx=$((idx + 1))
    local safe_title
    safe_title="$(sanitize_name "$title")"
    run_shortcut_json "$outdir/csv-${idx}.json" \
      lark-cli sheets +export --as "$IDENTITY" \
      --spreadsheet-token "$token" \
      --file-extension csv \
      --sheet-id "$sheet_id" \
      --output-path "$outdir/csv/$(printf '%02d' "$idx")-${safe_title}.csv"
  done < <(jq -r '.data.sheets[]? | [.sheet_id, (.title // .sheet_id)] | @tsv' "$info_json")
}

backup_file() {
  local token="$1"
  local outdir="$2"

  mkdir -p "$outdir"
  log "backup file: ${token}"

  (
    cd "$outdir"
    run_shortcut_json "download.json" \
      lark-cli drive +download --as "$IDENTITY" --file-token "$token"
  )
}

list_all_base_tables() {
  local token="$1"
  local out_json="$2"
  local offset=0
  local limit=100
  local total=0
  local count=0
  local merged_json page_json items_json next_merged_json

  merged_json="$(json_tmp)"
  printf '%s\n' '[]' >"$merged_json"

  while :; do
    page_json="$(json_tmp)"
    run_shortcut_json "$page_json" \
      lark-cli base +table-list --as "$IDENTITY" --base-token "$token" --offset "$offset" --limit "$limit"

    items_json="$(json_tmp)"
    jq -c '.data.items // []' "$page_json" >"$items_json"

    next_merged_json="$(json_tmp)"
    jq -s '.[0] + .[1]' "$merged_json" "$items_json" >"$next_merged_json"
    mv "$next_merged_json" "$merged_json"

    count="$(jq -r '(.data.items // []) | length' "$page_json")"
    total="$(jq -r '.data.total // 0' "$page_json")"

    if [[ "$count" -eq 0 ]]; then
      break
    fi

    offset=$((offset + count))

    if [[ "$total" =~ ^[0-9]+$ ]] && [[ "$total" -gt 0 ]] && [[ "$offset" -lt "$total" ]]; then
      continue
    fi
    if [[ "$count" -lt "$limit" ]]; then
      break
    fi
  done

  jq -n \
    --slurpfile items "$merged_json" \
    --argjson total "$total" \
    --argjson limit "$limit" \
    '
      ($items[0]) as $merged
      | {
          ok: true,
          data: {
            items: $merged,
            offset: 0,
            limit: $limit,
            count: ($merged | length),
            total: (if $total > 0 then $total else ($merged | length) end)
          }
        }
    ' >"$out_json"
}

backup_base() {
  local token="$1"
  local outdir="$2"
  local tables_json

  mkdir -p "$outdir/tables"
  log "backup base: ${token}"

  run_shortcut_json "$outdir/base.json" \
    lark-cli base +base-get --as "$IDENTITY" --base-token "$token"

  run_shortcut_json "$outdir/snapshot-xlsx.json" \
    lark-cli drive +export --as "$IDENTITY" \
    --token "$token" --doc-type bitable --file-extension xlsx --output-dir "$outdir"

  tables_json="$(json_tmp)"
  list_all_base_tables "$token" "$tables_json"
  write_json_copy "$tables_json" "$outdir/tables.json"

  while IFS=$'\t' read -r table_id table_name; do
    [[ -z "$table_id" ]] && continue
    local table_dir count has_more total offset page page_json
    table_dir="$outdir/tables/${table_id}-$(sanitize_name "$table_name")"
    mkdir -p "$table_dir"

    run_shortcut_json "$table_dir/table.json" \
      lark-cli base +table-get --as "$IDENTITY" --base-token "$token" --table-id "$table_id"

    offset=0
    page=1
    while :; do
      page_json="$table_dir/records-page-$(printf '%03d' "$page").json"
      run_shortcut_json "$page_json" \
        lark-cli base +record-list --as "$IDENTITY" \
        --base-token "$token" --table-id "$table_id" --offset "$offset" --limit 200

      count="$(jq -r '((.data.records.rows // .data.data // []) | length)' "$page_json")"
      has_more="$(jq -r '.data.has_more // false' "$page_json")"
      total="$(jq -r '.data.total // 0' "$page_json")"

      if [[ "$count" -eq 0 ]]; then
        break
      fi

      offset=$((offset + count))
      page=$((page + 1))

      if [[ "$has_more" == "true" ]]; then
        continue
      fi
      if [[ "$total" =~ ^[0-9]+$ ]] && [[ "$total" -gt "$offset" ]]; then
        continue
      fi
      break
    done
  done < <(jq -r '.data.items[]? | [.table_id, (.table_name // .table_id)] | @tsv' "$tables_json")
}

resolve_wiki_to_object() {
  local token="$1"
  local out_json="$2"
  run_service_json "$out_json" \
    lark-cli wiki spaces get_node --as "$IDENTITY" \
    --params "$(jq -cn --arg token "$token" '{token:$token}')"
}

backup_auto() {
  local target="$1"
  local outdir="$2"
  local inferred_type token wiki_json obj_type obj_token

  inferred_type="$(infer_type "$target" "$TARGET_TYPE")"
  token="$(extract_token "$target")"

  case "$inferred_type" in
    docx|doc)
      backup_doc "$token" "$inferred_type" "$outdir"
      ;;
    sheet)
      backup_sheet "$token" "$outdir"
      ;;
    bitable)
      backup_base "$token" "$outdir"
      ;;
    file)
      backup_file "$token" "$outdir"
      ;;
    folder)
      backup_folder "$token" "$outdir"
      ;;
    wiki)
      wiki_json="$(json_tmp)"
      resolve_wiki_to_object "$token" "$wiki_json"
      write_json_copy "$wiki_json" "$outdir/wiki-node.json"
      obj_type="$(jq -r '.data.node.obj_type // empty' "$wiki_json")"
      obj_token="$(jq -r '.data.node.obj_token // empty' "$wiki_json")"
      [[ -n "$obj_type" && -n "$obj_token" ]] || fail "failed to resolve wiki target: $target"
      case "$obj_type" in
        docx|doc)
          backup_doc "$obj_token" "$obj_type" "$outdir"
          ;;
        sheet)
          backup_sheet "$obj_token" "$outdir"
          ;;
        bitable)
          backup_base "$obj_token" "$outdir"
          ;;
        file)
          backup_file "$obj_token" "$outdir"
          ;;
        *)
          fail "wiki target resolved to unsupported type: $obj_type"
          ;;
      esac
      ;;
    *)
      fail "unsupported target type: $inferred_type"
      ;;
  esac
}

backup_folder() {
  local folder_token="$1"
  local outdir="$2"
  local list_json idx=0

  if [[ -n "${SEEN_FOLDERS[$folder_token]:-}" ]]; then
    log "skip already visited folder: ${folder_token}"
    return
  fi
  SEEN_FOLDERS["$folder_token"]=1

  mkdir -p "$outdir/items"
  log "backup folder: ${folder_token}"

  list_json="$(json_tmp)"
  run_service_json "$list_json" \
    lark-cli drive files list --as "$IDENTITY" --format json --page-all \
    --params "$(jq -cn --arg folder_token "$folder_token" '{folder_token:$folder_token,page_size:100}')"
  write_json_copy "$list_json" "$outdir/folder-items.json"

  while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue
    idx=$((idx + 1))

    local entry_name entry_type entry_token shortcut_type shortcut_token entry_dir safe_name
    entry_name="$(jq -r '.name // "item"' <<<"$entry")"
    entry_type="$(jq -r '.type // empty' <<<"$entry")"
    entry_token="$(jq -r '.token // empty' <<<"$entry")"
    shortcut_type="$(jq -r '.shortcut_info.target_type // empty' <<<"$entry")"
    shortcut_token="$(jq -r '.shortcut_info.target_token // empty' <<<"$entry")"

    if [[ -n "$shortcut_type" && -n "$shortcut_token" ]]; then
      entry_type="$shortcut_type"
      entry_token="$shortcut_token"
    fi

    safe_name="$(sanitize_name "$entry_name")"
    entry_dir="$outdir/items/$(printf '%02d' "$idx")-${safe_name}"
    mkdir -p "$entry_dir"
    jq . <<<"$entry" >"$entry_dir/entry.json"

    case "$entry_type" in
      folder)
        backup_folder "$entry_token" "$entry_dir"
        ;;
      docx|doc)
        backup_doc "$entry_token" "$entry_type" "$entry_dir"
        ;;
      sheet)
        backup_sheet "$entry_token" "$entry_dir"
        ;;
      bitable)
        backup_base "$entry_token" "$entry_dir"
        ;;
      file)
        backup_file "$entry_token" "$entry_dir"
        ;;
      *)
        printf '%s\n' "unsupported type: ${entry_type:-unknown}" >"$entry_dir/SKIPPED.txt"
        ;;
    esac
  done < <(jq -c '.data.files[]?' "$list_json")
}

main() {
  need_cmd lark-cli
  need_cmd jq

  local command="${1:-}"
  [[ -n "$command" ]] || {
    usage
    exit 1
  }
  if [[ "$command" == "-h" || "$command" == "--help" ]]; then
    usage
    exit 0
  fi
  shift || true

  local target="" token="" folder_token="" base_token="" doc_type=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --as)
        require_flag_value "$1" "${2:-}"
        IDENTITY="$2"
        shift 2
        ;;
      --output-dir)
        require_flag_value "$1" "${2:-}"
        OUTPUT_DIR="$2"
        shift 2
        ;;
      --type)
        require_flag_value "$1" "${2:-}"
        TARGET_TYPE="$2"
        shift 2
        ;;
      --target)
        require_flag_value "$1" "${2:-}"
        target="$2"
        shift 2
        ;;
      --token)
        require_flag_value "$1" "${2:-}"
        token="$2"
        shift 2
        ;;
      --folder-token)
        require_flag_value "$1" "${2:-}"
        folder_token="$2"
        shift 2
        ;;
      --base-token)
        require_flag_value "$1" "${2:-}"
        base_token="$2"
        shift 2
        ;;
      --doc-type)
        require_flag_value "$1" "${2:-}"
        doc_type="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done

  OUTPUT_DIR="$(prepare_output_dir "$OUTPUT_DIR")"
  log "identity=${IDENTITY}"
  log "output_dir=${OUTPUT_DIR}"

  case "$command" in
    auto)
      [[ -n "$target" ]] || fail "--target is required for auto"
      backup_auto "$target" "$OUTPUT_DIR"
      ;;
    folder)
      [[ -n "$folder_token" ]] || fail "--folder-token is required for folder"
      backup_folder "$folder_token" "$OUTPUT_DIR"
      ;;
    base)
      [[ -n "$base_token" ]] || fail "--base-token is required for base"
      backup_base "$base_token" "$OUTPUT_DIR"
      ;;
    doc)
      [[ -n "$token" ]] || fail "--token is required for doc"
      [[ "$doc_type" == "docx" || "$doc_type" == "doc" ]] || fail "--doc-type must be docx or doc"
      backup_doc "$token" "$doc_type" "$OUTPUT_DIR"
      ;;
    sheet)
      [[ -n "$token" ]] || fail "--token is required for sheet"
      backup_sheet "$token" "$OUTPUT_DIR"
      ;;
    file)
      [[ -n "$token" ]] || fail "--token is required for file"
      backup_file "$token" "$OUTPUT_DIR"
      ;;
    *)
      usage
      fail "unknown command: $command"
      ;;
  esac

  log "done"
  echo "$OUTPUT_DIR"
}

main "$@"
