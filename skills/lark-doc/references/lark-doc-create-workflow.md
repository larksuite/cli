# Lark Doc Authoring

## Philosophy

以下原则是每个内容、结构和视觉决策的判定依据；写作和复查时逐条套用，冲突时按「约束栈」排序。

- **读者本位**：落地前先回答：读者是谁、为什么要读、带着什么任务来。按读者的任务组织内容，不按功能或作者视角罗列。
- **结构先行**：结论先行，先整体后局部；按逻辑分组与递进，依据关系选择列表、步骤或表格，使内容便于扫读。（特殊体裁除外）
- **视觉服从语义**：先确定全篇主线和每节的中心任务或命题，再让视觉层级复现内容优先级。文档脱离讲解仍须完整、连续、可独立阅读。
- **最低理解成本**：选择最能降低读者理解、执行和出错成本的表达形式，而不是机械选择字符最少或制作成本最低的形式；删冗余，用短句、动词和数据，并按真实信息关系使用图、表格或交互组件。
- **克制且连贯**：每个视觉元素必须承担导航、比较、解释、证据、行动，或体裁所需的氛围与品牌功能；相关文字与视觉相邻，同类关系复用同类组件和样式。去掉后不影响读者任务或预期语气的装饰应删除。
- **约束栈**：事实 > 用户硬约束 > 读者任务 > 内容 > 组件样式；后项不得牺牲或放宽前项，格式与组件不得反向改变内容判断。
- **表达一致**：同一对象、动作和状态全文同名；标题层级与编号采用统一体系，如下；用户提供样例时，在不违反更高优先级规则的前提下延续其有效结构、语气、术语和编号。
  - **自动编号模式**：每一个正文标题都写 `seq="auto"`，标题文本不手写任何前置序号。
  - **中文手写模式**：适用于公文或正式场景，在标题文本中手写 `一、→（一）→ 1.→（1）`；最忌中文层级配阿拉伯小数，绝不出现 `一、` 下接 `1.1`。

## Step Plan

**CRITICAL：从零创作文档时按下述步骤依次执行，不可跳步。**

### Step 1：理解读者任务、文档格式要求、硬约束和禁区。

### Step 2：选择 genre content contract。

下表文件均位于当前 Skill 的 `references/genres/` 目录。

- 路由表仅用于选择候选，不代替 contract。高置信命中后必须读取对应 Profile / Adapter，并按其中的路由与消歧规则复核；未读取不得确定该值或进入 Step 3。确认后记录固定短名，最多各读取一个；未命中时可省略 `genre_contract` 和 `adapter`。
- contract 决定内容任务、证据和体裁边界；adapter 只调整与所选 contract 兼容的平台结构、写作风格和组件约束。

   | Content Profile | 独特专业任务 |
   |-|-|
   | [`route-workplace.md`](genres/route-workplace.md) | 组织决策、执行、留档 |
   | [`route-report.md`](genres/route-report.md) | 数据、研究和证据形成洞察 |
   | [`route-knowledge.md`](genres/route-knowledge.md) | 理解、自学、一次已知操作或检索 |
   | [`route-media.md`](genres/route-media.md) | 独立采集、核实和公共理解 |
   | [`route-opinion.md`](genres/route-opinion.md) | 形成并论证判断 |
   | [`route-consumer.md`](genres/route-consumer.md) | 以真实体验或测试辅助消费选择 |
   | [`route-marketing.md`](genres/route-marketing.md) | 组织授权的认知、转化或公关内容 |
   | [`route-personal-brand.md`](genres/route-personal-brand.md) | 本人经历、能力和作品的可信呈现 |
   | [`route-creative.md`](genres/route-creative.md) | 角色、冲突、情节与分支叙事 |

   | Adapter | 渠道 |
   |-|-|
   | [`route-platform.md`](genres/route-platform.md) | Email、微信公众号、小红书 |

### Step 3：收集资料并扫描表达机会。

1. 强制扫描事实、数据、案例、引用和图片等资源缺口；内容需要而现有材料不足时必须检索或生成，判断需要图片且用户未提供素材时必须搜索图片。
2. 根据用户要求、contract / adapter 限制和内容需要选择表达方式；可用 `presentation_mode` 记录视觉策略，不因字段存在就机械使用组件。

   | 信息关系 | 候选表达 |
   |-|-|
   | 同组字段的精确比较或映射 | `table` |
   | 流程、依赖、分支、时序、层级、因果、空间或拓扑关系 | `whiteboard` |
   | 对象、场景、界面、外观、氛围、示例或视觉证据 | `img` |
   | 复杂交互、动态状态、可探索数据或应用式布局 | `html5-block` |
   | 两组简短、等权且适合横向阅读的信息 | `grid` |
   | 单个关键提醒或限制 | `callout` |
   | 简单并列、步骤或连续论述 | 列表或段落 |

3. 按全篇、章节、block 三个尺度构图：相关内容相邻，同类关系保持相同顺序与对齐；正文可以是主表达，不要求每节都有 presentation block。
4. 只有需要校验最低数量时，才在 `visual_plan.blocks` 中声明相应 block 的 `type` 和 `min_count`；`purpose` 可按需说明用途。普通的图表使用意图无需转换成数量配额。

