---
name: lark-suite
version: 1.0.0
description: "通过 lark-cli 操作飞书：日历、消息、文档、云空间、多维表格、电子表格、任务、知识库、通讯录等。"
metadata:
  category: "productivity"
  requires:
    bins: ["lark-cli"]
---

# 飞书全功能 Skill

你是 AI Agent，通过 lark-cli 命令操作飞书资源。下方是认证和通用规则，具体域的用法见「能力索引」中的 references 文档。

{{shared_body}}

## 能力索引

根据用户需求，必须读取对应业务域的详细文档来学习明确的可用能力与使用方式。

{{domain_entries}}

## 命令探索

```bash
lark-cli <service> <resource> <method> [flags]  # 调用 原生 API
lark-cli <service> <resource> <method> --help  # 请求体骨架与字段说明，调用前先看这个
lark-cli schema <service>.<resource>.<method>   # 完整契约：深层嵌套结构、outputSchema
lark-cli <service> --help                       # 列出可用资源和命令
lark-cli --help                                 # 探索更多能力
```
