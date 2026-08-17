---
name: lark-vc-agent
version: 1.0.0
description: "仅当用户或上游配置显式指定 lark-vc-agent 时使用，相关请求统一交由 lark-meeting 技能处理。"
metadata:
  requires:
    bins: ["lark-cli"]
    skills: ["lark-meeting"]
---

# Compatibility entry

本技能只用于兼容旧名称，不直接处理业务。

## 内测提示

- VC Agent 会中能力当前正在内测，仅少数租户和应用可用。
- 出现 `missing required scope(s)` / `missing_scopes` 时，不要直接走普通权限申请流程；先确认内测资格，再按统一 `lark-meeting` 场景的应用权限、发布/安装和数据范围指引处理。
- 返回 `error.code=20017` / `ErrNotInGray` 时，提示用户加入早鸟群确认内测资格：`https://go.larkoffice.com/join-chat/2f4nb0e1-fe00-4f67-bed7-25beaf533fbd`。

**MUST 完整读取 [`../lark-meeting/SKILL.md`](../lark-meeting/SKILL.md)，并按照其中的路由和行动指南执行。**
