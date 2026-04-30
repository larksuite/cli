> **命名约定：** Shortcut 命令组和原生 API / schema 都使用 `lark-cli base ...`。
> **分流规则：** 如果用户要把本地文件导入成 Base / 多维表格 / bitable，第一步不是 `base`，而是 `lark-cli drive +import --type bitable`。

## 核心规则

1. **原生 API 命令路径必须完整** — 资源名是多级点分路径，不能省略前缀
   - `lark-cli base table.records create` ✅
   - `lark-cli base records create` ❌
2. **优先使用 Shortcut** — 有 Shortcut 的操作不要手拼原生 API
3. **写记录前** — 先调用 `table.fields list` 获取字段 `type/ui_type`，再读 [lark-base-cell-value.md](../../skills/lark-base/references/lark-base-cell-value.md)；该文档是 CellValue 的 source of truth
4. **写字段前** — 先读 [lark-base-shortcut-field-properties.md](../../skills/lark-base/references/lark-base-shortcut-field-properties.md) 确认字段类型 JSON 结构
5. **筛选查询前** — 先读 [lark-base-view-set-filter.md](../../skills/lark-base/references/lark-base-view-set-filter.md)，当前 `base/v3` 通过 `view.filter update + table.records list` 组合完成筛选读取
6. **批量上限 200 条/次** — 同一表建议串行写入，并在批次间延迟 0.5–1 秒
7. **改名和删除按明确意图执行** — 视图重命名这类低风险改名操作，目标和新名称明确时可直接执行；删除记录 / 字段 / 表时，只要用户已经明确要求删除且目标明确，也可直接执行，不需要再补一次确认
8. **不要走旧 bitable 路径** — Base 场景不要调用 `lark-cli api GET /open-apis/bitable/v1/...`；即使 wiki 解析结果是 `obj_type=bitable`，后续也应继续使用 `lark-cli base ...`
9. **不要把本地文件导入误判成 Base 表内操作** — 如果目标是“把 Excel / CSV / `.base` 快照导入成 Base / 多维表格”，必须先走 `lark-cli drive +import --type bitable`；只有导入完成后，才回到 `lark-cli base ...`

## 意图 → 命令索引

| 意图 | 推荐命令 | 备注 |
|------|---------|------|
| 查表字段 | `table.fields list` | 写记录 / 更新前必调 |
| 查记录 | `table.records list` | GET，简单列表，可附带 `view_id` |
| 按视图筛选查询 | `view.filter update` + `table.records list` | 当前 `base/v3` 没有独立 `search` |
| 把本地 Excel / CSV / `.base` 导入为 Base / 多维表格 | `lark-cli drive +import --type bitable` | 导入阶段属于 `drive`，不是 `base` |
| 新增单条记录 | `table.records create` | 少量数据 |
| 更新记录 | `table.records patch` | 只传需要变更的字段 |
| 删除记录 | `table.records delete` | 单条删除 |
| 创建数据表 | `tables create` | 原生 API 直接在已有 Base 下建表 |
| 创建 / 更新字段 | `table.fields create/update` | 复杂字段建议先核对 schema |
| 创建 / 管理视图 | `table.views create/list/patch` | 视图筛选条件通过 `view.filter` 更新 |

## 操作注意事项

- **Base token 口径统一**：无论 Shortcut 还是原生 API，都统一使用 `base_token`
- **附件字段**：上传本地文件时只能走 `lark-cli base +record-upload-attachment`
- **能力边界**：当前 `base/v3` 原生 spec 以单表 / 单记录 / 视图筛选配置为主，批量写入和旧 `search` 场景优先走 unified Shortcut 组合能力
- **视图重命名确认规则**：用户已经明确“把哪个视图改成什么名字”时，执行 `table.views patch` / 对应 shortcut 直接改名即可，不需要再补一句确认
- **删除确认规则（记录 / 字段 / 表）**：执行 `table.records delete / table.fields delete / tables delete` 或对应 shortcut 时，如果用户已经明确要求删除且目标明确，可以直接执行；只有目标不明确时才先追问
- **创建 / 复制 Base 的友好性规则**：创建或复制 Base 时，`folder_token`、`time_zone`、复制时的新名称都属于可选项；用户没特别要求时不要为这些参数额外打断
- **创建 / 复制 Base 的结果返回规范**：成功后必须主动返回新 Base 的 token；如果返回结果里带可访问链接（如 `base.url`），也要一并返回

