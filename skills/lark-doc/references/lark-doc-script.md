# `docs +script`

## 脚本列表

| `--command` | 用途 |
|-|-|
| `init-draft` | 创建带 Presentation Decision 基线的独占工作区，并预留尚不存在的 XML 路径。 |
| `parse` | 解析本地或在线文档，返回画像并检查决策与资源。 |

每个脚本只使用其小节列出的专用参数；所有脚本均可使用文末的通用参数。

## `init-draft`

### 参数

| 参数 | 必填 | 用法 |
|-|-|-|
| `--command init-draft` | 是 | 选择本脚本。 |
| `--presentation-decision` | 是 | 决策 JSON；接受内联 JSON、`@相对或绝对路径` 或 `-`（stdin）。 |

```bash
lark-cli docs +script --command init-draft \
  --presentation-decision '<完整 Presentation Decision JSON>' \
  --format json
```

`data` 的结构如下；实际随机段为 8 位十六进制字符：

```json
{
  "cwd": "/path/to/current-directory",
  "workspace": "draft_a1b2c3d4_folder",
  "draft_path": "draft_a1b2c3d4_folder/draft.xml",
  "tip": "Workspace created; draft XML does not exist yet. Write it directly to <cwd>/<draft_path>. Prefer workspace for new assets and reuse existing files. Prefer @relative paths within cwd and @absolute paths elsewhere, subject to file access rules. Relative resource paths try cwd first, then the source XML directory only if missing. Keep CLI calls in cwd."
}
```

- 在生成正文前执行；不要自行创建工作目录或决策文件。CLI 固定生成 `draft_<8位十六进制字符>_folder/draft.xml`，以返回的实际路径为准。
- 决策必须是单个 JSON 对象，包含 `visual_plan.blocks`。`audience`、`reader_task`、`genre_contract`、`adapter`、`presentation_mode`、`visual_plan.reason` 和每个 block 的 `purpose` 是可选描述信息，可省略、为空字符串或 `null`；不参与通过/失败判定。
- `visual_plan.blocks` 为数组；每项的硬约束为 `{type,min_count}`，`type` 不重复，`min_count` 为正整数。按本 Skill 创建文档时，`blocks` 只对 `whiteboard`、`img`、`html5-block` 设置最低数量，其他表达按内容需要使用但不设数量约束；三类均无需约束时写 `[]`。CLI 为外部决策兼容 `type: "list"`，检查时将 `<ul>` 与 `<ol>` 的数量相加。仅有字数要求时添加 `word_count: {min,max}`；未指定的一侧写 `null`，至少一侧为正整数，且 `min <= max`。
- 返回 `data.cwd`（本次文件操作的绝对工作目录）、`data.workspace`（相对工作区）、`data.draft_path`（相对 XML 路径）和操作提示 `data.tip`。工作区及其中的 `.presentation-decision.json` 已存在，XML 尚不存在；直接写入 `<cwd>/<draft_path>`，首次写入前不要读取该路径。后续 CLI 使用返回的 `cwd`；资源路径规则见 [lark-doc](../SKILL.md)。
- 后续始终使用 `draft_path`，不得另建 XML、复用其他任务的路径或修改工作区中的 `.presentation-decision.json`；保留 `workspace` 及其中的创作草稿。

## `parse`

### 参数

| 参数 | 必填 | 用法 |
|-|-|-|
| `--command parse` | 是 | 选择本脚本。 |
| `--content` | 二选一 | 本地 XML 的字面内容、`@相对或绝对路径` 或 `-`（stdin）。 |
| `--doc` | 二选一 | 在线 Docx/Wiki URL 或 token；与 `--content` 互斥。 |
| `--presentation-decision` | 否 | 用于检查当前输入的决策 JSON；支持内联、`@相对或绝对路径` 或 `-`。 |

```bash
lark-cli docs +script --command parse --content "@./document.xml" --format json
lark-cli docs +script --command parse --doc "<Docx/Wiki URL 或 token>" --format json
lark-cli docs +script --command parse --content "@./document.xml" --presentation-decision '<JSON>' --format json
```

- `--content` 与 `--presentation-decision` 同时使用时，最多一个参数读取 stdin。
- 决策格式同 `init-draft`：只强制检查字数范围、块类型及最低数量；描述字段可省略或为空。`visual_plan.blocks` 的 `type` 不重复；兼容的 `list` 约束按 `<ul>` 与 `<ol>` 的合计数量检查。
- 使用 `--content "@./<init-draft 返回的 data.draft_path>"` 时自动加载保存的决策；显式 `--presentation-decision` 优先。
- `--doc` 需要 `docx:document:readonly`；`--content` 不调用 OpenAPI。
- 返回 `data.assessment.status`、`data.profile` 和按需出现的 `data.diagnostics[]`；profile 包含 `word_count`、`char_count`、`block_count` 和 `blocks[]`。顶层 `ok` 只表示命令是否成功执行。画像、决策或资源预检未通过时，命令仍以 `ok:true` 和退出码 0 返回，但 `assessment.status` 为 `failed`；每条 diagnostic 提供 `severity`、稳定 `code`、`msg`、可选 `expected` / `actual` 和 `suggested`。同一原因失败的远程图片合并为一条 diagnostic，并在 `image_indices[]` 中列出图片序号，避免重复提示。修复后重新解析，直到 `assessment.status` 为 `passed`。
- `parse` 不是 XML/SDK schema validator。成功且无 warning 也不保证服务端接受；写入前仍须按 XML 规则复查。

## 所有脚本通用参数

| 参数 | 用法 |
|-|-|
| `--as user|bot` | 选择身份。 |
| `--dry-run` | 只返回执行计划，不联网、解析或写文件。 |
| `--format` | 输出格式：`json|pretty|table|ndjson|csv`；模型使用默认的 `json`。 |
| `--json` | `--format json` 的别名。 |
| `--jq` / `-q` | 裁剪 JSON；不得与非 JSON 格式同时使用。 |
| `-h` / `--help` | 查看帮助。 |
