
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
- 用户说“看一下文档里的图片/附件/素材”“预览素材”，优先用 `lark-cli docs +media-preview`。
- 用户明确说“下载素材”，再用 `lark-cli docs +media-download`。
- 如果目标明确是画板 / whiteboard / 画板缩略图，只能用 `lark-cli docs +media-download --type whiteboard`，不要用 `+media-preview`。
- 用户说“找一个表格”“按名称搜电子表格”“找报表”“最近打开的表格”，先用 `lark-cli docs +search` 做资源发现。
- `docs +search` 不是只搜文档 / Wiki；结果里会直接返回 `SHEET` 等云空间对象。
- 拿到 spreadsheet URL / token 后，再切到 `lark-sheets` 做对象内部读取、筛选、写入等操作。
- 用户说“给文档加评论”“查看评论”“回复评论”“给评论加表情 / reaction”“删除评论表情 / reaction”，**不要留在 `lark-doc`**，直接切到 `lark-drive` 处理。

## 补充说明 
`docs +search` 除了搜索文档 / Wiki，也承担“先定位云空间对象，再切回对应业务 skill 操作”的资源发现入口角色；当用户口头说“表格 / 报表”时，也优先从这里开始。