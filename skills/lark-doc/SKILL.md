---
name: lark-doc
version: 2.0.0
description: "飞书云文档（Docx / Wiki 文档，v2 API）：读取和编辑飞书文档内容。当用户给出文档 URL 或 token，或需要查看、创建、编辑文档、插入或下载文档图片附件时使用。文档中嵌入的电子表格、多维表格、画板，先用本 skill 提取 token 再切到对应 skill。当用户给出 doubao.com 的 /docx/ 或 /wiki/ URL/token 时，也应直接使用本 skill；路由依据是 URL 路径模式和 token，而不是域名。不负责文档评论管理，也不负责表格或 Base 的数据操作。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli docs --api-version v2 --help; lark-cli docs +create --api-version v2 --help; lark-cli docs +fetch --api-version v2 --help; lark-cli docs +update --api-version v2 --help; lark-cli docs resource-download --help; lark-cli docs resource-update --help; lark-cli docs resource-delete --help"
---

# docs (v2)

**身份：文档操作默认使用 `--as user`。首次使用前执行 `lark-cli auth login`。**

> **CRITICAL — API 版本：本 skill 使用 v2 API。执行 `docs +create`、`docs +fetch`、`docs +update` 时必须显式传入 `--api-version v2`。**

```bash
# 常用示例
lark-cli docs +fetch  --api-version v2 --doc "文档URL或token"
lark-cli docs +create --api-version v2 --content '<title>标题</title><p>内容</p>'
lark-cli docs +update --api-version v2 --doc "文档URL或token" --command append --content '<p>内容</p>'
```

## 前置条件 — 执行操作前必读

