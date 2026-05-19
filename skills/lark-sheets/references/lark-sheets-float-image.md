# Lark Sheet Float Image

> **单元格图片 vs 浮动图片**：飞书表格有两种图片类型，请根据需求选择正确的工具：
> - **单元格图片**：图片嵌入在单元格内部，随单元格移动，属于单元格内容的一部分。→ 使用 `+cells-set`，在 `rich_text` 中设置 `type: "embed-image"`（见 lark_sheet_write_cells Skill）。
> - **浮动图片**（本 Skill）：图片悬浮在单元格上方，可自由指定位置、大小和层级，不属于任何单元格的内容。→ 使用本 Skill 的 `+float-{image-create|image-update|image-delete}`。

## 真对象硬约束

当用户要求"插入图片 / 添加 logo / 放一张图"时，**必须**通过 `+float-{image-create|image-update|image-delete}`（浮动图片）或 `+cells-set` 的 `embed-image`（单元格图片）创建真实的图片对象。**禁止**只在文本回复中给出图片链接 / 描述图片内容代替插入。判断标准：交付后 `+float-image-list` 或单元格 `rich_text` 必须能读到该图片对象。

## 使用场景

读写**浮动图片**对象（悬浮在单元格上方的图片，不属于单元格内容）。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有浮动图片 | `+float-image-list` | 获取浮动图片的位置、大小和层级配置 |
| 创建/更新/删除浮动图片 | `+float-{image-create|image-update|image-delete}` | 对浮动图片执行写入操作 |

典型工作流：先读取现有浮动图片了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

**常见配置错误（必须注意）**：
- **单元格图片 vs 浮动图片选择错误**：如果用户希望图片嵌入单元格内部（随单元格移动），应使用 `+cells-set` 的 `rich_text` + `embed-image`，而非本 Skill
- **图片位置参数要精确**：锚点单元格的行列索引和偏移量决定了图片位置，设置不当会导致图片遮挡数据
- **创建后必须验证**：调用 `+float-image-list` 确认图片位置和大小正确

reference_id 的映射规则：
- `image_uri`：`<|image|>:abcdef` 或者 `<|superscript|>:abcdef-<|image|>:abcdef`
- `float_image_id`：`<|float_image|>:abcdef`
其中 `abcdef` 为实际的对象 ID，占位符仅用于示意，不可直接使用。

`image_uri` 与 `image_token` 是「指定图片资源」的两种等价方式（与 `+cells-set` 中 `embed-image` 的语义一致）：
- `image_uri`：上传链路给到的图片 reference_id（如 `<|image|>:abcdef`），由系统自动转 fileToken
- `image_token`：图片 fileToken，常见来源是 `+float-image-list` 返回的 `image_token`（适合"换皮不换位置"等基于已有图片的复用场景）
- create 时二者必须有其一；update 时**仅在需要替换图片本身时**传入新的 `image_uri` 或 `image_token`，不传则保留原图。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+float-image-list` | read | 对象 |
| `+float-image-create` | write | 对象 |
| `+float-image-update` | write | 对象 |
| `+float-image-delete` | high-risk-write | 对象 |

## Flags

### `+float-image-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--float-image-id` | string | 否 | 按 id 过滤；省略时列工作表全部 |

### `+float-image-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--image-name` | string | 是 | 图片名称，含拓展名（如 `logo.png`） |
| `--image-token` | string | XOR | 图片 file_token（与 `--image-uri` 二选一）。常见来源：`+float-image-list` 返回的 `image_token` |
| `--image-uri` | string | XOR | 图片 reference_id（与 `--image-token` 二选一）；形如 `<\|image\|>:abcdef` 这种带前缀的字符串，从上游 SKILL.md 的素材引用约定取 |
| `--position-row` | int | 是 | 图片左上角所在行（0-based） |
| `--position-col` | string | 是 | 图片左上角所在列（列字母，如 `A` / `B`） |
| `--size-width` | int | 是 | 图片宽度（像素） |
| `--size-height` | int | 是 | 图片高度（像素） |
| `--offset-row` | int | 否 | 在 position 基础上的行内偏移（像素） |
| `--offset-col` | int | 否 | 在 position 基础上的列内偏移（像素） |
| `--z-index` | int | 否 | 图片 Z 轴层级，控制重叠顺序 |

### `+float-image-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--float-image-id` | string | 是 | 目标图片 id |
| `--image-name` | string | 是 | 图片名称，含拓展名（如 `logo.png`） |
| `--image-token` | string | XOR | 图片 file_token（与 `--image-uri` 二选一）。常见来源：`+float-image-list` 返回的 `image_token` |
| `--image-uri` | string | XOR | 图片 reference_id（与 `--image-token` 二选一）；形如 `<\|image\|>:abcdef` 这种带前缀的字符串，从上游 SKILL.md 的素材引用约定取 |
| `--position-row` | int | 是 | 图片左上角所在行（0-based） |
| `--position-col` | string | 是 | 图片左上角所在列（列字母，如 `A` / `B`） |
| `--size-width` | int | 是 | 图片宽度（像素） |
| `--size-height` | int | 是 | 图片高度（像素） |
| `--offset-row` | int | 否 | 在 position 基础上的行内偏移（像素） |
| `--offset-col` | int | 否 | 在 position 基础上的列内偏移（像素） |
| `--z-index` | int | 否 | 图片 Z 轴层级，控制重叠顺序 |

### `+float-image-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--float-image-id` | string | 是 | 目标图片 id |

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。浮动图片是 sheet 级对象——和单元格内嵌图片不同（后者走 `+cells-set`）。

### `+float-image-list`

### `+float-image-create`

所有字段拍平为独立 flag：`--image-name` / `--image-token` 或 `--image-uri`（XOR） / `--position-{row,col}` / `--size-{width,height}` / `--offset-{row,col}` / `--z-index`。

```bash
# 用 file_token（从 +float-image-list 返回的 image_token 或独立上传得到）
lark-cli sheets +float-image-create --url "..." --sheet-id "$SID" \
  --image-name "logo.png" --image-token "$TOKEN" \
  --position-row 0 --position-col A --size-width 200 --size-height 150

# 用 reference_id（部分租户直接引用）
lark-cli sheets +float-image-create --url "..." --sheet-id "$SID" \
  --image-name "logo.png" --image-uri "<|image|>:abcdef" \
  --position-row 2 --position-col B --size-width 300 --size-height 200 --z-index 1
```

### `+float-image-update`

> 必须先 `+float-image-list --float-image-id <id>` 回读当前完整属性，再通过 `--image-name` / `--position-*` / `--size-*` 等独立 flag 改对应字段。

### `+float-image-delete`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+float-image-create` 校验 `--image-name` 非空，`--image-token` 与 `--image-uri` 互斥且至少一个非空，`--position-row/col` 与 `--size-width/height` 必填且为合法整数；`+float-image-update` 必须 `--float-image-id`；`+float-image-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 float_image 请求模板"。
- `Execute`：写后调用 `+float-image-list --float-image-id <id>` 回读，envelope.meta.verification 给出新位置 / 尺寸对比。
