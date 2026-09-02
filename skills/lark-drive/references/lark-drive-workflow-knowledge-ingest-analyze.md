# 本地资料入库 — 盘点、对齐、分诊与映射

本文件是 [`lark-drive-workflow-knowledge-ingest.md`](lark-drive-workflow-knowledge-ingest.md) 的配套 phase 文档，在 workflow 进入 `INVENTORY`、`TARGET_ALIGN`、`NODE_PROPOSE` 和 `ANALYZE_TRIAGE` 状态时加载。

本文承载资料摄取前的**读与分析**细节：本地盘点、与目标库对齐（含降级）、承载节点提议、逐份资料分诊与映射。本阶段全程零飞书写入，唯一例外是 `NODE_PROPOSE` 经用户确认后的节点新建。发布计划、转换写入和验证见 [`lark-drive-workflow-knowledge-ingest-publish.md`](lark-drive-workflow-knowledge-ingest-publish.md)。

## INVENTORY：本地资料盘点

`inventory.py` 是确定性盘点脚本，只算元数据、不读正文、不修改原件。先从当前 `SKILL.md` 位置解析 skill 根目录绝对路径 `<SKILL_ROOT>`，再运行；不要假设当前工作目录，也不要用相对路径直接执行。

```bash
python3 "<SKILL_ROOT>/references/scripts/inventory.py" \
  --root "<用户授权的本地路径>" \
  --output-dir "<本次任务目录>/inventory"
```

脚本行为要点：

- **SHA-256 精确去重**：内容哈希相同归为同一 `duplicate_group`，`proposed_action` 标 `deduplicate_review`；只选一个主来源入库，其余保留溯源，不删文件。
- **敏感初筛**：按文件名命中敏感词（身份证 / 薪资明细 / 合同 / password 等）标 `risk_hint=possible_sensitive:*`；这是初筛线索，最终敏感等级在 `ANALYZE_TRIAGE` 据正文判定。
- **可解析性判断**：`parse_readiness` = `text_extractable`（可提取文本）/ `ocr_or_visual_review`（图片需 OCR / 视觉审阅）/ `manual_review`（需人工）/ `failed`（读取失败）。
- **符号链接安全**：拒绝符号链接作为授权根，跳过目录内符号链接（记入 `skipped_symlinks`），不通过快捷方式越界读取。

产物：`inventory/inventory.csv`（Excel 友好）+ `inventory/inventory.json`（含 `summary` 与逐条 `items`），填入 `Runtime State` 的 `inventory`。

### 增量识别（第二次及以后触发）

台账地基分层：

- **每页级持久状态**（复核日期 / 负责人 / 版本）写进知识页的 6 行治理表，跟随知识库、换人不丢。
- **本次运行的增量去重台账**为本地 `inventory.json` + `execution_ledger`，用于跳过与断点续跑。

增量分工：`inventory.py` 每次做**全量盘点**、不读旧台账；**增量对比由 agent 完成**——拿本次 `inventory.json` 与上次的按 `source_id`(SHA-256) 比对，未变资料标记跳过，只对新增 / 变化资料继续后续状态。本地台账缺失时，从目标库现状（`node_inventory`）重建最小基线继续，不直接写入。

用户可见输出：资料盘点概览（文件数、重复组数、可能敏感数、无法解析数；增量时标出跳过数）。样式见 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。

## TARGET_ALIGN：与目标库对齐

递归读取目标库节点树填充 `node_inventory`：每次 `wiki +node-list` 用 `--page-all`（或按 `page_token` 翻到 `has_more=false`），对 `has_child=true` 的节点逐层下钻，不得只取首页；任何一层未读全时置 `partial` 并记原因。

### 探测维护规范与对齐模式

逐节点用 `docs +fetch` 探测是否存在 `knowledge_base_bootstrap` 写入的维护规范（顶部 6 行治理表 + 收录范围 / 命名规范段落），填充 `standard_map`，并据覆盖情况设 `alignment_mode`：

| alignment_mode | 判定 | 处理 |
|----------------|------|------|
| `standard` | 目标节点均有维护规范 | 按规范收录范围与命名做映射（主路径，情况 4） |
| `degraded` | 目标节点均无维护规范 | 据资料内容 + 节点标题推断映射与命名；提示可先跑 `knowledge_base_bootstrap`（情况 3） |
| `mixed` | 部分节点有、部分没有 | 逐节点分流：有规范走 `standard`，无规范走 `degraded`（情况 6） |

### 降级映射的具体做法（无规范时）

无规范不阻塞入库，只是缺少归类 / 命名标尺。降级四件事：

