
> **导入分流规则：** 如果用户要把本地 Excel / CSV 导入成 Base / 多维表格 / bitable，必须优先使用 `lark-cli drive +import --type bitable`。不要先切到 `lark-base`；`lark-base` 只负责导入完成后的表内操作。

## 快速决策

- 用户要把本地 `.xlsx` / `.csv` 导入成 Base / 多维表格 / bitable，第一步必须使用 `lark-cli drive +import --type bitable`。
- 用户要把本地 `.md` / `.docx` / `.doc` / `.txt` / `.html` 导入成在线文档，使用 `lark-cli drive +import --type docx`。
- 用户要把本地 `.xlsx` / `.xls` / `.csv` 导入成电子表格，使用 `lark-cli drive +import --type sheet`。
- `lark-base` 只负责导入完成后的 Base 内部操作（表、字段、记录、视图），不要在“本地文件 -> Base”这一步提前切到 `lark-base`。

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

### 常见操作 Token 需求

| 操作 | 需要的 Token | 说明 |
|------|-------------|------|
| 读取文档内容 | `file_token` / 通过 `docs +fetch` 自动处理 | `docs +fetch` 支持直接传入 URL |
| 添加局部评论（划词评论） | `file_token` | 传 `--selection-with-ellipsis` 或 `--block-id` 时，`drive +add-comment` 会创建局部评论；仅支持 `docx`，以及最终解析为 `docx` 的 wiki URL |
| 添加全文评论 | `file_token` | 不传 `--selection-with-ellipsis` / `--block-id` 时，`drive +add-comment` 默认创建全文评论；支持 `docx`、旧版 `doc` URL，以及最终解析为 `doc`/`docx` 的 wiki URL |
| 下载文件 | `file_token` | 从文件 URL 中直接提取 |
| 上传文件 | `folder_token` / `wiki_node_token` | 目标位置的 token |
| 列出文档评论 | `file_token` | 同添加评论 |

### 评论能力边界（关键！）

- `drive +add-comment` 支持两种模式。
- 全文评论：未传 `--selection-with-ellipsis` / `--block-id` 时默认启用，也可显式传 `--full-comment`；支持 `docx`、旧版 `doc` URL，以及最终解析为 `doc`/`docx` 的 wiki URL。
- 局部评论：传 `--selection-with-ellipsis` 或 `--block-id` 时启用；仅支持 `docx`，以及最终解析为 `docx` 的 wiki URL。
- `drive +add-comment` 的 `--content` 需要传 `reply_elements` JSON 数组字符串，例如 `--content '[{"type":"text","text":"正文"}]'`。
- 如果 wiki 解析后不是 `doc`/`docx`，不要用 `+add-comment`。
- 如果需要更底层地直接调用评论 V2 协议，再走原生 API：先执行 `lark-cli schema drive.file.comments.create_v2`，再执行 `lark-cli drive file.comments create_v2 ...`。全文评论省略 `anchor`，局部评论传 `anchor.block_id`。

### 评论查询与统计口径（关键！）

- 查询文档评论时，使用 `drive file.comments list`。
- `drive file.comments list` 返回的 `items` 应理解为"评论卡片"列表，每个 `item` 对应用户界面里看到的一张评论卡片，而不是平铺的互动消息列表。
- 服务端语义上，创建第一条评论时会同时创建该卡片里的第一条 reply；因此真正承载正文的是每个 `item.reply_list.replies`，其中第一条 reply 在用户视角下就是这张卡片里的"评论本身"。
- 当用户要统计"评论数"或"评论卡片数"时，统计 `items` 的长度即可；如果是全量统计，则对所有评论分页返回的 `items` 长度累加。
- 当用户要统计"回复数"时，按用户视角应排除每张评论卡片里的首条评论，统计口径是所有 `item.reply_list.replies` 的长度之和减去 `items` 的长度。
- 当用户要统计"总互动数"时，统计所有 `item.reply_list.replies` 的长度之和即可；这个口径包含每张评论卡片里的首条评论。
- 如果某个 `item.has_more=true`，说明该评论卡片下还有更多回复未包含在当前返回中；此时需要继续调用 `drive file.comment.replys list` 拉全后，再做全量回复数 / 总互动数统计。

### 评论业务特性与引导（关键！）

#### 评论排序引导
- 一个文档通常有多个评论，评论按 `create_time`（创建时间）排序。
- **重要**：只有当用户明确提到"最新评论"、"最后评论"、"最早评论"时，才需要根据 `create_time` 进行排序：
  - **必须先获取所有评论（处理分页拉完所有数据）**，不能只获取一页就排序
  - "最新评论" / "最后评论"：按 `create_time` 降序排列，取第一条
  - "最早评论"：按 `create_time` 升序排列，取第一条
- 如果用户只说"第一条评论"，直接使用 `drive file.comments list` 返回的第一条即可，不需要额外排序。

#### 评论回复限制
- **添加评论回复前先检查是否存在以下限制**
- **全文评论不支持回复**：`is_whole=true` 的评论（全文评论）无法添加回复，遇到此类评论应提示用户"全文评论不支持回复"。
- **已解决评论不支持回复**：`is_solved=true` 的评论无法添加回复，遇到此类评论应提示用户"该评论已被解决，无法回复"。
- **注意**：当用户要回复某条评论但该评论因上述限制不能回复时，只提示不能回复即可，**不要自动帮用户找其他可以回复的评论**，避免不符合用户预期。

#### 批量查询与列表查询的选择
- 使用 `drive file.comments batch_query` 是**已知评论 ID 后**的批量查询，需要传入具体的评论 ID 列表。
- 使用 `drive file.comments list` 用于分页获取评论列表，适合统计评论总数、遍历所有评论，或获取"最新/最后 N 条评论"等场景。

#### Reaction / 表情场景
- 遇到评论 / 回复上的 reaction（表情、各表情数量、谁点了什么、添加/删除表情）相关问题时，**先阅读 [lark-drive-reactions.md](../../skills/lark-drive/references/lark-drive-reactions.md) 了解如何使用**。

### 典型错误与解决方案

| 错误信息 | 原因 | 解决方案 |
|----------|------|----------|
| `not exist` | 使用了错误的 token | 检查 token 类型，wiki 链接必须先查询获取 `obj_token` |
| `permission denied` | 没有相关操作权限 | 引导用户检查当前身份对文档/文件是否有相应操作权限；如果需要，可以授予相应权限 |
| `invalid file_type` | file_type 参数错误 | 根据 `obj_type` 传入正确的 file_type（docx/doc/sheet） |

### 授权当前应用访问文档

当需要将文档权限授予**当前应用（bot）自身**时，先通过 bot info 接口获取应用的 open_id，再调用权限接口授权：

```bash
# 1. 获取当前应用的 open_id
lark-cli api GET /open-apis/bot/v3/info --as bot
# 从返回值中取 bot.open_id

# 2. 授权当前应用访问文档
lark-cli drive permission.members create \
  --params '{"token":"<doc_token>","type":"<resource_type>"}' \
  --data '{"member_type":"openid","member_id":"<bot_open_id>","perm":"view","type":"user"}'
```

> **注意**：此方式仅适用于需要授权给**当前应用**的场景。授权给其他用户时，直接使用对方的 open_id 即可，无需调用 bot info 接口。

`<resource_type>` 可选值：`doc`、`docx`、`sheet`、`bitable`、`file`、`folder`、`wiki`。
