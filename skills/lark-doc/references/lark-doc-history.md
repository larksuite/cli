# docs history（历史版本与回滚）

用于查看 Docx 历史版本、按 `history_version_id` 回滚，以及查询回滚任务状态。

## 安全流程

1. 先用分页接口 `+history-list` 精确查找目标版本的 `history_version_id`。
2. 如果用户指定的是 `revision_id`，不要假设它唯一，也不要把 `revision_id` 直接传给 `+history-revert`。先拉一页并在 `entries[]` 中筛选 `revision_id` 相同的候选；如果未匹配到且 `has_more=true`，继续用 `page_token` 翻页；如果已匹配到候选，最多额外再拉一页补齐可能跨页的相邻候选。最终优先根据用户目标时间与 `edit_time` 的接近程度选择最合适的一条，取同一条的 `history_version_id`；如果没有目标时间，或多个候选无法可靠区分，再向用户展示候选版本（`history_version_id`、`revision_id`、`edit_time`、`name/description`）并确认后回滚。
3. 如果用户指定的是某一时刻但没有指定 `revision_id`，按 `entries[].edit_time` 匹配；优先选择不晚于目标时刻的最近一条历史记录，无法明确匹配时先向用户确认候选版本。
4. **有精确 history entry**：用该条目的 `history_version_id` 调用 `+history-revert`。不要猜测或使用邻近条目。默认最多等待 30 秒；如果返回 `status: running`，记录 `task_id`，再用 `+history-revert-status` 轮询到终态。
5. **没有精确 history entry**：不要直接停止。若用户给出了目标 `revision_id`，尝试一次 `docs +fetch --revision-id <revision_id> --detail full --format json`：
   - 成功返回目标 revision 的真实完整内容：将同一次响应的 `data.document.content` 与 `data.document.reference_map` 配套保存并用于 `docs +update --command overwrite`，不得根据摘要、缓存、模型记忆或硬编码重建正文。
   - 目标 revision 明确不可读或返回业务错误：停止并如实说明。权限、网络或临时系统错误保持原分类，不要据此声称 revision 不存在，也不要绕过身份、权限或确认门禁。
6. `overwrite` 会清空并重建正文，旧 block ID 会失效，当前版本中未包含在目标 fetch 响应里的评论等非正文对象可能丢失。用户明确要求整篇版本恢复时可以替换正文；若还要求保留当前评论或其它未包含在 fetch 响应里的对象，先说明该 fallback 无法保证保留并确认，不得宣称无损恢复。
7. 恢复完成后，用 `docs +fetch --detail full --format json` 读取最新完整文档，验证 `content` 和可回放的 `reference_map` 均与目标 revision 的真实返回一致。

> 路径是二选一：精确 history entry → `history-revert`；无精确 entry但目标 revision 可读 → `fetch target revision` → `overwrite` → `fetch latest verify`。不要猜邻近版本，也不要用重复 help、重复读取 Skill/Reference 或大型探针脚本替代这条有界流程。

## 按 revision_id 或时间点回滚

当用户说“回滚到 revision_id=42”“恢复到昨天下午 3 点的版本”这类需求时，流程是：

1. 执行 `docs +history-list --doc <doc>` 获取第一页历史记录；`+history-list` 是分页接口，只有 `has_more=true` 且还需要更多候选时才继续传 `--page-token` 翻页。
2. 如果用户给出 `revision_id`：先筛选当前页中 `entries[].revision_id == 用户给出的 revision_id`。如果未命中且 `has_more=true`，继续拉下一页；如果已经命中候选，最多额外再拉一页，补齐同一个 `revision_id` 可能跨页出现的相邻 `history_version_id`。若用户同时给出目标时间，在候选里选择 `edit_time` 与目标时间最接近的一条；若未给目标时间但候选只有一条，可直接使用；若多个候选无法可靠区分，不要自行取第一条，向用户展示候选并确认。
3. 如果用户只给出时间：用 `entries[].edit_time` 匹配，选择目标时刻之前最近的一条；如果用户表达的是“最接近某时刻”，则选择绝对时间差最小的一条。
4. 如果存在最终精确匹配条目，从该条目读取 `history_version_id`。`history_version_id` 对应服务端 `minor_history.version`，这是回滚接口需要的 ID。
5. 有精确匹配时，执行 `docs +history-revert --doc <doc> --history-version-id <history_version_id>`，完成后 fetch 最新完整文档核验。
6. 没有精确匹配且用户给出了目标 `revision_id` 时，执行以下 fallback；只有目标 revision 确实可读，才可覆盖当前文档：

