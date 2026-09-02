# 本地资料入库 — 发布计划模板与输出模板

本文件是 [`lark-drive-workflow-knowledge-ingest.md`](lark-drive-workflow-knowledge-ingest.md) 的配套引用文档，在 workflow 进入 `NODE_PROPOSE`、`ANALYZE_TRIAGE`、`PUBLISH_PLAN`、`VERIFY` 等需要展示结构化输出的状态时加载。

本文承载两类内容：`publish_gate.py` 的发布计划 JSON schema 与 6 行治理表模板，以及各状态用户可见输出的表格样式。所有模板是结构骨架，实际文本由 agent 结合资料内容、目标节点和维护规范填充。列名使用中文；内部枚举值在用户可见输出中转为自然语言中文标签。

## 6 行治理表（每个知识页统一表头）

每个新建或更新的知识页顶部套一张 6 行治理表，字段名称与顺序保持一致，不因资料类型删减。

| 字段 | 填写规则 | 门禁相关 |
|------|----------|----------|
| 来源 | 资料的原始出处：原文件名 + 版本 / 日期，多来源逐条列出；转换页标注原 PDF 页码或原文件 | 不得为空 |
| 负责人 | 该知识页内容负责人，优先「姓名｜团队」。未知填「待确认」 | 待确认时 `page_status` 须为进行中 |
| 版本与状态 | 写成 `v1.0｜已完成`；状态只能是 `进行中 / 已完成 / 已废弃` | 状态须合规；有「待确认」时不得为已完成 |
| 适用与可见范围 | 适用的组织、岗位、人群和可见范围；全员适用也显式写明 | 不得为空 |
| 生效与更新 | 写成 `生效：YYYY-MM-DD｜更新：YYYY-MM-DD｜原因：首次入库 + 说明`；无独立生效日期写「不适用（原因）」 | 允许「待确认」 |
| 复核策略 | 写成 `类型：<知识类型>｜周期：<天>｜下次复核：YYYY-MM-DD` | 允许「待确认」 |

字段值未确定时统一填「待确认」，不得编造。含「待确认」或仅部分解析（partial）的页面 `page_status` 保持「进行中」；只有 6 行齐备、无「待确认」、完整解析的页面才可标「已完成」。

## 发布计划 JSON schema（publish_gate.py 输入）

`PUBLISH_PLAN` 生成的发布计划为 JSON 对象，顶层 `items` 数组，每项字段：

```json
{
  "items": [
    {
      "source_id": "<SHA-256 或稳定来源 ID>",
      "title": "退货政策说明",
      "publish_role": "knowledge_page",
      "write_via": "import_docx",
      "target_obj_type": "docx",
      "target_token": "wikcn_NODE",
      "sensitivity": "internal",
      "sensitive_review_status": "",
      "conflict_status": "none",
      "parse_status": "parsed",
      "attachment_confirmed": false,
      "governance": {
        "source": "退货政策原始 Word｜2026-08 版",
        "owner": "李四｜客服团队",
        "version_status": "v1.0｜已完成",
        "scope_visibility": "全员",
        "effective_update": "生效：2026-09-01｜更新：2026-09-01｜原因：首次入库",
        "review_policy": "类型：政策｜周期：180｜下次复核：2027-03-01",
        "page_status": "已完成"
      }
    }
  ]
}
```

字段取值：

- `publish_role`：`knowledge_page`（默认，需 governance）/ `source_attachment`（原文件附件，需 `attachment_confirmed=true`、`write_via=drive_upload`，无 governance）。
- `write_via`：`import_docx` / `docs_update` / `node_create_docx`（知识页三种写法）/ `drive_upload`（仅附件）。
- `target_obj_type`：目标节点对象类型；知识页必须为 `docx`。
- `sensitivity`：`public` / `internal` / `restricted` / `prohibited`。
- `conflict_status`：`none` / `suspected` / `confirmed` / `resolved`。
- `parse_status`：`parsed` / `partial` / `unsupported` / `failed`。

门禁输出为 `{ok, summary{total,writable,blocked,narrowed,attachments}, items[]}`，每项含 `ready`、`blocked_reasons`、`narrowed`、`narrow_reasons`、`counts_as_page`、`effective_page_status`。

## 用户可见输出模板

### INVENTORY：资料盘点概览

```text
本地来源：<授权路径>
资料总数：<N>（可提取文本 <a> / 需 OCR 视觉 <b> / 需人工 <c> / 读取失败 <d>）
精确重复组：<G>（组内仅保留一个主来源入库）
可能敏感（按文件名）：<S>
跳过的符号链接：<L>
增量：跳过未变资料 <U>，本次处理 <M>   （首次运行省略此行）
```

### TARGET_ALIGN：结构与规范对齐

```text
目标知识库：<名称>
节点总数：<N>；对齐模式：按规范 / 降级推断 / 混合

| 节点标题 | 类型 | 维护规范 | 收录范围（摘要） |
|----------|------|----------|------------------|
| 退货售后 | 文档 | 有 | 退货政策、售后流程、FAQ |
| 物流配送 | 文档 | 无（降级） | （据资料内容推断） |

降级提示（无规范时）：目标节点缺少维护规范，本次映射将据资料内容与节点标题推断，
建议先运行 knowledge_base_bootstrap 立规范以获得统一收录范围与命名。
```

