# docs history（历史版本与回滚）

用于查看 Docx 历史版本、按 `history_version_id` 回滚，以及查询回滚任务状态。

`entries[].edit_time` 是 RFC3339 时间字符串（例如 `2026-06-22T12:24:45Z`）。按时间匹配时先将其解析为时间值，再比较先后关系或时间差。

## 安全约束

- `overwrite` 会重建正文和 block ID，且无法保证保留评论等非正文对象。用户要求保留这些对象时，应先说明限制并确认。
- `overwrite` 返回 warning 或 `partial_success` 时，先核验最新内容。核验失败或发生 revision conflict 时停止，不要再次覆盖。
- 权限、网络或临时系统错误应保留原错误分类，不得解释为目标版本不存在。

## 按 revision_id 或时间点回滚

1. 使用 `+history-list` 定位目标记录。需要更多候选时，根据 `has_more` 和 `page_token` 翻页。
   - 用户指定 `revision_id`：逐页筛选相同 `revision_id` 的记录。未命中时必须继续翻页至 `has_more=false` 才可进入 fallback；命中位于页尾时，继续读取下一页以收集相邻的同 `revision_id` 候选。多条记录时结合 `edit_time` 选择；无法区分时请用户确认。
   - 用户指定时间：选择不晚于目标时间的最近一条记录；用户明确要求“最接近”时，选择时间差最小的记录。
2. 找到目标记录后，使用该记录的 `history_version_id` 调用 `+history-revert`。不要将 `revision_id` 传给回滚接口。返回 `running` 时使用 `+history-revert-status` 查询；只有 `done` 表示成功，其他终态均停止并报告。
3. 没有目标记录但用户指定了 `revision_id` 时，可读取目标版本并恢复正文：
   - 使用 `docs +fetch --doc "<doc>" --revision-id <revision_id> --scope full --detail full --format json` 读取目标版本。确认文档一致、返回的 `revision_id` 与目标一致，且 `content` 不是 `<fragment>`。
   - 使用 `docs +fetch --doc "<doc>" --scope full --detail full --format json` 读取当前完整文档，其 `content` 同样不得是 `<fragment>`。目标与当前响应的 `revision_id` 相同时直接结束，不执行 `overwrite`。否则移除目标 `content` 中旧的 block ID，将正文写入任务目录下的相对路径，然后仅执行一次 `docs +update --doc "<doc>" --command overwrite --revision-id <current_revision_id> --content @target.xml`，其中 `current_revision_id` 来自当前文档响应。目标响应包含非空 JSON object 形式的 `reference_map` 时，将其写入相对路径并追加 `--reference-map @target-reference-map.json`；否则省略该参数。`+update` 不支持 `--yes`。
   - 使用 `docs +fetch --doc "<doc>" --scope full --detail full --format json` 读取最新完整文档并核验。忽略重新生成的 block ID，正文结构、文本、链接和引用资源应与目标版本一致。
4. 目标版本明确不可读时停止并报告。

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
      "edit_time": "2026-06-22T12:24:45Z",
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