**CRITICAL — 执行对应操作前，MUST 先用 Read 工具读取以下文件，缺一不可：**
1. [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、权限处理、全局参数（所有操作通用）
2. **读取文档（`docs +fetch --api-version v2`）** → 必读 [`lark-doc-fetch.md`](references/lark-doc-fetch.md)（`--scope` / `--detail` 选择、局部读取策略、`<fragment>` / `<excerpt>` 输出结构）
3. **创建或编辑文档内容** → 必读 [`lark-doc-xml.md`](references/lark-doc-xml.md)（XML 语法规则，仅当用户明确要求 Markdown 时改读 [`lark-doc-md.md`](references/lark-doc-md.md)）；从零创建时加读 [`lark-doc-create-workflow.md`](references/style/lark-doc-create-workflow.md)；编辑已有文档时加读 [`lark-doc-update-workflow.md`](references/style/lark-doc-update-workflow.md)
4. **需要使用 callout、grid、table、whiteboard 等富 block 时** → 参考 [`lark-doc-style.md`](references/style/lark-doc-style.md) 的元素能力说明。该文件不是固定模板或强制排版规范；除非用户明确要求美化、重排版或特定风格，不要为了“达标”主动套用固定结构。

**未读完以上文件就执行相应操作会导致参数选择错误或格式错误。**

> **格式选择规则（全局）：**
> - **创建 / 导入场景**（`docs +create`，或 `docs +update --command append/overwrite` 的整段写入）：XML 和 Markdown 都可以。用户提供 `.md` 本地文件、或明确说"导入 Markdown"时，直接用 Markdown；否则默认 XML（可用 callout、grid、checkbox 等富 block）。
> - **精准编辑场景**（`docs +update` 的 `str_replace` / `block_insert_after` / `block_replace` / `block_delete` / `block_move_after` 等局部精修指令）：优先使用 XML（`--doc-format xml`，即默认值）。XML 能稳定表达 block 结构和样式，局部精修更可控；不要因为 Markdown 更简单就自行切换。

## 快速决策
- 用户需要“某个 block 的直达链接 / 锚点链接”时：返回 `文档基础 URL#block_id`。如果当前只有文档 URL 没有 block_id，先用 `docs +fetch --detail with-ids` 拿到目标 block 的 id
- 例：
  - 已知文档 URL = `https://xxx.feishu.cn/docx/doxcn123`
  - 已知 block_id = `blkcn456`
  - 应返回 `https://xxx.feishu.cn/docx/doxcn123#blkcn456`
- 用户需要在文档内**创建、复制或移动**资源块（画板、电子表格、多维表格等）时，必须先读取 [`lark-doc-xml.md`](references/lark-doc-xml.md) 的「三、资源块」章节
- 写文档时，由内容和用户意图决定表达形式；流程、架构、路线图、关键指标等信息可以使用画板，但不要默认把重要信息都画板化
- 新增画板必须隔离到 SubAgent：简单图由 SubAgent 直接插入 `<whiteboard type="svg">完整 SVG</whiteboard>`，不读 `lark-whiteboard`；复杂图才由主 Agent 先建 `<whiteboard type="blank"></whiteboard>`，再启动 SubAgent 读取 `lark-whiteboard` 写入
- 用户说"看一下文档里的图片/附件/素材""预览素材" → 用 `lark-cli docs +media-preview`
- 用户明确说"下载素材" → 用 `lark-cli docs +media-download`
- 用户明确说"下载/更新/删除文档封面图" → 用 `lark-cli docs resource-download/resource-update/resource-delete --type cover`
- `resource-*` 目前仅支持 Docx 封面资源；其他图片、附件或素材请走 `+media-*`
- 如果目标是画板/whiteboard/画板缩略图 → 只能用 `lark-cli docs +media-download --type whiteboard`（不要用 `+media-preview`）
- 拿到 spreadsheet URL/token 后 → 切到 `lark-sheets` 做对象内部操作
- 用户说"给文档加评论""查看评论""回复评论""给评论加/删除表情 reaction" → 切到 `lark-drive` 处理
- 文档内容中出现嵌入的 `<sheet>`、`<bitable>` 或 `<cite file-type="sheets|bitable">` 标签时 → **必须主动提取 token 并切到对应技能下钻读取内部数据**，不能只呈现标签本身

| 标签 / 属性 | 提取字段 | 切到技能 |
|-|-|-|
| `<sheet token="..." sheet-id="...">` | `token` -> spreadsheet_token, `sheet-id` | [`lark-sheets`](../lark-sheets/SKILL.md) |
| `<bitable token="..." table-id="...">` | `token` -> app_token, `table-id` | [`lark-base`](../lark-base/SKILL.md) |
| `<cite type="doc" file-type="sheets" token="..." sheet-id="...">` | 同 `<sheet>` | [`lark-sheets`](../lark-sheets/SKILL.md) |
| `<cite type="doc" file-type="bitable" token="..." table-id="...">` | 同 `<bitable>` | [`lark-base`](../lark-base/SKILL.md) |
| `<vc-transcribe-tab vc-node-id="...">` | `vc-node-id` -> note_id | [`lark-note`](../lark-note/SKILL.md)：先 `note +detail --note-id <vc-node-id>` |
| `<synced_reference src-token="..." src-block-id="...">` | `src-token` -> doc_token, `src-block-id` -> block_id | 用 `docs +fetch --api-version v2` 读取 src-token 文档，定位 block |

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli docs +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-doc-create.md) | Create a Lark document (XML / Markdown) |
| [`+fetch`](references/lark-doc-fetch.md) | Fetch Lark document content (XML / Markdown) |
| [`+update`](references/lark-doc-update.md) | Update a Lark document (str_replace / block_insert_after / block_replace / ...) |
| [`+media-insert`](references/lark-doc-media-insert.md) | Insert a local image or file at the end of a Lark document (4-step orchestration + auto-rollback). Prefer `--from-clipboard` when the image is already on the system clipboard (screenshots, copy from Feishu/browser); use `--file` only for on-disk sources. |
| [`+media-download`](references/lark-doc-media-download.md) | Download document media or whiteboard thumbnail (auto-detects extension) |
| [`+media-preview`](references/lark-doc-media-preview.md) | Preview document media file (auto-detects extension) |
| [`resource-download` / `resource-update` / `resource-delete`](references/lark-doc-resource-cover.md) | Download, update, or delete a Docx cover image resource with `--type cover` |
| [`+whiteboard-update`](../lark-whiteboard/references/lark-whiteboard-update.md) | Alias of `whiteboard +update`. Update an existing whiteboard with DSL, Mermaid or PlantUML. Prefer `whiteboard +update`; refer to lark-whiteboard skill for details. |

## 不在本 Skill 范围

- 文档评论管理 → [`lark-drive`](../lark-drive/SKILL.md)
- 电子表格或 Base 的数据操作 → [`lark-sheets`](../lark-sheets/SKILL.md) / [`lark-base`](../lark-base/SKILL.md)
- 云空间文件上传、下载、权限管理 → [`lark-drive`](../lark-drive/SKILL.md)