维护规范列：有 / 无（降级）。类型列：文档 / 表格 / 多维表格 / 思维笔记 / 幻灯片 / 快捷方式。

### NODE_PROPOSE：承载节点提议表

仅在节点不足以承载资料时展示。据真实资料内容提议，确认后新建。

```text
现有节点不足以承载本批资料。基于已盘点资料内容，提议以下承载节点（新建为文档节点，可增删改名）：

| 拟建 --title | 目标位置（parent） | 对象类型 | 收录范围（据资料摘要） | 对应资料数 |
|--------------|--------------------|----------|------------------------|-----------|
| 退货售后 | wikcn_ROOT | docx | 退货政策、售后流程 | 5 |
| 物流配送 | wikcn_ROOT | docx | 配送时效、区域政策 | 3 |

确认后对每个节点执行：
  wiki +node-create --as user --parent-node-token wikcn_ROOT --title "<拟建标题>" --obj-type docx
  → 记录返回 node_token / obj_token，回读并入 node_inventory

确认新建这些节点吗？也可增删 / 改名后再建；如只想归入现有节点，可跳过新建。
```

### ANALYZE_TRIAGE：资料分析表

```text
| 资料 | 类别 | 目标节点 | 拟定标题 | 处置 | 冲突 | 敏感 | 映射置信度 |
|------|------|----------|----------|------|------|------|-----------|
| 退货政策.docx | 政策 | 退货售后 | 退货政策说明 | 新增 | 无 | 内部 | 高 |
| 退货政策_旧.pdf | 政策 | 退货售后 | — | 待确认 | 疑似冲突 | 内部 | 高 |
| 员工薪资表.xlsx | 台账 | — | — | 不入库 | 无 | 受限 | — |
```

处置列：新增 / 更新 / 合并 / 仅引用 / 待确认 / 不入库。冲突列：无 / 疑似冲突 / 确认冲突 / 已裁决。敏感列：公开 / 内部 / 受限 / 禁止。映射置信度列：高 / 中 / 低（降级映射时给出）。

### PUBLISH_PLAN：发布计划 + 三清单

R2-R3 高风险写入。确认前必须展示每份资料的目标节点稳定标识、`write_via`、门禁结果，以及将写入的精确内容或相对现状的 diff——不能只给标题和摘要。

```text
即将入库 <M> 份资料，门禁拦截 <B> 份，状态收紧 <N> 份，跳过 <K> 份。写入为高风险操作，确认后执行。

| 资料 | 目标节点 (token) | 角色 | 写法 (write_via) | 门禁 | 写入内容 |
|------|------------------|------|------------------|------|----------|
| 退货政策.docx | 退货售后 (wikcn_A) | 知识页 | import_docx | 通过 | 退货政策知识页（全文见下） |
| 配送时效.pdf | 物流配送 (wikcn_B) | 知识页 | docs_update | 收紧为进行中：仅部分解析 | 配送时效正文（标页码，全文见下） |
| 退货政策原件.pdf | 退货售后 (wikcn_A) | 来源附件 | drive_upload | 通过（附件，不计页面） | 原文件留证 |

<逐份展开将写入的精确正文与治理表；import_docx 标明整理要点，docs_update 标明来源页码>

门禁拦截（不写入，记入 unsupported_checks）：
| 资料 | 原因 |
|------|------|
| 退货政策_旧.pdf | 存在未裁决冲突（conflict_status=confirmed） |
| 员工薪资表.xlsx | 受限敏感内容未通过审核 |

冲突清单：<列出疑似 / 确认冲突资料及差异，待业务裁决>
敏感清单：<列出 restricted / prohibited 资料及处置>
无法解析清单：<列出 unsupported / failed 资料及原因>
```

门禁列：通过 / 通过（附件，不计页面）/ 收紧为进行中（原因）/ 拦截（原因）。角色列：知识页 / 来源附件。

### VERIFY：验证与汇总

```text
| 资料 | 目标节点 | 写入 | 校验 |
|------|----------|------|------|
| 退货政策.docx | 退货售后 | 成功 | 已确认 docx 正文落地 |
| 配送时效.pdf | 物流配送 | 成功 | 已确认正文落地（进行中） |

汇总：已入库 <M> 份知识页，附件 <A> 个，跳过 <K> 份，失败 <F> 份。
未入库（unsupported_checks）：
| 资料 | 原因 |
|------|------|
| 退货政策_旧.pdf | 未裁决冲突，待业务确认 |
| 员工薪资表.xlsx | 受限敏感未审 |

台账位置：<本次任务目录>/inventory + execution_ledger
知识库链接：<URL>
对齐模式：<按规范 / 降级推断 / 混合>（降级时提示本批映射未依据维护规范）
```

校验列：已确认落地 / 已确认落地（进行中）/ 未落地（需重试）/ 失败。

## References

- [entry：knowledge_ingest 主文档](lark-drive-workflow-knowledge-ingest.md)
- [analyze：盘点、对齐、分诊与映射](lark-drive-workflow-knowledge-ingest-analyze.md)
- [publish：发布计划、转换写入与验证](lark-drive-workflow-knowledge-ingest-publish.md)
- 门禁脚本：`scripts/publish_gate.py`（及测试 `scripts/publish_gate_test.py`）
