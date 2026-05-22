# base +base-block-rename

重命名 Base 容器里的 block。

## 推荐命令

```bash
lark-cli base +base-block-rename \
  --base-token app_xxx \
  --block-id blk_xxx \
  --name "新的名称"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--block-id <block_id>` | 是 | 要重命名的 base block id |
| `--name <name>` | 是 | 新名称 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 返回重点

- 返回 `block` 和 `renamed=true`。

## 坑点

- `--name` 必填，且不能为空白字符串。
- 这是重命名 Base 容器入口；具体资源内部标题是否同步，以后端语义为准。

## 参考

- [lark-base-base-block.md](lark-base-base-block.md) — base block 总览
