# base +base-block-move

移动 Base 容器里的 block，可移动到根层级或某个 folder 下，并可指定同级顺序。

## 推荐命令

```bash
# 移动到根层级
lark-cli base +base-block-move \
  --base-token app_xxx \
  --block-id blk_xxx

# 移动到文件夹里
lark-cli base +base-block-move \
  --base-token app_xxx \
  --block-id blk_xxx \
  --parent-id blk_folder

# 移动到某个同级条目之后
lark-cli base +base-block-move \
  --base-token app_xxx \
  --block-id blk_xxx \
  --parent-id blk_folder \
  --after-id blk_sibling
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--block-id <block_id>` | 是 | 要移动的 base block id |
| `--parent-id <block_id>` | 否 | 目标 folder 的 base block id；不传表示移动到根层级 |
| `--before-id <block_id>` | 否 | 放到该同级 block 前 |
| `--after-id <block_id>` | 否 | 放到该同级 block 后 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 返回重点

- 返回 `block` 和 `moved=true`。

## 坑点

- `--before-id` 与 `--after-id` 互斥，不能同时传。
- 不传 `--parent-id` 表示根层级，不需要也不要输入 `null`。
- 移动 folder 是移动整个 folder 入口，子项仍归属该 folder。

## 参考

- [lark-base-base-block.md](lark-base-base-block.md) — base block 总览
