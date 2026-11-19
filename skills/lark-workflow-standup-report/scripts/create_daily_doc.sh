#!/usr/bin/env bash
set -euo pipefail

MARKDOWN_PATH=""
TITLE=""
WIKI_NODE=""
RESPONSE_PATH=""
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --markdown-path|-MarkdownPath) MARKDOWN_PATH=$2; shift 2 ;;
    --title|-Title) TITLE=$2; shift 2 ;;
    --wiki-node|-WikiNode) WIKI_NODE=$2; shift 2 ;;
    --response-path|-ResponsePath) RESPONSE_PATH=$2; shift 2 ;;
    --dry-run|-DryRun) DRY_RUN=true; shift ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$MARKDOWN_PATH" && -n "$TITLE" && -n "$WIKI_NODE" && -n "$RESPONSE_PATH" ]] || {
  echo "Required: --markdown-path --title --wiki-node --response-path" >&2
  exit 1
}
command -v jq >/dev/null 2>&1 || {
  echo "Missing required command: jq" >&2
  exit 1
}
command -v lark-cli >/dev/null 2>&1 || {
  echo "Missing required command: lark-cli" >&2
  exit 1
}
command -v python3 >/dev/null 2>&1 || {
  echo "Missing required command: python3" >&2
  exit 1
}
[[ -f "$MARKDOWN_PATH" ]] || {
  echo "MarkdownPath not found: $MARKDOWN_PATH" >&2
  exit 1
}

mkdir -p "$(dirname "$RESPONSE_PATH")"

if [[ "$DRY_RUN" == true ]]; then
  jq -n --arg title "$TITLE" --arg wiki_node "$WIKI_NODE" --arg markdown_path "$MARKDOWN_PATH" --arg response_path "$RESPONSE_PATH" \
    '{dry_run:true,title:$title,wiki_node:$wiki_node,markdown_path:$markdown_path,response_path:$response_path}' >"$RESPONSE_PATH"
  printf '%s\n' "$RESPONSE_PATH"
  exit 0
fi

temp_content_dir="$(dirname "$RESPONSE_PATH")"
temp_content="$temp_content_dir/daily_doc_create_content.md"
if grep -Eq '^[[:space:]]*#[[:space:]]+' "$MARKDOWN_PATH"; then
  cp "$MARKDOWN_PATH" "$temp_content"
else
  {
    printf '# %s\n\n' "$TITLE"
    cat "$MARKDOWN_PATH"
  } >"$temp_content"
fi

parent_token="$WIKI_NODE"
if [[ "$parent_token" =~ /wiki/([^/?#]+) ]]; then
  parent_token="${BASH_REMATCH[1]}"
fi

content_arg_path="$temp_content"
case "$content_arg_path" in
  /*)
    content_arg_path="$(python3 - "$PWD" "$temp_content" <<'PY'
from pathlib import Path
import os, sys
try:
    print(os.path.relpath(Path(sys.argv[2]).resolve(), Path(sys.argv[1]).resolve()))
except Exception:
    print(sys.argv[2])
PY
)"
    ;;
esac

set +e
lark-cli docs +create --api-version v2 --as user --parent-token "$parent_token" --doc-format markdown --content "@$content_arg_path" --format json >"$RESPONSE_PATH" 2>&1
status=$?
set -e
rm -f "$temp_content"

if [[ $status -ne 0 ]] || jq -e '.ok == false' "$RESPONSE_PATH" >/dev/null 2>&1; then
  echo "lark-cli create failed; response saved to $RESPONSE_PATH" >&2
  exit 1
fi

printf '%s\n' "$RESPONSE_PATH"
