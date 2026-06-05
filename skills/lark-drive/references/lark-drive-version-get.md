# drive +version-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

下载指定版本的文件内容。该 shortcut 同时支持 `--as user` 和 `--as bot`；自动化场景推荐使用 `--as bot`。

## 命令

```bash
lark-cli drive +version-get \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --as bot

lark-cli drive +version-get \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --as user

lark-cli drive +version-get \
  --file-token boxcnxxxxxxxx \
  --version 7633658129540910621 \
  --output ./downloads/ \
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
| `--version` | 是 | `drive +version-history` 返回的长数字 `version` 字段，不是 `tag` |
| `--output` | 否 | 本地保存路径或目录；省略时保存到当前目录，并优先使用服务端文件名 |
| `--overwrite` | 否 | 覆盖已存在的本地输出文件 |

## 关键行为

- 省略 `--output` 时，CLI 保存到当前目录，并优先使用服务端文件名
- `--output` 指向已存在目录，或以 `/` / `\\` 结尾时，CLI 会使用远端文件名保存
- `--output` 是文件路径且没有后缀时，CLI 会像 `docs +media-download` 一样尝试从响应头推断后缀；推不出来就保持无后缀
- 目标文件已存在时，只有显式传 `--overwrite` 才会覆盖

## 返回值

返回值：

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

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
