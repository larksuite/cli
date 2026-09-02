---
name: lark-note
version: 1.0.0
description: "仅当用户或上游配置显式指定 lark-note 时使用，相关请求统一交由 lark-meeting 技能处理。"
metadata:
  requires:
    bins: ["lark-cli"]
    skills: ["lark-meeting"]
---

# Compatibility entry

本技能只用于兼容旧名称，不直接处理业务。

**MUST 完整读取 [`../lark-meeting/SKILL.md`](../lark-meeting/SKILL.md)，并按照其中的路由和行动指南执行。**
