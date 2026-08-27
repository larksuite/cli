# Docs Create 分批与限制测试报告

## 1. 报告信息

| 项目 | 内容 |
| --- | --- |
| 被测项目 | `lark-cli docs +create` 大文档分批写入 |
| 被测分支 | `sun/docs-create-batching` |
| PR | [larksuite/cli#2518](https://github.com/larksuite/cli/pull/2518) |
| 测试日期 | 2026-08-27 |
| 测试身份 | Bot（已验证可用） |
| 测试环境 | 远端开发机 + 真实飞书租户（生产基线；PPE 复测见 10.3） |
| 内容格式 | DocxXML、Markdown |
| 可靠目标批量 | 2,000 Block |
| 单次硬上限 | 5,000 Block |
| 文档总上限 | 40,000 Block |

## 2. 结论

### 2.1 总体结论

**分批创建主链路通过；若以最初完整限制计划作为发布验收，当前结论为不通过。**

通过项：

- XML、Markdown 均可按安全顶层边界执行 `create -> serial append`。
- 2,000 Block 可靠目标、5,000 Block 单次硬上限、40,000 Block 文档总量边界符合预期。
- XML、Markdown 的 5,001 Block 真实租户工作流均完成 create、2 次 append、fetch 首尾校验和清理。
- 82 个已注册内容标签全部完成 XML 与 Markdown 双格式目录测试，共 164 个子用例。
- Markdown parser、标题提升/去重、GFM Table、List、Task List、Definition List、公式、DocxXML 容器和资源标签均有专项覆盖。
- Append 失败采用 fail-fast，返回 partial result，保留已完成批次、失败批次和上游 typed error 信息。

阻断项：

1. 单 Block 100,001 字符未在 CLI 预检拦截，真实下游也接受创建。
2. Table 101 Column 未在 CLI 预检拦截，真实下游也接受创建。
3. Table 2,001 Cell 未在 CLI 预检拦截；下游在第二批 append 返回失败，但首批文档已创建，不符合“任何写入前拒绝”，且错误仅为 `api/unknown`。

### 2.2 需求验收表

| 需求 | 期望 | 实际 | 结论 |
| --- | --- | --- | --- |
| Create 总 Block | `<= 40,000` | 40,000 接受；40,001 在写入前拒绝 | PASS |
| Create 自动分批 | 大内容 create 后串行 append | XML/Markdown 真实 5,001 均成功 | PASS |
| 单次操作硬上限 | `<= 5,000` | 原子单元 5,000 接受；5,001 拒绝 | PASS |
| 可靠批量 | 避免真实服务 timeout | 5,000 与 XML 3,000 timeout；2,000 双格式成功 | PASS（目标设为 2,000） |
| 单 Block 字符数 | `<= 100,000` | 100,001 被真实创建 | **FAIL** |
| Table Cell | `<= 2,000` | 2,001 未预检；首批写入后 append failed | **FAIL** |
| Table Column | `<= 100` | 101 被真实创建 | **FAIL** |
| 校验失败时零写入 | 内容限制失败不得调用下游 | 2,001 Cell 已产生 partial 文档 | **FAIL** |
| 专用错误码 | `DOC_*_LIMIT` | 当前主要为 `validation/invalid_argument` 或 `api/unknown` | **FAIL** |
| Append/Replace 独立入口限制 | 按原计划覆盖 | 本 PR 只改造 create 编排 | NOT IN SCOPE |

## 3. 测试方法与证据等级

| 等级 | 含义 |
| --- | --- |
| L1 单元测试 | 直接验证 parser、Block 计数、分批计划和错误类型 |
| L2 Shortcut 测试 | 验证请求体、调用顺序、partial result 和资源聚合 |
| L3 Dry-run E2E | 使用编译后的 `lark-cli` 验证完整 CLI 请求计划，不写真实租户 |
| L4 Live E2E | 真实租户 create、append、fetch、delete，全流程自清理 |
| S 静态审阅 | 源码/目录覆盖，未声明为真实业务语义验证 |

报告中“PASS”至少有 L1 或更高证据；真实写入结论标注 L4。

## 4. 全 Block/标签目录覆盖

### 4.1 覆盖结果

新增 `TestCreateBatchPlannerCoversEveryRegisteredContentTag`，动态遍历统一 `blockCatalog`：

- 内容标签：82
- XML 子用例：82
- Markdown 子用例：82
- 合计：164
- 结果：全部通过

每个标签至少验证：

1. XML 严格 parser 可读取代表性结构。
2. XML 物化分类符合 block / inline / dual / structural 规则。
3. XML create planner 可规划且各批拼接后与原文逐字节一致。
4. Markdown SDK 对齐 parser 可读取该 DocxXML 标签壳。
5. Markdown create planner 可规划且各批拼接后与原文逐字节一致。

### 4.2 Block-level 标签（62）

| 分类 | 标签 |
| --- | --- |
| 标题与正文 | `title`, `h1`–`h9`, `p` |
| 列表与引用 | `ul`, `ol`, `li`, `checkbox`, `blockquote` |
| 布局与容器 | `div`, `grid`, `column`, `callout` |
| 表格壳与行 | `table`, `thead`, `tbody`, `tfoot`, `tr` |
| 代码与分隔 | `pre`, `hr` |
| 图片与文件 | `img`, `figure`, `source` |
| 文档资源 | `base_refer`, `synced_reference`, `synced-source`, `readonly-block`, `isv`, `view` |
| 嵌入资源 | `bitable`, `sheet`, `mindnote`, `whiteboard`, `html5-block` |
| OKR/任务 | `okr`, `okr-objective`, `okr-key-result`, `okr-progress`, `task` |
| 只读业务块 | `chat_card`, `poll`, `agenda`, `folder-manager`, `sub-page-list`, `wiki_catalog`, `wiki_recent_update`, `bookmark` |
| 图表块 | `chart-embedded`, `chart-refer-host-perm`, `chart_embedded`, `chart_refer_host_perm` |
| VC 块 | `vc-tabs`, `vc-summary-tab`, `vc-transcribe-tab` |
| 指令壳 | `append` |

### 4.3 Inline 标签（13）

`a`, `b`, `em`, `u`, `del`, `i`, `span`, `br`, `inline-file`, `mention-date`, `cite`, `button`, `time`

### 4.4 Dual 标签（2）

`code`, `latex`

验证根级使用时按 Block 物化，富文本容器内使用时保持 inline 语义。

### 4.5 Structural 标签（5）

`th`, `td`, `colgroup`, `col`, `sub-page`

其中 `th` / `td` 按物理 Cell Block 计数，其余仅参与结构。

### 4.6 不纳入内容 Block 目录的 Command 标签

以下标签属于指令协议，不是 create 正文 Block，因此不进入“82 个内容标签”分母：

`comment`, `block_delete`, `str_delete`, `str_replace`, `block_replace`, `block_insert`, `block_move`, `block_copy_insert_after`, `src_block_ids`, `create`, `answer`, `response`, `identifier`, `genre`, `anchor`, `type`, `revision`, `pattern`, `replacement`, `replace_content`, `action`, `content`, `parameter`, `generation`, `block_id`

### 4.7 全目录测试的边界

目录测试证明 parser/planner 对所有注册标签无遗漏，但不等价于每个业务 Block 的所有属性组合都通过真实引擎验证。必须结合下列专项用例理解：

- required attributes、token 有效性和权限由下游业务 visitor 验证。
- `rowspan` / `colspan`、资源绑定、标题分析、Table 内容分发等由专项测试覆盖。
- 只读业务块的真实 token 数据没有逐类型创建，覆盖等级为 L1/S。

## 5. 分批边界矩阵

### 5.1 可靠目标边界

| Case | 期望 | 结果 | 证据 |
| --- | --- | --- | --- |
| 2,000 Block | 单 create | 单批 2,000 | L1 |
| 2,001 Block | create + append | `[2000, 1]` | L1 |
| 普通内容 5,000 | 3 批 | 3 次 API 计划 | L3 |
| 普通内容 5,001 | 3 批 | 3 次 API 计划 | L3 |
| 普通内容 40,000 | 20 批 | 20 次 API 计划 | L3 |
| 普通内容 40,001 | 写入前拒绝 | exit 2 / `invalid_argument` | L3 |

XML 与 Markdown 均执行上述矩阵，结果一致。

### 5.2 单次硬上限

| Case | 期望 | 结果 |
| --- | --- | --- |
| 不可拆顶层单元 5,000 | 允许独占一批 | PASS |
| 不可拆顶层单元 5,001 | `subtree_limit` | PASS |
| Markdown List 5,000 item | 允许独占一批 | PASS |
| Markdown List 5,001 item | 拒绝 | PASS |
| 首批隐式 title + 5,000 Block 子树 | 容量 5,001，拒绝 | PASS |

### 5.3 文档总量

| Case | XML | Markdown |
| --- | --- | --- |
| 40,000 | 接受，20 批 | 接受，20 批 |
| 40,001 | 写入前拒绝 | 写入前拒绝 |

### 5.4 原文保真

对 XML 与 Markdown 均断言：

```text
strings.Join(plan.Batches, "") == originalSource
```

覆盖普通段落、title、List、fenced code、同一行容器、DocxXML 容器和 SDK 预处理场景。

## 6. XML 专项覆盖

| 类别 | Case | 结果 |
| --- | --- | --- |
| 顶层切分 | 仅在顶层 element 之间切分 | PASS |
| Title | leading title 保留在 create | PASS |
| Late title | title 将落入 append 时拒绝 | PASS |
| 隐式 title | 无 title 时预留 1 Block | PASS |
| List | `ul`/`ol` 壳不计，`li` 计数 | PASS |
| Table | table、物理 Cell、占位 Cell、Cell 内容计数 | PASS |
| Table inline run | Cell 中块前后文本分成独立段落 | PASS |
| Row/col span | 声明尺寸进入物理 Cell 估算；occupancy 遍历有安全上限 | PASS（计数逻辑） |
| Source | 独立 `source` 物化为 view + file，计 2 | PASS |
| Inline source | 富文本/Table Cell 内保持 inline file 语义 | PASS |
| Code | `<pre><code>` 中 code 不重复计 Block | PASS |
| Latex/code dual | 根级计 Block，富文本内不独立计 | PASS |
| OKR rich-text shell | 虚拟 `p` 壳不重复计数 | PASS |
| Comment/CDATA/PI | 不作为顶层可切分 Block | PASS（parser 既有覆盖） |
| Strict parse failure | 兼容 XML 继续交给服务处理，不做本地错误拆分 | 兼容策略保留 |

## 7. Markdown 专项覆盖

### 7.1 Parser 对齐

使用与 SDK create 路径一致的：

- Goldmark + GFM
- Definition List
- inline / display math
- underscore HTML tags
- DocxXML container block parser
- DocxXML XML block / inline parser
- 相同 parser priority
- 与 Block AST 有关的 SDK 预处理

### 7.2 语义矩阵

| 类别 | Case | 结果 |
| --- | --- | --- |
| Paragraph/Heading | 普通段落、ATX H1、H2–H6 | PASS |
| Title promotion | 唯一可靠顶层 H1 提升并删除正文 H1 | PASS |
| Ambiguous H1 | 多个可靠 H1 保留正文并阻止错误切分 | PASS |
| Explicit title duplicate | 匹配且可删除的 H1 不重复计数 | PASS |
| Late promoted H1 | 无法留在 create 时拒绝 | PASS |
| Empty ATX H1 | SDK 会删除的空 H1 不计 Block | PASS |
| H1 in fence | 不参与标题分析 | PASS |
| `--title` 单换行 | `<title>\n首段` 不再被 Goldmark 合并漏计 | PASS（真实边界测试发现并修复） |
| Ordered/unordered List | 整个 List 原子化，item 精确计数 | PASS |
| Nested List | SDK create AST 计数 | PASS |
| Task List | checkbox Block 计数 | PASS |
| Definition List | term paragraph + description blockquote/paragraph | PASS |
| Blockquote | 容器 + children；空容器补 fallback paragraph | PASS |
| GFM Table | table + Cell + Cell 内容 | PASS |
| Table image | image 与两侧 inline runs 分别计数 | PASS |
| Fenced code | 整体原子，不从 fence 内切分 | PASS |
| Empty fence | opening/closing 边界定位 | PASS |
| Mermaid/PlantUML/SVG fence | 按单个 whiteboard 类 Block 计数 | parser 路径覆盖，未逐语言 live |
| Math | inline/block math 留在段落语义 | PASS |
| `<pre><code>` | 多行内容保护，内部标签按字面量 | PASS |
| Same-line grid/column | SDK 相邻容器预处理，原文不改变 | PASS |
| Empty callout | fallback paragraph 计数 | PASS |
| Underscore resource tag | block 与 inline 两种位置 | PASS |
| Markdown image | paragraph/list/table Cell 中物化计数 | PASS |
| Whiteboard raw body | 内部 Markdown 不作为 children 重复计数 | PASS |

### 7.3 Markdown fail-closed

当 Markdown planner 无法证明安全边界时，CLI 返回 typed validation error，不回退为未统计的单请求。这样避免绕过 5,000/40,000 限制。

## 8. 资源 Block 与结果聚合

| Case | 结果 |
| --- | --- |
| `reference_map` 复制到所有 append body | PASS |
| `scene` 复制到所有 append body | PASS |
| create + append 的 `document.new_blocks` 聚合 | PASS |
| 最新 `revision_id` 回填 | PASS |
| warnings 聚合 | PASS |
| 本地图片/文件在所有成功批次后统一绑定 | PASS |
| failed batch 的 `new_blocks` 不并入成功结果 | PASS |
| 远程图片预检发生在第一次写入前 | PASS |

资源型标签目录覆盖：

`img`, `figure`, `source`, `inline-file`, `whiteboard`, `sheet`, `bitable`, `base_refer`, `synced_reference`, `synced-source`, `mindnote`, `html5-block`, `isv`, `readonly-block`, `bookmark`, `chat_card`, `task`

说明：目录和单元测试验证规划/计数；本地 image/file 的上传、绑定与回查由既有 local-resources E2E 覆盖。并非每个只读资源类型都使用真实 token 创建。

## 9. 失败与恢复

| Case | 期望 | 结果 |
| --- | --- | --- |
| Create API 失败 | 不执行 append | PASS |
| Create 响应缺 document ID | append 不执行 | PASS |
| Append API error | 停止后续批次 | PASS |
| Append 返回 `result=failed` | 停止后续批次 | PASS |
| Partial metadata | total/completed/failed batch 完整 | PASS |
| Upstream typed error | 写入 `data.create_batches.error` | PASS |
| 已成功批次 | 不回滚 | 按设计 |
| 失败批次之后 | 不继续写入 | PASS |
| Partial 文档资源 | 可按 document token 清理 | Live 已验证 |

## 10. 内容限制边界实测

### 10.1 CLI dry-run 诊断

| Case | Planner 结果 | API 计划 |
| --- | --- | --- |
| Block 100,000 字符 | 接受 | 1 |
| Block 100,001 字符 | 接受 | 1 |
| Table 100 Column | 接受 | 1 |
| Table 101 Column | 接受 | 1 |
| Table 2,000 Cell | 接受 | 2，规划总 Block 4,002 |
| Table 2,001 Cell | 接受 | 2，规划总 Block 4,004 |

结论：当前 create planner 只统计物化 Block，没有实现字符/Cell/Column 专用预检。

### 10.2 真实下游结果

| Case | 真实结果 | 是否符合需求 |
| --- | --- | --- |
| Block 100,000 字符 | 创建成功；已执行串行删除 | PASS |
| Block 100,001 字符 | **创建成功；已执行串行删除** | **FAIL** |
| Table 100 Column | 创建成功；已执行串行删除 | PASS |
| Table 101 Column | **创建成功；已执行串行删除** | **FAIL** |
| Table 2,000 Cell（20 × 100） | 创建成功；已执行串行删除 | PASS |
| Table 2,001 Cell（20 × 100 + 1） | 首批创建后，append `result=failed`；已对 partial 文档执行删除 | **FAIL：非预检，且产生写入** |

2,001 Cell 的 CLI 错误为：

```text
type=api
subtype=unknown
message="append batch 2 returned result=failed"
```

没有返回预期的 `DOC_TABLE_CELL_LIMIT`、actual 或 limit。

内容限制探针使用精确 document token 串行调用 `drive +delete --yes`；本轮没有再做搜索索引级残留审计。5,001 Block Live E2E 使用 `DeleteDriveResourceAndVerify` 轮询验证不可见，清理证据更强。

### 10.3 `ppe_sun_ai_test` 复测

2026-08-27 使用本机 `lark-cli` Control Room 的 PPE 切流规则，将验证机上的当前分支二进制经同一 Whistle 代理路由到 `ppe_sun_ai_test`。每次 Docs AI 响应均显示 `env_psm=lark.apigw.apigw_pre_release`；Cell 2,000 的下游 warning 进一步明确调用链为 `creation.platform.ai_edit_pre_release -> creation.docx.engine_pre_release`。

| Case | PPE 结果 | LogID | 结论 |
| --- | --- | --- | --- |
| Block 100,000 字符 | 创建成功，约 6 秒；删除成功 | `20260827155849F8FB745AC839837E5DCB` | PASS |
| Block 100,001 字符 | 首次 create 拒绝，无 document token；`12320003` / `DOC_BLOCK_CHAR_LIMIT`，`operation=create, actual=100001, limit=100000` | `2026082715585943C0342B13454E24422C` | PASS |
| Table 100 Column | 创建成功，约 6 秒；删除成功 | `20260827155906EA127E246F3FDE1C43FC` | PASS |
| Table 101 Column | 首次 create 拒绝，无 document token；`12320005` / `DOC_TABLE_COLUMN_LIMIT`，`operation=create, actual=101, limit=100` | `202608271559172FDF13C5CAF814764A70` | PASS |
| Table 2,000 Cell（20 × 100） | 未触发 Cell 限制；首批创建后第二批 append 调用 engine 超时，返回 partial document；删除成功 | create `202608271559234B446F8C0F6FC495457A`；append `202608271559274B446F8C0F6FC49547F2` | **ENV BLOCKED**：`engine_pre_release` 10 秒超时，不能据此验收成功边界 |
| Table 2,001 Cell（2,001 × 1） | 首批创建后第二批 append 拒绝；`12320004` / `DOC_TABLE_CELL_LIMIT`，`operation=append, actual=2001, limit=2000`；partial document 删除成功 | create `202608271600443AD6F71862C9AE0A111B`；append `2026082716004869129D3420B2CC3DAA71` | **FAIL**：限制已生效，但 create 未在任何写入前完成整份内容预检 |

补充探针 `20 × 100 + 1` 在 SDK 中按 21 行、100 列的有效矩形统计为 2,100 Cell，因此不能作为精确 2,001 边界；该请求同样在第二批 append 返回 `DOC_TABLE_CELL_LIMIT(actual=2100)`，partial document 已删除。精确边界改用 `2,001 × 1` 后，actual 为 2,001。

PPE 复测结论：

- 字符数与 Column 两项下游限制已部署，未复现生产基线中的“超限仍创建成功”。
- Cell 专用错误码已部署，不再退化为单纯的 `api/unknown`。
- CLI 的 create 自动分批会先写入 title，再把原子 Table 放入 append；因此 Cell 超限仍会产生 partial document，且错误 operation 变为 `append`。这仍违反“创建请求在任何下游写入前拒绝”的原始要求。
- Cell 2,000 成功边界受 PPE `creation.docx.engine_pre_release` 超时阻断；测试时该 PPE 资源无可用 Pod，需要环境恢复后补测。
- 本轮所有返回 document token 的探针均已通过 `drive +delete --yes` 删除。

## 11. 真实租户大文档测试

### 11.1 可靠批量探测

| 格式/规模 | 结果 | 耗时 |
| --- | --- | --- |
| Markdown 首批 5,000 | timeout（连续 2 次） | 约 35–37 秒/次 |
| XML 首批 3,000 | timeout | 约 37 秒 |
| Markdown 约 2,000 | 成功 | 25.05 秒 |
| XML 约 2,000 | 成功 | 25.94 秒 |

因此最终选择两种格式共同稳定的 2,000 Block 作为可靠目标，而保留 5,000 作为硬上限。

### 11.2 最终 5,001 Block 工作流

| 格式 | 请求形态 | 验证 | 耗时 | 结果 |
| --- | --- | --- | --- | --- |
| XML | create 2,000 + append 2,000 + append 1,001 | fetch 包含首尾 marker；删除文档/文件夹 | 36.03 秒 | PASS |
| Markdown | create 2,000 + append 2,000 + append 1,001 | fetch 包含首尾 marker；删除文档/文件夹 | 33.87 秒 | PASS |

耗时包含测试文件夹创建、文档写入、fetch 和 cleanup，不包含编译时间；不作为 SLA。

## 12. 构建与质量门禁

| 检查 | 结果 |
| --- | --- |
| `make fmt-check` | PASS |
| `make vet` | PASS |
| `make unit-test`（race） | PASS |
| `make build` | PASS |
| `make quality-gate` | PASS；仅既有仓库提示 |
| diff-scoped golangci-lint | 0 issue |
| source guards / lint tests | PASS |
| go-licenses | PASS；仅 x/sys assembly 提示 |
| Skill quick validation | PASS |
| Skill format check | PASS |

## 13. 已知缺口与建议

### P0：补统一新增内容限制器

在任何资源处理、转换和下游写入前，对 XML/Markdown 的解析结果统一校验：

- 单 Block 字符数 `<= 100,000`
- 单 Table Cell 数 `<= 2,000`
- 单 Table Column 数 `<= 100`

该限制器应复用 SDK 语义或由 SDK 提供统计结果，避免 CLI 与 SDK 再次漂移。

### P0：错误码与阶段

内容限制必须在首个 create 前失败，并返回稳定的领域错误：

- `DOC_BLOCK_CHAR_LIMIT`
- `DOC_TABLE_CELL_LIMIT`
- `DOC_TABLE_COLUMN_LIMIT`

错误至少携带 operation、actual、limit。2,001 Cell 当前的 partial write + `api/unknown` 不可作为最终验收结果。

### P1：补真实资源 Block fixture

为 whiteboard、sheet、bitable、mindnote、synced reference、chat card、task 等建立稳定 token fixture，逐类型验证：

- create/append 后真实 Block 类型
- `new_blocks` 顺序和 token
- fetch XML/Markdown 往返
- delete 后无残留引用

### P1：Append/Block Replace 独立入口

本报告只覆盖 create 自动分批。原计划中的已有文档总量、单次 Append 5,000、Block Replace 增减公式和旧子树计数，需要在对应 SDK/服务 PR 中单独形成同等级报告。

## 14. 发布判定

如果本次发布范围仅为“lark-cli create 自动分批”，结论为：**可进入评审，主链路通过。**

如果本次发布范围包含最初计划中的全部内容限制，结论为：**不可发布，需先解决字符、Table Cell、Table Column 三项阻断问题及错误码。**

PPE 复测后可进一步收敛为：字符与 Column 下游拦截已就绪；当前 create 分批发布阻断点主要是 **整份内容预检缺失导致的 Table Cell partial write**，以及 PPE engine 不可用导致的 2,000 Cell 成功边界待补测。