`presentation_mode` 只表示模型采用的视觉策略；只有用户要求、contract / adapter 限制互相冲突时才询问用户：

- `formal`：视觉正式、克制；不使用高亮块、emoji 或装饰性组件，只保留正式体裁确有必要的结构。
- `normal`：按内容需要使用组件；只有能降低理解、执行或出错成本时才扩展视觉表达。
- `rich`：主动利用图片、画板、HTML 和其他飞书组件；每个组件须有明确目的，不设全局数量配额。

### Step 4：声明需校验的约束，并初始化草稿。

按实际要求声明需要机器校验的约束；没有此类约束时使用 `{}`。格式和错误处理详见 [`docs +script`](lark-doc-script.md)。

| 本次需要校验什么 | 决策示例 |
|-|-|
| 无字数或块数量要求 | `{}` |
| 仅要求 800–1200 字 | `{"word_count":{"min":800,"max":1200}}` |
| 至少一张画板 | `{"visual_plan":{"blocks":[{"type":"whiteboard","min_count":1}]}}` |

字数和块数量要求可组合；字数单边无限制时写 `null`，“约 N 字”按 ±10%。受众、阅读任务、体裁、视觉策略、reason、purpose 等描述信息有助于后续创作时再填写，均可省略或为空。

缺失字段不启用对应检查。对用户明确提出的数量要求，写出完整条目，才能让 CLI 实际检查。

不预建临时目录、草稿或决策文件。以下命令适用于无可量化约束的情况；有约束时替换其中的 JSON：

```bash
lark-cli docs +script --command init-draft --presentation-decision '{}' --format json
```

成功后：

- 将 `data.cwd` 作为后续 CLI 调用的工作目录；将 `data.workspace` 原样记为 `work_dir`、`data.draft_path` 原样记为 `draft_path`。写文件工具需要绝对路径时使用 `<cwd>/<draft_path>`，CLI 使用 `@./<draft_path>`。
- CLI 会创建独占的 `work_dir` 并保存 `.presentation-decision.json` 作为固定基线，**但不会创建 `draft_path` 指向的 XML**。`draft_path` 是当前任务可直接写入的新文件路径；要求、资料或 contract 实质变化时，提交新决策并重新初始化，不得直接改基线。

### Step 5：生成 release candidate。

读取 [`lark-doc-xml.md`](lark-doc-xml.md)，并结合 Presentation Decision、适用 contract 和 Philosophy 生成完整 XML。使用扩展标签时按需读取 [`拓展标签`](lark-doc-xml-extended-blocks.md)。

1. 公开网络图片使用 `<img href="URL"/>`；已有本地图片使用 `<img path="@./downloads/image.png"/>`；画板使用 `<whiteboard type="svg" path="@./<work_dir>/diagram.svg"/>` 并遵循[`画板工作流`](lark-doc-whiteboard.md)；HTML 使用 `<html5-block path="@./<work_dir>/widget.html"/>` 并遵循[`拓展标签`](lark-doc-xml-extended-blocks.md)。
2. 直接在 `<cwd>/<draft_path>` 创建并写入完整 release candidate。新建资源建议放 `<cwd>/<work_dir>`，已有资源可原地复用；CWD 内优先用相对路径，其他位置用允许访问的绝对路径。XML 内相对资源先查 CWD，仅文件不存在时回退到 XML 所在目录。
3. 首次写入后，发现 XML 语法问题时只修复最小范围，不无故重写正确内容。

### Step 6：执行 Draft Profile Check。

1. 执行 `lark-cli docs +script --command parse --content "@./<draft_path>" --format json`。顶层 `ok` 仅表示命令执行成功，是否通过看 `data.assessment.status`。失败时按 `data.diagnostics[]` 局部修复；只有草稿为空、截断或结构无效时才全文重建。`parse` 不替代 XML 规则或服务端校验。
2. `passed` 只覆盖已启用的检查；先对照用户要求确认应声明的约束已完整填写，再按 [`lark-doc-xml.md`](lark-doc-xml.md) 复查标签、属性和值，并依据 Philosophy 检查事实与来源、用户硬约束、适用 contract / adapter 以及 `visual_plan`。最终 XML 能否写入以 `docs +create` 的服务端结果为准。

### Step 7：创建文档并处理局部失败。

1. 只有最新 release candidate 完成 Draft Profile Check 和 XML 规则复查后，才读取 [`lark-doc-create.md`](lark-doc-create.md)，使用同一个 `draft_path` 创建文档。
2. 创建结果存在 warning、局部资源失败或回查发现局部问题时，不得再次新建文档；读取 [`lark-doc-update.md`](lark-doc-update.md)，对已创建文档做最小范围修复，并按 update 流程 fetch 验证。

### Step 8：交付。

保留 Step 4 返回的 `work_dir` 及其中的创作草稿。最终只交付用户需要的结果，并说明必要来源、未关闭缺口、异常、失败或阻塞原因，以及文档 URL 或 token。
