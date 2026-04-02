# base record shortcuts

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

record 相关命令索引。

## 命令导航

| 文档 | 命令 | 说明 |
|------|------|------|
| [lark-base-record-list.md](lark-base-record-list.md) | `+record-list` | 分页列记录 |
| [lark-base-record-get.md](lark-base-record-get.md) | `+record-get` | 获取单条记录 |
| [lark-base-record-upsert.md](lark-base-record-upsert.md) | `+record-upsert` | 创建或更新记录 |
| [lark-base-record-upload-attachment.md](lark-base-record-upload-attachment.md) | `+record-upload-attachment` | 上传本地文件到附件字段并更新记录 |
| [lark-base-record-delete.md](lark-base-record-delete.md) | `+record-delete` | 删除记录 |

## 说明

- 聚合页只保留目录职责；每个命令的详细说明请进入对应单命令文档。
- 所有 `+xxx-list` 调用都必须串行执行；若要批量跑多个 list 请求，只能串行执行。
- `+record-list / +record-get` 的字段筛选统一使用 repeatable `--field`；每次传一个字段，可重复多次传入多个字段，接受字段 ID 或字段名。
- 写记录 JSON 前优先阅读 [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md)。
- 本地文件写入附件字段时，必须使用 `+record-upload-attachment`。