```bash
# 在子 shell 中 fail closed，避免任一校验失败后继续执行 overwrite
(
set -eu

# 每个恢复任务使用独占临时目录；target/current/latest 各用独立工作目录隔离 fetch sidecar
doc_restore_tmp=$(mktemp -d "${TMPDIR:-/tmp}/lark-doc-restore.XXXXXX") || exit 1
readonly doc_restore_tmp
trap 'rm -rf -- "$doc_restore_tmp"' EXIT
mkdir "$doc_restore_tmp/target" "$doc_restore_tmp/current" "$doc_restore_tmp/latest"

# 读取目标 revision，并将同一响应的正文与 sidecar 分开保存
target_revision_id="<target_revision_id>"
(
  cd "$doc_restore_tmp/target"
  lark-cli docs +fetch --doc "<doc>" --revision-id "$target_revision_id" --detail full --format json > target-revision.json
)
jq -e --arg revision_id "$target_revision_id" \
  '.data.document.revision_id | tostring == $revision_id' \
  "$doc_restore_tmp/target/target-revision.json" > /dev/null
jq -j '.data.document.content' "$doc_restore_tmp/target/target-revision.json" > "$doc_restore_tmp/target/target-content.xml"
jq '.data.document.reference_map // {}' "$doc_restore_tmp/target/target-revision.json" > "$doc_restore_tmp/target/target-reference-map.json"

# 写前读取当前 revision_id；将它作为乐观锁，避免静默覆盖并发编辑
(
  cd "$doc_restore_tmp/current"
  lark-cli docs +fetch --doc "<doc>" --detail full --format json > current-document.json
)
target_document_id=$(jq -er '.data.document.document_id' "$doc_restore_tmp/target/target-revision.json")
current_document_id=$(jq -er '.data.document.document_id' "$doc_restore_tmp/current/current-document.json")
if [ "$target_document_id" != "$current_document_id" ]; then
  printf 'target/current document_id mismatch; refusing overwrite\n' >&2
  exit 1
fi
current_revision_id=$(jq -er '.data.document.revision_id' "$doc_restore_tmp/current/current-document.json")

# 从 target 工作目录配套回放正文与 sidecar，确保 reference_map 的相对路径解析到目标快照
# 只使用 +update --help 中定义的参数；该命令没有 --yes，不要添加未定义的确认参数
(
  cd "$doc_restore_tmp/target"
  lark-cli docs +update --doc "<doc>" --command overwrite \
    --revision-id "$current_revision_id" \
    --content @target-content.xml \
    --reference-map @target-reference-map.json
)

# 读取最新完整文档，并与目标 revision 的 content/reference_map 比对
(
  cd "$doc_restore_tmp/latest"
  lark-cli docs +fetch --doc "<doc>" --detail full --format json > latest-document.json
)

python3 - \
  "$doc_restore_tmp/target/target-revision.json" \
  "$doc_restore_tmp/latest/latest-document.json" \
  "$doc_restore_tmp/target" \
  "$doc_restore_tmp/latest" <<'PY'
import copy
import json
import re
import sys
from pathlib import Path

def document(path):
    with open(path, encoding="utf-8") as stream:
        return json.load(stream)["data"]["document"]

def normalize_content(value):
    return re.sub(r'\s+id="[^"]*"', "", value or "")

def materialize_paths(value, base):
    value = copy.deepcopy(value or {})
    if isinstance(value, dict):
        path = value.get("path")
        if isinstance(path, str) and path.startswith("@"):
            relative = path[1:]
            candidate = (base / relative).resolve()
            if base.resolve() not in candidate.parents:
                raise SystemExit(f"reference path escapes snapshot directory: {relative}")
            value["data"] = candidate.read_text(encoding="utf-8")
            value["path"] = ""
        return {key: materialize_paths(item, base) for key, item in value.items()}
    if isinstance(value, list):
        return [materialize_paths(item, base) for item in value]
    return value

target = document(sys.argv[1])
latest = document(sys.argv[2])
content_matches = normalize_content(target.get("content")) == normalize_content(latest.get("content"))
target_references = materialize_paths(target.get("reference_map"), Path(sys.argv[3]))
latest_references = materialize_paths(latest.get("reference_map"), Path(sys.argv[4]))
references_match = target_references == latest_references
print(json.dumps({"content_matches": content_matches, "reference_map_matches": references_match}))
if not content_matches or not references_match:
    raise SystemExit("latest document does not match target revision")
PY
)
```

