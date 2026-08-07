# Agent 可消费的文档上下文读取

当用户要求“完整理解 / 深度阅读 / 审阅”一篇文档，且正文之外的评论、图片、画板、同步引用或嵌入数据可能影响结论时，使用本工作流。

目标是让用户只表达一次读取意图，由 Agent 编排已有只读命令。“完整”是覆盖与任务相关的信息类型，不是把所有能力塞进一次 API 请求，也不是无界抓取。

## 工作流

### 1. 先选择最小读取模式

| 用户意图 | 首选路径 |
|-|-|
| 只要标题、类型、canonical token / URL | `lark-cli drive +inspect --url '<DOC_URL>'`，不读取正文 |
| 阅读或总结正文 | `docs +fetch --detail simple` |
| 连同评审意见理解文档 | 正文 + `drive +list-comments` |
| 结论依赖图片、画板或嵌入数据 | 正文 + 第 3 节的按需补全 |

读取正文时，按 [`lark-doc-fetch.md`](lark-doc-fetch.md) 选择最小充分范围：

- 用户给出具体术语、错误码或标识：先用 `keyword` 定位，需要更多上下文时再按返回的 `top-block-id` 用 `section` / `range` 精读
- 文档较短且任务涉及整体：`docs +fetch --detail simple`
- 长文档或只涉及部分主题，且没有具体关键词：先读 `outline`，再按 `section` / `range` 精读
- 需要把评论定位回正文 block：改用 `--detail with-ids`

不要因为用户说“完整”就默认使用 `--detail full`；`full` 用于保真编辑元数据，不等于更完整的业务语义。

### 2. 按任务决定是否读取评论

评论属于 `lark-drive`，不是 `lark-doc`。以下场景才补充评论：

- 用户明确要求查看评论、评审意见或讨论结论
- 用户要求审阅文档，且未解决评论可能改变对当前方案状态的判断

```bash
# 默认只读取未解决评论
lark-cli drive +list-comments --url '<DOC_URL>'

# 需要把评论定位到 Docx 正文时
lark-cli drive +list-comments --url '<DOC_URL>' --need-relation
```

只有用户明确要求包含已解决评论时才添加 `--solved-status all`。评论细节与回复限制以 [`lark-drive`](../../lark-drive/SKILL.md) 为准。

### 3. 识别需要补全的结构化上下文

检查 fetch 内容中的结构化标签。Sheet/Base 标签按主 Skill 的路由规则必须下钻；其他资源只在会影响当前任务结论时处理：

| 信号 | 处理方式 |
|-|-|
| `<img>` / `<source>` | 图片、截图或附件承载关键信息时，用 `docs +media-preview` 预览；不要默认下载全部素材 |
| `<whiteboard>` | 架构、流程或决策依赖画板时，用 `docs +media-download --type whiteboard` 获取缩略图并查看 |
| `<sheet>` / `<cite file-type="sheets">` | 必须提取 `token` 与 `sheet-id`，切到 `lark-sheets` 读取相关范围 |
| `<bitable>` / `<cite file-type="bitable">` | 必须提取 `token` 与 `table-id`，切到 `lark-base` 查询相关字段和记录 |
| `<synced_reference>` | 源内容可能影响结论时，提取 `src-token` 与 `src-block-id`，只读取对应源 block |

素材预览会写入本地文件；遵守 [`lark-shared`](../../lark-shared/SKILL.md) 的相对路径和安全规则。

### 4. 处理长文档与降级

- 优先用 `outline` + `section` 分段读取，避免一份超长响应挤占 Agent 上下文
- 只去重等价请求；revision、scope、range、detail 或 fields 不同时视为不同请求
- 某个补充能力失败时保留已获得的正文，并明确标记评论、素材或嵌入数据未覆盖；不要把局部失败伪装成整篇读取失败
- 用户需要稳定的本地导出文件时，切到 `lark-drive` 的 `drive +export`；不要给 `docs +fetch` 虚构本地输出参数

## 输出要求

最终回答应区分：

1. 正文直接陈述的事实
2. 评论、图片、画板或嵌入数据补充的事实
3. 基于上述证据的推断
4. 未读取或无法访问、且可能影响结论的上下文

不要声称“已完整读取”而不说明实际覆盖了哪些上下文类型。
