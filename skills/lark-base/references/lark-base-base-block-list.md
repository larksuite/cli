# base +base-block-list

列出 Base 容器管理的 block 条目，可按 folder 过滤直属子项。

## 推荐命令

```bash
lark-cli base +base-block-list \
  --base-token app_xxx

lark-cli base +base-block-list \
  --base-token app_xxx \
  --parent-id blk_folder
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--parent-id <block_id>` | 否 | folder 的 base block id；不传时列出全部 Base block |
| `--format <fmt>` | 否 | 输出格式：json / pretty / table / csv / ndjson |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 返回重点

- 返回后端 `blocks` 与 `total`。
- 每个 block 至少包含 `id`、`type`、`name`、`parent_id`。
- docx 类型可能额外包含 `docx_token`，用于后续 docx 文档操作。

## 坑点

- CLI 不暴露 `limit/offset`。如果结果超过后端当前上限，需要先调整 Base 结构或等待后端能力扩展。
- `+base-block-list` 是 Base 容器列表，不是表记录列表，也不是仪表盘组件列表。

## 参考

- [lark-base-base-block.md](lark-base-base-block.md) — base block 总览
