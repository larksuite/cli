# drive +version-delete

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

删除指定的历史版本。当前 shortcut 推荐使用应用身份：`--as bot`。

## 命令

```bash
lark-cli drive +version-delete \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --as bot
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标文件 token |
| `--version` | 是 | 要删除的版本号 |

## 返回值

无额外业务字段，以命令成功 / 失败为准。

## 参考

- [lark-drive](../SKILL.md) -- 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