1. **归到哪个节点**：读资料内容，与现有各节点**标题**语义匹配，归入最贴近者；匹配不出则归根节点或列「待人工归位」，不硬塞。
2. **起什么名**：无命名规范，按「资料主题 + 类型」生成通用标题（如 `退货政策说明`），不套格式模板。
3. **打标记**：`alignment_mode=degraded`；每份资料映射标 `mapping_confidence`（`low` / `medium`）。
4. **告知**：`TARGET_ALIGN` 提示无规范、映射据推断；发布计划逐项展示推断的节点与名字供用户核对可改；`DONE` 汇总再次声明本批为降级映射。

### 判定节点是否足以承载

节点为空（仅根节点）、或现有节点无法承载本批资料的主题分布时，判定「节点不足」，进入 `NODE_PROPOSE`（情况 1）。否则直接进入 `ANALYZE_TRIAGE`。

## NODE_PROPOSE：据真实资料提议承载节点

仅在节点不足以承载资料（情况 1 / 5）时进入。与 `knowledge_base_bootstrap` 的 `OUTLINE_PROPOSE` 区别：后者据**知识库主题**（尚无资料）提大纲，本状态据**已盘点的真实资料内容**提承载节点，因见过真实资料而更贴合。两者输入不同，不重复、不互调。

步骤：

1. 据 `inventory` 与已读资料内容，按主题聚类提议一组承载节点（拟建标题 + 目标位置 + 收录范围摘要），填充 `outline_proposal`。
2. 展示提议表（含每个拟建节点的精确 `--title`、`--parent-node-token` 或 `--space-id`、`--obj-type docx`），请用户确认。
3. **建节点是外部写入，必须用户确认后**才执行 `wiki +node-create --obj-type docx`；记录返回的 `node_token` / `obj_token`，回读并入 `node_inventory`。
4. 用户拒绝新建时，只把资料映射到现有节点或列「待人工归位」，不擅自新建。

提议表样式见 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。

## ANALYZE_TRIAGE：逐份资料分诊与映射

本状态 agent 真正**读取资料正文**（Word / PDF / 图片内容），逐份分析填充 `material_map`。至少完成：

### 判类与内容归纳

- 判定资料类别（制度 / 操作手册 / FAQ / 案例 / 台账等）与承担的知识职责。
- 一份资料对应一个主问题；据内容归纳，缺失信息记「待确认」不补造。

### 版本冲突识别

- 优先核对发布主体、版本号、生效 / 失效日期、适用范围、Owner；文件修改时间只是线索，不足以判新旧。
- 与目标节点已有生产页讲同一件事但对不上时，标 `conflict_status`：`suspected`（疑似）/ `confirmed`（确认冲突）/ `resolved`（已裁决）/ `none`。
- **无法自动裁决的冲突**：不覆盖现有生产页，保持现状，列入「需业务确认」；该资料由 `publish_gate.py` 阻塞发布，直到 `resolved`。

### 敏感判定

- 结合 `inventory` 的 `risk_hint` 与正文，定 `sensitivity`：`public` / `internal` / `restricted` / `prohibited`。
- `prohibited` 不入库；`restricted` 需 `sensitive_review_status=approved` 且用户对具体资料 + 目标范围明确确认，否则由门禁阻塞。

### 映射与命名

- 据 `standard_map` 把资料映射到 `target_node` 并生成拟定标题（`standard` 模式按规范收录范围与命名；`degraded` 模式据内容推断，标 `mapping_confidence`）。
- 给 `proposed_action`：`add`（新增）/ `update`（更新现有页）/ `merge`（合并到主页）/ `reference`（仅引用）/ `review`（待确认）/ `skip`（不入库）。

### Node Type Triage（对映射目标节点）

对每份资料的 `target_node` 按 entry 文件 `Node Type Triage` 分诊：`writable_docx` 走主路径；`non_docx_entity` 默认 `skip`（用户要才 `new_docx`）；`shortcut` 一律 `skip`。被 skip 的记入 `unsupported_checks` 并在汇总说明。

用户可见输出：资料分析表（判类、目标节点、拟定名、冲突 / 敏感标记、映射置信度、处置建议）。样式见 [`lark-drive-workflow-knowledge-ingest-outputs.md`](lark-drive-workflow-knowledge-ingest-outputs.md)。

## References

- [entry：knowledge_ingest 主文档](lark-drive-workflow-knowledge-ingest.md)
- [publish：发布计划、转换写入与验证](lark-drive-workflow-knowledge-ingest-publish.md)
- [outputs：模板](lark-drive-workflow-knowledge-ingest-outputs.md)
- [lark-wiki-node-list](../../lark-wiki/references/lark-wiki-node-list.md)、[lark-wiki-node-create](../../lark-wiki/references/lark-wiki-node-create.md)
- [lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)
- [knowledge_base_bootstrap](lark-drive-workflow-knowledge-base-bootstrap.md)
