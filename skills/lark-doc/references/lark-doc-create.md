# docs +create（创建飞书云文档）

> **前置条件（MUST READ）：** 生成文档内容前，必须先用 Read 工具读取以下文件，缺一不可：
> 1. [`lark-doc-xml.md`](lark-doc-xml.md) — XML 语法规则（使用 Markdown 格式时改读 [`lark-doc-md.md`](lark-doc-md.md)）
> 2. [`lark-doc-style.md`](style/lark-doc-style.md) — 写作原则（默认段落、按体裁、组件克制）
> 3. [`lark-doc-create-workflow.md`](style/lark-doc-create-workflow.md) — 从零创作工作流（Code-Act Loop、单 Agent 串行撰写）
>
> **未读完以上文件就生成内容会导致格式错误。**

从 XML（默认）或 Markdown 内容创建一个新的飞书云文档。

> **⚠️ 格式选择规则：** 创建 / 导入场景下 XML 和 Markdown 都可以——用户提供 `.md` 本地文件、或明确说"导入 Markdown"时，直接用 Markdown；没有明确指示时默认 XML（表达能力更强，可承载更丰富的结构化内容）。不要在用户没要求的情况下主动从 XML 切到 Markdown，也不要在用户已给出 Markdown 时强行改成 XML。

## 命令

```bash
# 创建 XML 文档（默认格式，推荐）
lark-cli docs +create --content '<title>项目计划</title><h1>目标</h1><p>记录本周重点。</p>'

# 仅当用户明确要求导入 Markdown 时才使用；文档标题用 --title，正文标题按内容自然组织
lark-cli docs +create --doc-format markdown --title "项目计划" --content $'## 目标\n\n- 明确重点\n- 记录待办'
```

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "document": {
      "document_id": "docx_token",
      "revision_id": 1,
      "url": "https://xxx.feishu.cn/docx/docx_token",
      "new_blocks": [
        { "block_id": "blkcnXXXX", "block_type": "whiteboard", "block_token": "boardXXXX" }
      ]
    }
  }
}
```

- **`document.new_blocks`**：本次操作新增的 block 列表（如画板）。`block_id` 可用于 `docs +update` 的 `--block-id` 做精确编辑；`block_token` 是资源块（如画板）的 token，可交给 `lark-whiteboard` 等 skill 继续操作

> \[!IMPORTANT]
> 如果文档是**以应用身份（bot）创建**的，如 `lark-cli docs +create --as bot` 在文档创建成功后，CLI 会**尝试为当前 CLI 用户自动授予该文档的 `full_access`（可管理权限）**。
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该文档的可管理权限
> - `status = skipped`：本地没有可用的当前用户 `open_id`，因此不会自动授权；可提示用户先完成 `lark-cli auth login`，再让 AI / agent 继续使用应用身份（bot）授予当前用户权限
> - `status = failed`：文档已创建成功，但自动授权用户失败；会带上失败原因，并提示稍后重试或继续使用 bot 身份处理该文档
>
> `permission_grant.perm = full_access` 表示该资源已授予”可管理权限”。
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数                  | 必填 | 说明                                          |
| ------------------- | -- |---------------------------------------------|
| `--title`           | 否  | 文档标题，Markdown 导入时使用；XML 创建推荐在 `--content` 开头写 `<title>...</title>`；多个标题仅保留第一个并在 `warnings` / `degrade_details` 提示 |
| `--content`         | 视情况 | 文档内容（XML 或 Markdown 格式）；不传 `--content` 时必须传 `--title` |
| `--reference-map` | 否 | 结构化 `reference_map` JSON object；必须与 `--content` 一起使用。普通写入优先把结构写在正文里；该参数主要用于保留或回放已有 `document.reference_map`。支持直接 JSON、`@reference-map.json`（相对路径）或 `-` 从 stdin 读取。 |
| `--doc-format`      | 否  | 内容格式：`xml`（默认，始终优先使用）\| `markdown`（仅用户明确要求时） |
| `--parent-token`    | 否  | 父文件夹或知识库节点 token（与 `--parent-position` 互斥）  |
| `--parent-position` | 否  | 父节点位置，如 `my_library`（与 `--parent-token` 互斥） |

## 最佳实践

- **较长文档**：参考 [`lark-doc-create-workflow.md`](style/lark-doc-create-workflow.md) 先建骨架再分段写入；短文档可一次写完整内容
- **表达形式**：由用户目标和内容决定。需要结构化表达时可参考 [`lark-doc-style.md`](style/lark-doc-style.md)，但不要默认套用固定开头、固定富 block 比例或固定图表

## 参考

- [`lark-doc-create-workflow.md`](style/lark-doc-create-workflow.md) — 从零创作工作流（Code-Act Loop、单 Agent 串行撰写）
- [`lark-doc-style.md`](style/lark-doc-style.md) — 文档写作原则（默认段落、按体裁、组件克制）
- [`lark-doc-xml.md`](lark-doc-xml.md) — XML 语法规范
- [`lark-doc-fetch.md`](lark-doc-fetch.md) — 获取文档
- [`lark-doc-update.md`](lark-doc-update.md) — 更新文档
- [`lark-doc-media-insert.md`](lark-doc-media-insert.md) — 插入图片/文件到文档
