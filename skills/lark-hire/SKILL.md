---
name: lark-hire
version: 1.0.0
description: "飞书招聘 (Feishu/Lark Hire)：管理职位、人才、投递、Offer 等招聘全流程。查询职位与详情、管理人才库、跟踪投递进度与招聘阶段、管理 Offer。当用户需要查职位/候选人、看投递管线、查 Offer，或操作招聘数据时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli hire --help"
---

# hire (招聘)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、身份（`--as user|bot`）、权限与错误处理等通用规则。**

`lark-cli hire` 覆盖飞书招聘开放接口，命令形如 `lark-cli hire <resource> <method>`。
查看接口参数：`lark-cli schema hire.<resource>.<method>`。

## Core Concepts

- **Job 职位** (`job`, `job_id`): 招聘岗位。`hire job list` 列表、`hire job get` 详情。
- **Talent 人才** (`talent`, `talent_id`): 候选人，人才库核心实体。`hire talent batch_get_id` 可按手机/邮箱查 ID。
- **Application 投递** (`application`, `application_id`): 某人才对某职位的一次投递，串联整个流程；可按 `job_id`/`talent_id`/`stage_id` 过滤。
- **Offer** (`offer`, `offer_id`): 录用意向，有状态流转。`offer list` 需带 `talent_id`。

## Resource Relationships

```text
Job (job_id)
└── Application (application_id)   ← talent + job
    ├── Talent (talent_id)
    ├── Stage 招聘阶段
    └── Offer (offer_id)
```

## 常用只读工作流

```bash
# 职位列表（参数走 --params JSON；分页用 --page-all）
lark-cli hire job list --params '{"page_size":"20"}' --jq '.data.items[] | {id, title}'

# 职位详情（路径参数 job_id 放进 --params）
lark-cli hire job get --params '{"job_id":"<job_id>"}'

# 某职位下的投递（自动翻页）
lark-cli hire application list --params '{"job_id":"<job_id>"}' --page-all -q '.data.items[]'

# 按手机号查人才 ID，再查其投递
lark-cli hire talent batch_get_id --data '{"mobile_code":"86","mobile_number_list":["138..."]}'
lark-cli hire application list --params '{"talent_id":"<talent_id>"}'

# 某人才的 Offer（talent_id 必填）
lark-cli hire offer list --params '{"talent_id":"<talent_id>"}'
```

## 写操作

写/删除接口带 `Risk: write` / `high-risk-write`（见 `lark-cli schema hire.<r>.<m>`）。务必先 `--dry-run` 预览，确认后再执行；body 用 `--data`（支持 `@file` 与 `-` stdin）。

```bash
lark-cli hire talent combined_create --data @talent.json --dry-run
lark-cli hire talent combined_create --data @talent.json
```

## 要点

- 路径参数与查询参数都放进 `--params` 的 JSON；请求体放 `--data`。
- 列表响应取 `.data.items[]`；详情取 `.data.<resource>`（如 `.data.job`、`.data.talent`、`.data.application`）。
- 不同接口 `page_size` 上限不同（职位 20，投递 100）。
- 身份默认 `--as bot`（tenant_access_token）；接口若标注支持 user，可 `--as user`。

更多工作流见 [`references/recruitment-pipeline.md`](references/recruitment-pipeline.md)。
