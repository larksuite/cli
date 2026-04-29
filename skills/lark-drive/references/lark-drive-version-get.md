# drive +version-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

下载指定版本的文件内容。当前 shortcut 推荐使用应用身份：`--as bot`。

## 命令

```bash
lark-cli drive +version-get \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --as bot

lark-cli drive +version-get \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --output ./artifact.bin \
  --overwrite \
  --as bot
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 目标文件 token |
| `--version` | 是 | 目标版本号 |
| `--output` | 否 | 本地保存路径；省略时沿用 `drive +download` 的默认落点行为 |
| `--overwrite` | 否 | 覆盖已存在的本地输出文件 |

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "file_token": "boxcnxxxxxxxx",
    "version": "7633658129540910621",
    "file_name": "artifact.bin",
    "saved_path": "/abs/path/artifact.bin",
    "size_bytes": 12345
  }
}
```

## 参考

- [lark-drive](../SKILL.md) -- 云空间全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
