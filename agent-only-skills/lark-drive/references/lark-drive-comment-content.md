# Drive 评论内容格式（--content）

> 本文是写入类评论命令（`+add-comment` / `+add-reply` / `+update-reply`）共享的 `--content` 内容格式说明，由这三个命令的 ref 引用。

`drive +add-comment`、`drive +add-reply`、`drive +update-reply` 的 `--content` 使用同一套 `reply_elements` JSON 数组格式。本文集中说明 schema、元素类型、转义和长度限制，各命令 ref 只保留最常见的纯文本例子。

## Schema

`--content` 是一个 JSON 数组字符串，至少一个元素。每个元素按 `type` 用对应字段承载值：

| type | 字段 | 值 |
|---|---|---|
| `text` | `text` | 普通文本正文 |
| `mention_user` | `mention_user` | 被 @ 用户的 open_id |
| `link` | `link` | 飞书云文档链接（docx/doc/sheet/bitable/wiki 等云文档 URL；对应 wire `docs_link`） |

最常见就是单个纯文本元素：

```bash
--content '[{"type":"text","text":"评论正文"}]'
```

组合多种元素：

```bash
--content '[
  {"type":"text","text":"请 "},
  {"type":"mention_user","mention_user":"ou_xxx"},
  {"type":"text","text":" 看下 "},
  {"type":"link","link":"https://your-tenant.feishu.cn/docx/<TOKEN>"}
]'
```

- `type=text` 的 `text` 不能为空；未知 `type` 会被拒绝，只允许 `text` / `mention_user` / `link`。
- 为省事，`mention_user` / `link` 的值也可以直接放在 `text` 字段（如 `{"type":"mention_user","text":"ou_xxx"}`），CLI 会识别；推荐用上表的专属字段，语义更清晰。
- `link` 是**飞书云文档链接**（wire 类型就叫 `docs_link`），不是任意网页链接。回复类命令（`+add-reply` / `+update-reply`）会校验，传外部 URL 被服务端拒绝（`1069302`），只接受飞书云文档 URL；`+add-comment` 对外部 URL 较宽松（能写入），但外部链接未必按云文档链接渲染，仍建议只放云文档 URL。


## 长度限制

- 所有 `type=text` 元素的字符（rune）总和上限 10000，按原始输入的字符数计（中英文、符号一视同仁，不是字节数、也不是转义后的长度）。
- 这是对**总额**的限制：把一段长文本拆成多个 text 元素不能绕过，它们共用同一个 10000 字符预算。
- `mention_user` / `link` 不计入该长度。
- 超限时 shortcut 在发送前拒绝并指出累计超长的元素；服务端对超限返回不透明的 `[1069302]`，所以这是预检。

## 参考

- [lark-drive-add-comment](lark-drive-add-comment.md) -- 添加评论
- [lark-drive-add-reply](lark-drive-add-reply.md) -- 回复评论
- [lark-drive-update-reply](lark-drive-update-reply.md) -- 更新回复
