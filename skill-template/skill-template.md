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
lark-cli {{service}} <resource> <method> --help  # 请求体骨架与字段说明，调用前先看这个
lark-cli schema {{service}}.<resource>.<method>   # 完整契约：深层嵌套结构、outputSchema
lark-cli {{service}} <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，先看 method `--help` —— 它内嵌了 `--data` 的请求体骨架与字段说明。只有深层嵌套结构，或需要完整契约（含 `outputSchema`）时才查 `schema`。两者都不要猜测字段格式。

{{resource_sections}}
## 权限表

| 方法 | 所需 scope |
|------|-----------|
{{permission_rows}}
{{/actions}}
