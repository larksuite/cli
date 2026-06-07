---
name: lark-approval
version: 1.0.0
description: "飞书审批：查询待办任务、查询审批实例、处理审批任务（同意/拒绝/转交/加签/退回/催办）。当用户需要查询待办任务、查看审批详情、处理待审批任务、催办审批人、撤回已发起的审批时使用。不负责：创建审批定义/表单设计（走原生 OpenAPI）、发起新审批（需通过飞书客户端或走原生 OpenAPI发起）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli approval --help"
---

# approval (v4)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**身份**：审批操作默认使用 `--as user`（以当前登录用户身份处理审批任务）

## API Resources


### instances

  - `get` — 获取单个审批实例详情
  - `cancel` — 撤回审批实例
  - `cc` — 抄送审批实例
  - `initiated` — 查询用户的已发起列表

### tasks

  - `remind` — 催办审批人
  - `approve` — 同意审批任务
  - `reject` — 拒绝审批任务
  - `transfer` — 转交审批任务
  - `query` — 查询用户的任务列表
  - `add_sign` — 审批任务加签
  - `rollback` — 退回审批任务

## 不在本 skill 范围

- 创建审批定义/表单设计 → 原生 OpenAPI（通过 lark-openapi-explorer 查找）
- 发起新审批实例 → 原生 OpenAPI
- 待办任务（非审批类）→ [`lark-task`](../lark-task/SKILL.md)