`mktemp -d` 生成的目录只属于当前恢复任务；退出时只清理该目录，不要删除共享工作区中的固定文件。若目标 revision 的完整内容包含引用或资源块，必须按 `lark-doc-fetch.md` 返回的 `reference_map` 和 `lark-doc-update.md` 的保真规则一并回放，不得降级成纯文本或占位符。若服务端因 revision 冲突拒绝写入，重新检查当前内容并向用户说明，不要自动覆盖。

候选确认时使用类似格式：

```text
同一个 revision_id 命中多个历史版本，请确认要回滚哪一条：
- history_version_id=11 revision_id=42 edit_time=2026-06-22T12:24:45Z name=...
- history_version_id=12 revision_id=42 edit_time=2026-06-22T12:25:14Z name=...
```

## 命令

```bash
# 列出历史版本
lark-cli docs +history-list --doc "<docx_url_or_token>" --page-size 20

# 翻页
lark-cli docs +history-list --doc "<docx_url_or_token>" --page-size 20 --page-token "<page_token>"

# 回滚到指定 history_version_id（默认等待 30000ms）
lark-cli docs +history-revert --doc "<docx_url_or_token>" --history-version-id 42

# 只发起任务，不等待
lark-cli docs +history-revert --doc "<docx_url_or_token>" --history-version-id 42 --wait-timeout-ms 0

# 查询回滚任务状态
lark-cli docs +history-revert-status --doc "<docx_url_or_token>" --task-id "<task_id>"
```

## 参数

| 命令 | 参数 | 必填 | 说明 |
|-|-|-|-|
| `+history-list` | `--doc` | 是 | Docx URL/token，或可解析为 Docx 的 wiki URL |
| `+history-list` | `--page-size` | 否 | 返回条数，范围 `1-20`，默认 `20` |
| `+history-list` | `--page-token` | 否 | 上一页返回的 `page_token` |
| `+history-revert` | `--doc` | 是 | Docx URL/token，或可解析为 Docx 的 wiki URL |
| `+history-revert` | `--history-version-id` | 是 | `+history-list` 返回的 `history_version_id`，必须大于 0 |
| `+history-revert` | `--wait-timeout-ms` | 否 | 等待回滚完成的毫秒数，范围 `0-30000`，默认 `30000` |
| `+history-revert-status` | `--doc` | 是 | 同一个文档 |
| `+history-revert-status` | `--task-id` | 是 | `+history-revert` 返回的 `task_id` |

## 返回值要点

`+history-list` 返回：

```json
{
  "entries": [
    {
      "revision_id": 42,
      "history_version_id": "11",
      "edit_time": "1780000000",
      "type": 1,
      "name": "版本名",
      "description": "版本说明",
      "editor_ids": ["ou_xxx"]
    }
  ],
  "has_more": true,
  "page_token": "page_token"
}
```

`+history-revert` 返回：

```json
{
  "task_id": "task_xxx",
  "status": "running",
  "history_version_id": "11",
  "poll_after_ms": 10000
}
```

`+history-revert-status` 返回：

```json
{
  "status": "partial_failed",
  "history_version_id": "11",
  "failed_block_tokens": ["blk_xxx"]
}
```

`status` 可能是 `running`、`done`、`partial_failed`、`failed`。当状态是 `partial_failed` 或 `failed` 时，优先检查 `failed_block_tokens`。
