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
  --presentation-decision '{}' \
  --format json
```

路径返回值与用法见 [创建工作流 Step 4](lark-doc-create-workflow.md)。

- 在生成正文前执行；不要自行创建工作目录或决策文件。CLI 固定生成 `draft_<8位十六进制字符>_folder/draft.xml`，以返回的实际路径为准。
- 决策是单个 JSON 对象；无可量化约束时传 `{}`。`audience`、`reader_task`、`genre_contract`、`adapter`、`presentation_mode`、`visual_plan.reason` 和每个 block 的 `purpose` 是可选描述信息，可省略、为空字符串或 `null`；不参与通过/失败判定。
- `visual_plan`、`visual_plan.blocks`、每项的 `type` 和 `min_count` 都可省略或为 `null`，表示未设置。`blocks` 写 `[]` 也表示无数量约束；条目缺少 `type` 或 `min_count` 时，不启用该条数量校验。
- 显式填写的字段仍须合法：`visual_plan` 为对象，`blocks` 为数组，`type` 为支持的块类型，`min_count` 为正整数。空字符串类型、未知块类型、零/负数或非整数数量会报错，即使另一字段缺失也一样。仅 `type` 与 `min_count` 都有效的条目参与数量检查；启用的条目不能重复声明同一类型。兼容的 `list` 约束按 `<ul>` 与 `<ol>` 的合计数量检查。
- `word_count` 仅在需要字数校验时填写 `{min,max}`；未指定的一侧写 `null`，至少一侧为正整数，且 `min <= max`。没有字数要求时省略整个字段。
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
- 决策格式同 `init-draft`：缺失的约束不检查，显式填写但非法的值报错，合法且完整的约束参与检查。`passed` 只表示本次启用的检查通过，不代表未声明的要求已满足。
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