## Wiki 链接特殊处理（特别关键！）

知识库链接（`/wiki/TOKEN`）背后可能是云文档、电子表格、多维表格等不同类型的文档。**不能直接假设 URL 中的 token 就是 file_token**，必须先查询实际类型和真实 token。

### 处理流程

1. **使用 `wiki.spaces.get_node` 查询节点信息**
   ```bash
   lark-cli wiki spaces.get_node --params '{"token":"&lt;wiki_token&gt;"}'
   ```

2. **从返回结果中提取关键信息**
   - `node.obj_type`：文档类型（docx/doc/sheet/bitable/slides/file/mindnote）
   - `node.obj_token`：**真实的文档 token**（用于后续操作）
   - `node.title`：文档标题

3. **根据 `obj_type` 选择后续命令**

   | obj_type | 说明 | 后续命令 |
      |----------|------|-----------|
   | `docx` | 新版云文档 | `drive file.comments.*`、`docx.*` |
   | `doc` | 旧版云文档 | `drive file.comments.*` |
   | `sheet` | 电子表格 | `sheets.*` |
   | `bitable` | 多维表格 | `lark-cli base +...`（优先）；如果 shortcut 不覆盖，再用 `lark-cli base <resource> <method>`；不要改走 `lark-cli api /open-apis/bitable/v1/...` |
   | `slides` | 幻灯片 | `drive.*` |
   | `file` | 文件 | `drive.*` |
   | `mindnote` | 思维导图 | `drive.*` |

4. **把 wiki 解析出的 `obj_token` 当成 Base token 使用**
   - 当 `obj_type=bitable` 时，`node.obj_token` 就是后续 `base` 命令应使用的真实 token
   - 不要把 `wiki_token` 直接塞给 `--base-token`

5. **如果已经报了 token 错，再回退检查 wiki**
   - 如果命令返回 `param baseToken is invalid`、`base_token invalid`、`not found`，并且输入来自 `/wiki/...`，优先怀疑“把 wiki token 当成了 base token”
   - 重新执行 `wiki.spaces.get_node`
   - 确认 `obj_type=bitable` 后，用 `node.obj_token` 重试 `lark-cli base ...`

### 查询示例

```bash
# 查询 wiki 节点
lark-cli wiki spaces.get_node --params '{"token":"Pgrrwvr***********UnRb"}'
```

返回结果示例：
```json
{
  "node": {
    "obj_type": "docx",
    "obj_token": "UAJh***********ccaE9nic",
    "title": "ai friendly 测试 - 1 副本",
    "node_type": "origin",
    "space_id": "6946***********839"
  }
}
```

## 常见错误速查

| 错误码 | 含义 | 解决方案 |
|--------|------|---------|
| 1254064 | 日期格式错误 | 传 `YYYY-MM-DD HH:mm:ss` 字符串，不要写相对时间 |
| 1254068 | 超链接格式错误 | `"https://example.com"` 或 `"[文本](https://example.com)"` |
| 1254066 | 人员字段错误 | `[{ "id": "ou_xxx" }]` |
| 1254045 | 字段名不存在 | 检查字段名（含空格、大小写） |
| 1254015 | 字段值类型不匹配 | 先 list 字段，再按类型构造 |
| `param baseToken is invalid` / `base_token invalid` | 把 wiki token、workspace token 或其他 token 当成了 `base_token` | 如果输入来自 `/wiki/...`，先查 `wiki.spaces.get_node`；当 `obj_type=bitable` 时，用 `node.obj_token` 作为 `base_token` 重试，不要改走 `bitable/v1` |
| 1254104 | 批量超 200 条 | 分批调用 |
| 1254291 | 并发写冲突 | 串行写入 + 批次间延迟 |

## 参考文档

- [lark-base-shortcut-field-properties.md](../../skills/lark-base/references/lark-base-shortcut-field-properties.md) — 字段类型 JSON 配置
- [lark-base-cell-value.md](../../skills/lark-base/references/lark-base-cell-value.md) — CellValue source of truth
- [lark-base-view-set-filter.md](../../skills/lark-base/references/lark-base-view-set-filter.md) — 查询筛选指南（filter / operator / sort / 分页）
- [examples.md](../../skills/lark-base/references/examples.md) — 完整操作示例（建表、筛选、更新）
