---
name: lark-doc
version: 1.0.0
description: "飞书云文档：创建和编辑飞书文档。从 Markdown 创建文档、获取文档内容、更新文档（追加/覆盖/替换/插入/删除）、上传和下载文档中的图片和文件、搜索云空间文档。当用户需要创建或编辑飞书文档、读取文档内容、在文档中插入图片、搜索云空间文档时使用；如果用户是想按名称或关键词先定位电子表格、报表等云空间对象，也优先使用本 skill 的 docs +search 做资源发现。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli docs --help"
---

# docs (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 核心概念

### 文档类型与 Token

飞书开放平台中，不同类型的文档有不同的 URL 格式和 Token 处理方式。在进行文档操作（如添加评论、下载文件等）时，必须先获取正确的 `file_token`。

### 文档 URL 格式与 Token 处理

| URL 格式 | 示例                                                      | Token 类型 | 处理方式 |
|----------|---------------------------------------------------------|-----------|----------|
| `/docx/` | `https://example.larksuite.com/docx/doxcnxxxxxxxxx`    | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/doc/` | `https://example.larksuite.com/doc/doccnxxxxxxxxx`     | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/wiki/` | `https://example.larksuite.com/wiki/wikcnxxxxxxxxx`    | `wiki_token` | ⚠️ **不能直接使用**，需要先查询获取真实的 `obj_token` |
| `/sheets/` | `https://example.larksuite.com/sheets/shtcnxxxxxxxxx`  | `file_token` | URL 路径中的 token 直接作为 `file_token` 使用 |
| `/drive/folder/` | `https://example.larksuite.com/drive/folder/fldcnxxxx` | `folder_token` | URL 路径中的 token 作为文件夹 token 使用 |

### Wiki 链接特殊处理（关键！）

知识库链接（`/wiki/TOKEN`）背后可能是云文档、电子表格、多维表格等不同类型的文档。**不能直接假设 URL 中的 token 就是 file_token**，必须先查询实际类型和真实 token。

#### 处理流程

1. **使用 `wiki.spaces.get_node` 查询节点信息**
   ```bash
   lark-cli wiki spaces get_node --params '{"token":"wiki_token"}'
   ```

2. **从返回结果中提取关键信息**
   - `node.obj_type`：文档类型（docx/doc/sheet/bitable/slides/file/mindnote）
   - `node.obj_token`：**真实的文档 token**（用于后续操作）
   - `node.title`：文档标题

3. **根据 `obj_type` 使用对应的 API**

   | obj_type | 说明 | 使用的 API |
   |----------|------|-----------|
   | `docx` | 新版云文档 | `drive file.comments.*`、`docx.*` |
   | `doc` | 旧版云文档 | `drive file.comments.*` |
   | `sheet` | 电子表格 | `sheets.*` |
   | `bitable` | 多维表格 | `bitable.*` |
   | `slides` | 幻灯片 | `drive.*` |
   | `file` | 文件 | `drive.*` |
   | `mindnote` | 思维导图 | `drive.*` |

#### 查询示例

```bash
# 查询 wiki 节点
lark-cli wiki spaces get_node --params '{"token":"wiki_token"}'
```

返回结果示例：
```json
{
   "node": {
      "obj_type": "docx",
      "obj_token": "xxxx",
      "title": "标题",
      "node_type": "origin",
      "space_id": "12345678910"
   }
}
```

### 资源关系

```
Wiki Space (知识空间)
└── Wiki Node (知识库节点)
    ├── obj_type: docx (新版文档)
    │   └── obj_token (真实文档 token)
    ├── obj_type: doc (旧版文档)
    │   └── obj_token (真实文档 token)
    ├── obj_type: sheet (电子表格)
    │   └── obj_token (真实文档 token)
    ├── obj_type: bitable (多维表格)
    │   └── obj_token (真实文档 token)
    └── obj_type: file/slides/mindnote
        └── obj_token (真实文档 token)

Drive Folder (云空间文件夹)
└── File (文件/文档)
    └── file_token (直接使用)
```

## 重要说明：画板编辑
> **⚠️ lark-doc skill 不能直接编辑已有画板内容，但 `docs +update` 可以新建空白画板**
### 场景 1：已通过 docs +fetch 获取到文档内容和画板 token
如果用户已经通过 `docs +fetch` 拉取了文档内容，并且文档中已有画板（返回的 markdown 中包含 `<whiteboard token="xxx"/>` 标签），请引导用户：
1. 记录画板的 token
2. 查看 [`../lark-whiteboard/SKILL.md`](../lark-whiteboard/SKILL.md) 了解如何编辑画板内容
### 场景 2：刚创建画板，需要编辑
如果用户刚通过 `docs +update` 创建了空白画板，需要编辑时：
**步骤 1：按空白画板语法创建**
- 在 `--markdown` 中直接传 `<whiteboard type="blank"></whiteboard>`
- 需要多个空白画板时，在同一个 `--markdown` 里重复多个 whiteboard 标签
  **步骤 2：从响应中记录 token**
