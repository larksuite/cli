# drive +version-history

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

列出指定文件的历史版本快照。该 shortcut 同时支持 `--as user` 和 `--as bot`；自动化场景推荐使用 `--as bot`。

## 命令

```bash
lark-cli drive +version-history \
  --file-token boxcnxxxxxxxx \
  --as bot

lark-cli drive +version-history \
  --file-token boxcnxxxxxxxx \
  --as user

lark-cli drive +version-history \
  --file-token boxcnxxxxxxxx \
  --limit 50 \
  --cursor 1777013761763 \
  --as bot

lark-cli drive +version-history \
  --file-token boxcnxxxxxxxx \
  --dry-run \
  --as bot
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标文件 token |
| `--limit` | 否 | 返回条数上限，范围 `1-200`，默认 `20` |
| `--cursor` | 否 | 分页游标；取上一页返回的 `next_cursor` 回填 |

## 关键行为

- shortcut 内部固定传 `only_tag=true`
- 返回 `has_more=true` 时，使用 `next_cursor` 继续翻页
- `versions[].version` 是传给 `drive +version-get` / `+version-revert` / `+version-delete` 的长数字版本串；`tag` 只是展示序号，不能替代 `version`
- `versions[].is_deleted` 为布尔值，表示该历史版本是否已被删除

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "versions": [
      {
        "version": "7633658129540910621",
        "name": "report.md",
        "edited_at": "1777013761763",
        "edited_by": "ou_xxx",
        "size_bytes": "12345",
        "action_type": "upload",
        "is_deleted": false,
        "tag": 7
      }
    ],
    "has_more": true,
    "next_cursor": "1777013761763"
  }
}
```

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
