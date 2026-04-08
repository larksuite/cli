
# docs +fetch（获取飞书云文档）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

## 命令

```bash
# 获取文档内容（默认输出 Markdown 文本）
lark-cli docs +fetch --doc "https://xxx.feishu.cn/docx/Z1FjxxxxxxxxxxxxxxxxxxxtnAc"

# 直接传 token
lark-cli docs +fetch --doc Z1FjxxxxxxxxxxxxxxxxxxxtnAc

# 知识库 URL 也支持
lark-cli docs +fetch --doc "https://xxx.feishu.cn/wiki/Z1FjxxxxxxxxxxxxxxxxxxxtnAc"

# 分页获取（大文档）
lark-cli docs +fetch --doc Z1FjxxxxxxxxxxxxxxxxxxxtnAc --offset 0 --limit 50

# 人类可读格式输出
lark-cli docs +fetch --doc Z1FjxxxxxxxxxxxxxxxxxxxtnAc --format pretty
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--doc` | 是 | 文档 URL 或 token（支持 `/docx/` 和 `/wiki/` 链接，系统自动提取 token） |
| `--offset` | 否 | 分页偏移 |
| `--limit` | 否 | 分页大小 |
| `--format` | 否 | 输出格式：json（默认，含 title、markdown、has_more 等字段） \| pretty |

## 大文档处理 ⚠️

**已知问题**：对体积特别大的文档（万行级别、内嵌大量表格 / 媒体），`docs +fetch` 可能直接返回 `MCP HTTP 504 Gateway Timeout`。日志类文档（每日更新型）尤其常见。

### 处理策略（按优先级）

1. **先看 search 结果的 summary**：`docs +search --query "..."` 返回结果的 `summary_highlighted` 字段已经包含 query 命中的关键句，很多时候这就够答题了，根本不需要 fetch 全文
2. **分段 fetch**：`lark-cli docs +fetch --doc <token> --offset 0 --limit 50`，先拿前 50 个 block 看看结构和命中位置
3. **退到 raw blocks API**：
   ```bash
   # 列 block_id（很快，不会 timeout）
   lark-cli api GET /open-apis/docx/v1/documents/<token>/blocks --params '{"page_size":50}'

   # 拿到 block_id 列表后，针对相关 block 单独取内容
   lark-cli api GET /open-apis/docx/v1/documents/<token>/blocks/<block_id>/children --params '{"page_size":50}'
   ```
4. **wiki 包装的大文档**：先用 `lark-cli wiki +resolve-node --token <wiki_token>` 拿到真正的 `obj_token`，再用上面的策略

### 反模式

- **不要在 fetch 失败后直接说"找不到"** —— 大文档 fetch 失败 ≠ 内容找不到，源文档已经定位了，只是工具能力暂时拿不到全文
- **不要在 fetch 失败后无限重试** —— 504 通常是稳定失败，重试 1 次仍然失败就该切策略

## 重要：图片、文件、画板的处理

**文档中的图片、文件、画板需要通过 `lark-doc-media-download`（docs +media-download）单独获取！**

### 识别格式

返回的 Markdown 中，媒体文件以 HTML 标签形式出现：

- **图片**：
  ```html
  <image token="Z1FjxxxxxxxxxxxxxxxxxxxtnAc" width="1833" height="2491" align="center"/>
  ```

- **文件**：
  ```html
  <view type="1">
    <file token="Z1FjxxxxxxxxxxxxxxxxxxxtnAc" name="skills.zip"/>
  </view>
  ```

- **画板**：
  ```html
  <whiteboard token="Z1FjxxxxxxxxxxxxxxxxxxxtnAc"/>
  ```
- 画板编辑：详见 [SKILL.md](../SKILL.md#重要说明画板编辑)

### 获取步骤

1. 从 HTML 标签中提取 `token` 属性值
2. 调用 lark-doc-media-download（docs +media-download）：
   ```bash
   lark-cli docs +media-download --token "提取的token" --output ./downloaded_media
   ```

## Wiki URL 处理策略

知识库链接（`/wiki/TOKEN`）背后可能是云文档、电子表格、多维表格等不同类型的文档。当不确定类型时，**不能直接假设是云文档**，必须先查询实际类型。

### 处理流程

1. **先调用 lark-wiki 解析 wiki token**
2. **从返回的 `node` 中获取 `obj_type`（实际文档类型）和 `obj_token`（实际文档 token）**
3. **根据 `obj_type` 调用对应工具**：

| obj_type | 工具 | 说明 |
|----------|------|------|
| `docx` | `lark-doc-fetch` | 云文档 |
| `sheet` | `lark-sheet` | 电子表格 |
| `bitable` | `lark-base` | 多维表格 |
| 其他 | 告知用户暂不支持 | — |

## 工具组合

| 需求 | 工具 |
|------|------|
| 获取文档文本 | `docs +fetch` |
| 下载图片/文件/画板 | `docs +media-download` |
| 创建新文档 | `docs +create` |
| 更新文档内容 | `docs +update` |

## 参考

- [lark-doc-create](lark-doc-create.md) — 创建文档
- [lark-doc-update](lark-doc-update.md) — 更新文档
- [lark-doc-media-download](lark-doc-media-download.md) — 下载素材/画板缩略图
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
