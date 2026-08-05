# drive +update-title

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

重命名云空间（云盘/云存储）里的文件、文件夹、在线文档或知识库节点。

## 命令

```bash
# 推荐：传 URL（自动识别类型和 token）
lark-cli drive +update-title \
  --url 'https://example.larksuite.com/docx/<DOCX_TOKEN>' \
  --title '<NEW_TITLE>'

# 裸 token 必须显式传 --type
lark-cli drive +update-title \
  --token <FILE_TOKEN> \
  --type file \
  --title '<NEW_TITLE>.xlsx'

# 知识库节点：传 /wiki/ URL 里的 node_token
lark-cli drive +update-title \
  --url 'https://example.larksuite.com/wiki/<NODE_TOKEN>' \
  --title '<NEW_TITLE>'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 与 `--token` 二选一 | 目标 URL，支持 `/docx/`、`/sheets/`、`/base/`、`/bitable/`、`/slides/`、`/file/`、`/drive/folder/`、`/wiki/` |
| `--token` | 与 `--url` 二选一 | 目标 token 或 URL；裸 token 必须配合 `--type` |
| `--type` | 裸 token 时必填 | `docx`、`sheet`、`bitable`（`base` 为兼容别名）、`slides`、`file`、`folder`、`wiki`；传 URL 时可省略，显式传入时必须与 URL 类型一致 |
| `--title` | 是 | 新标题，别名 `--new-title`；不能为空或纯空白，首尾空格会被去掉 |
| `--on-extension-mismatch` | 否 | 仅 `--type file`：`keep`（默认，标题缺后缀时自动补上当前后缀，后缀不一致时报错）/ `allow`（跳过校验，原样提交）。传给其他 `--type` 会报错 |

## 行为说明

- **空标题会被拒绝**：CLI 拒绝空或纯空白的 `--title`
- **`file` 类型会校验后缀**：`--type file` 的标题就是完整文件名。CLI 会比对 `--title` 与当前文件名的后缀：没有后缀时默认补上当前后缀（输出里用 `extension_appended` 说明），后缀不一致时拦截（`a.md` → `a.txt`）。要跳过校验加 `--on-extension-mismatch=allow`
- **wiki 不解包**：`--type wiki` 用 `/wiki/` URL 里的 `wiki_token`，传底层文档 token 会 `981003`
- **不支持旧版 doc 和思维笔记**：服务端不支持改这两类的标题（`type=doc` / `type=mindnote` 返回 `981002 params error`），CLI 在本地就拒绝，不会白发一次写请求
- **不支持妙搭 apps**：要改妙搭应用标题，切换到 [`lark-apps`](../../lark-apps/SKILL.md) 业务域处理

## 输出

```json
{
  "updated": true,
  "file_token": "<file_token>",
  "type": "docx",
  "title": "<new_title>",
  "url": "https://example.feishu.cn/docx/<file_token>"
}
```

`--type file` 且未用 `allow` 时，额外返回改名前的文件名，改错了可以据此一条命令改回去；自动补了后缀还会带上 `extension_appended`：

```json
{
  "updated": true,
  "title": "<new_title>.txt",
  "previous_title": "<old_title>.txt",
  "extension_appended": ".txt"
}
```

## 常见错误

| 错误码 | 含义 | 处理 |
|---|---|---|
| `99991672` / `99991679` | 缺失 scope | 按错误里的 `missing_scopes`、`hint` 申请/授权所需 scope 后重试 |
| `99991400` | 命中接口限频 | 等待一段时间后重试；批量改名时保持串行并降低频率 |

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
