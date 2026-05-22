# base +base-block-delete

删除 Base 容器里的 block。

## 推荐命令

```bash
lark-cli base +base-block-delete \
  --base-token app_xxx \
  --block-id blk_xxx \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--block-id <block_id>` | 是 | 要删除的 base block id |
| `--yes` | 是 | 高风险写入确认 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 返回重点

- 返回 `block` 和 `deleted=true`。

## 坑点

- 不支持递归删除 folder。删除非空 folder 前，先移动或删除其子项。
- 删除的是 Base 容器里的 block。不同类型底层资源是否会被物理删除，以后端语义为准。
- 删除前建议先用 `+base-block-list` 确认 `block-id`。

## 参考

- [lark-base-base-block.md](lark-base-base-block.md) — base block 总览
