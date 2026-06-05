# markdown +diff

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

比较 Drive 中原生 Markdown 的两个历史版本，或比较远端 Markdown 与本地 `.md` 草稿。需要历史版本号时，先用 [`drive +version-history`](../../lark-drive/references/lark-drive-version-history.md) 获取 `version`，不要使用 `tag`。

## 命令

```bash
# 比较两个远端版本
lark-cli markdown +diff \
  --file-token boxcnxxxx \
  --from-version 7633658129540910621 \
  --to-version 7633658129540910628

# 比较历史版本与远端最新版本
lark-cli markdown +diff \
  --file-token boxcnxxxx \
  --from-version 7633658129540910621

# 比较远端最新版本与本地草稿
lark-cli markdown +diff \
  --file-token boxcnxxxx \
  --file ./draft.md \
  --format pretty

# 比较指定远端版本与本地草稿
lark-cli markdown +diff \
  --file-token boxcnxxxx \
  --from-version 7633658129540910621 \
  --file ./draft.md

# 预览底层请求
lark-cli markdown +diff \
  --file-token boxcnxxxx \
  --from-version 7633658129540910621 \
  --to-version 7633658129540910628 \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标 Markdown 文件 token |
| `--from-version` | 否 | 基准远端版本；不传 `--file` 时必填，传 `--file` 时省略表示“远端最新 vs 本地文件” |
| `--to-version` | 否 | 目标远端版本；要求同时传 `--from-version`，且不能与 `--file` 一起使用。省略时表示远端最新版本 |
| `--file` | 否 | 本地 `.md` 文件路径；传入后进入“远端 vs 本地”比较模式 |
| `--context-lines` | 否 | unified diff 每个 hunk 前后保留的上下文行数，默认 `3` |
| `--format` | 否 | 仅支持 `json`（默认）和 `pretty` |

## 关键行为

- `--file` 存在时：
  - 省略 `--from-version` = 比较“远端最新版本 vs 本地文件”
  - 传入 `--from-version` = 比较“指定远端版本 vs 本地文件”
- `--to-version` 只能用于“远端版本 vs 远端版本”，不能与 `--file` 同时出现
- `--format pretty` 输出带颜色的 unified diff；`--format json` 返回结构化摘要和完整 diff 文本
- 无差异时：
  - `json` 输出里 `changed=false`
  - `pretty` 输出固定为 `No differences.`

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "changed": true,
    "mode": "remote_vs_remote",
    "file_token": "boxcnxxxx",
    "from_version": "7633658129540910621",
    "to_version": "7633658129540910628",
    "from_label": "a/boxcnxxxx@version:7633658129540910621",
    "to_label": "b/boxcnxxxx@version:7633658129540910628",
    "added_lines": 3,
    "deleted_lines": 2,
    "context_lines": 3,
    "hunks": [
      {
        "header": "@@ -1,6 +1,7 @@",
        "old_start": 1,
        "old_lines": 6,
        "new_start": 1,
        "new_lines": 7
      }
    ],
    "diff": "--- a/boxcnxxxx@version:7633658129540910621\n+++ b/boxcnxxxx@version:7633658129540910628\n@@ -1,2 +1,2 @@\n..."
  }
}
```

完整字段说明：

| 字段 | 层级 | 含义 |
|------|------|------|
| `ok` | 顶层 | CLI 通用成功标记；`true` 表示命令执行成功 |
| `identity` | 顶层 | 本次执行使用的身份，通常是 `user` 或 `bot` |
| `data` | 顶层 | 本次 diff 的业务结果对象 |
| `changed` | `data` | 是否存在差异；`true` 表示两侧内容不同，`false` 表示完全一致 |
| `mode` | `data` | 比较模式；`remote_vs_remote` = 远端对远端，`remote_vs_local` = 远端对本地 |
| `file_token` | `data` | 被比较的远端 Markdown 文件 token |
| `from_version` | `data` | 基准远端版本号；远端最新 vs 本地时可能为空字符串 |
| `to_version` | `data` | 目标远端版本号；当目标侧是远端最新版本或本地文件时通常为空字符串 |
| `from_label` | `data` | unified diff 基准侧标签名，会直接出现在 `diff` 文本的 `---` 头部 |
| `to_label` | `data` | unified diff 目标侧标签名，会直接出现在 `diff` 文本的 `+++` 头部 |
| `added_lines` | `data` | 新增行数统计 |
| `deleted_lines` | `data` | 删除行数统计 |
| `context_lines` | `data` | 每个 hunk 前后保留的上下文行数，对应传入的 `--context-lines` |
| `hunks` | `data` | 结构化的变更块摘要数组；每个元素对应 patch 里的一个 `@@ ... @@` 段 |
| `diff` | `data` | 完整 unified diff 文本；最适合直接阅读或保存 |
| `local_file` | `data` | 仅在 `remote_vs_local` 模式下出现；值就是传给 `--file` 的本地 Markdown 路径 |

标签字段补充：

- `from_label` / `to_label` 只用于标识 diff 两侧，不代表额外 API 字段
- `from_label` 表示基准侧，`to_label` 表示目标侧
- 远端版本通常形如 `a/<file_token>@version:<version>`、`b/<file_token>@version:<version>`
- 当目标侧是远端最新版本时，`to_label` 形如 `b/<file_token>@latest`
- 当目标侧是本地文件时，`to_label` 形如 `b/./draft.md`

`hunks` 子字段说明：

| 字段 | 含义 |
|------|------|
| `header` | 原始 hunk 头，例如 `@@ -3,1 +3,1 @@` |
| `old_start` | 旧内容从第几行开始 |
| `old_lines` | 旧内容这段覆盖多少行 |
| `new_start` | 新内容从第几行开始 |
| `new_lines` | 新内容这段覆盖多少行 |

补充说明：

- `hunks` 适合 agent 或脚本快速定位变更范围；完整逐行内容仍以 `diff` 字段为准
- `changed=false` 时，`hunks` 通常为空数组，`diff` 通常为空字符串；如果使用 `--format pretty`，终端输出会是 `No differences.`

远端 vs 本地时会额外返回：

```json
{
  "local_file": "./draft.md"
}
```

- `local_file`
  - 只有传了 `--file`、进入“远端 vs 本地”模式时才会返回
  - 值就是本次命令实际比较的本地 Markdown 路径，也就是你传给 `--file` 的那个路径
  - 它表示“目标侧本地文件”，不是临时下载文件，也不是远端文件名
  - 如果没有这个字段，说明本次是“远端版本 vs 远端版本”

## 参考

- [lark-markdown](../SKILL.md) — Markdown 域总览
- [lark-drive-version-history](../../lark-drive/references/lark-drive-version-history.md) — 获取可用于 diff 的历史版本号
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
