---
name: lark-{{project}}
version: {{meta_version}}
description: "{{meta_description}}"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli {{service}} --help"
---

# {{service}} ({{version}})

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

{{introduction}}
{{#shortcuts}}
## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli {{service}} +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
{{shortcut_rows}}
{{/shortcuts}}
{{#actions}}
## API Resources

```bash
lark-cli schema {{service}}.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli {{service}} <resource> <method> [flags] # 调用 API
```

{{schema_note_block}}

{{resource_sections}}
{{permission_block}}
{{/actions}}