- `docs +update` 成功后，读取响应字段 `data.board_tokens`
- `data.board_tokens` 是新建画板的 token 列表，后续编辑直接使用这里的 token
  **步骤 3：引导编辑**
- 记录需要编辑的画板 token
- 查看 [`../lark-whiteboard/SKILL.md`](../lark-whiteboard/SKILL.md) 了解如何编辑画板内容
### 注意事项
- 已有画板内容无法通过 lark-doc 的 `docs +update` 直接编辑
- 编辑画板需要使用专门的 [`../lark-whiteboard/SKILL.md`](../lark-whiteboard/SKILL.md)

## 快速决策
- 用户说“找一个表格”“按名称搜电子表格”“找报表”“最近打开的表格”，先用 `lark-cli docs +search` 做资源发现。
- `docs +search` 不是只搜文档 / Wiki；结果里会直接返回 `SHEET` 等云空间对象。
- 拿到 spreadsheet URL / token 后，再切到 `lark-sheets` 做对象内部读取、筛选、写入等操作。

## AI Usage Guidance：企业知识搜索方法论 ⭐

> **强制阅读**：搜索（`docs +search`）类任务，下面这套方法论是默认动作，不能跳过。详见 [`references/lark-doc-search-recipes.md`](references/lark-doc-search-recipes.md)。

### 1. 多轮关键词改写是默认动作

**单次搜索的召回率非常低**。开放问题或有明确目标的搜索任务，**至少跑 2-3 轮不同关键词**才算 baseline。每一轮换一个角度：

| 轮次 | 策略 | 例子（query: "飞书Office SaaS直销政策"） |
|---|---|---|
| 1 | 原始关键词 | `--query "飞书Office SaaS 直销 政策"` |
| 2 | 去掉修饰词，保留核心词 | `--query "SaaS 直销 售卖政策"` |
| 3 | 换同义词或具体术语 | `--query "飞书 售卖 折扣 政策"` |
| 4（如需） | 加业务术语限定 | `--query "Office 套件 价格 直销"` |

**反模式**：第一轮搜了一个看似贴近的候选就一头扎进去深挖。正确做法是先比较多轮的 top 结果，挑相关度最高的再深挖。

### 2. 广撒网 → 深挖，而不是一头扎进去

每一轮搜索看 top 5 候选（不是 top 1），按以下顺序判断哪个最相关：

1. **标题包含 query 核心词** > 标题不含
2. **标题用户场景对应** > 标题是评测集 / 周报 / 通用文档
3. **doc_types 匹配预期**（找权威文档优先 docx/wiki，找数据优先 sheet/bitable）
4. **owner / update_time 信号**（owner 是相关业务方、update_time 较近）

### 3. 空查询时不要轻易 abstain

如果搜了 2-3 轮都没明确命中，**不要直接说"找不到"**：

- **开放性问题**（用户问"为什么 X"、"怎么写 Y"）：可以基于通用知识 + 找到的弱相关材料 给出 best-effort 答案，但要明确标注"未找到权威文档，以下是基于通用知识 + 部分相关材料的推断"
- **事实性问题**（用户问具体数字、具体人）：才适合直接说"找不到"
- **聚合性问题**（用户问"列出所有 X"）：列出搜到的部分，并说明这是不完全列表

### 4. 大文档处理：先看摘要，必要时分段

`docs +fetch` 在体积特别大的文档上可能 504 timeout。处理策略：

1. 先看 search 结果里的 `summary_highlighted` 字段（已含关键句）
2. 若必须 fetch 全文，用 `--limit 50 --offset 0` 分段
3. 还失败时退到 raw API：`lark-cli api GET /open-apis/docx/v1/documents/<token>/blocks --params '{"page_size":20}'` 拉 block 列表，再针对相关 block 单独取内容
4. 详见 [`references/lark-doc-fetch.md`](references/lark-doc-fetch.md) 的"大文档处理"段

## 补充说明 
`docs +search` 除了搜索文档 / Wiki，也承担“先定位云空间对象，再切回对应业务 skill 操作”的资源发现入口角色；当用户口头说“表格 / 报表”时，也优先从这里开始。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli docs +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+search`](references/lark-doc-search.md) | Search Lark docs, Wiki, and spreadsheet files (Search v2: doc_wiki/search) |
| [`+create`](references/lark-doc-create.md) | Create a Lark document |
| [`+fetch`](references/lark-doc-fetch.md) | Fetch Lark document content |
| [`+update`](references/lark-doc-update.md) | Update a Lark document |
| [`+media-insert`](references/lark-doc-media-insert.md) | Insert a local image or file at the end of a Lark document (4-step orchestration + auto-rollback) |
| [`+media-download`](references/lark-doc-media-download.md) | Download document media or whiteboard thumbnail (auto-detects extension) |
| [`+whiteboard-update`](references/lark-doc-whiteboard-update.md) | Update an existing whiteboard in lark document with whiteboard dsl. Such DSL input from stdin. refer to lark-whiteboard skill for more details. |

