
# drive +inspect（文档 URL 检视：类型、标题、Token 解析）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

给定一个飞书文档 URL 或 bare token，返回其类型、标题和 canonical token。对 wiki URL 自动解包到底层文档。

## 命令

```bash
# 检视一个 docx URL
lark-cli drive +inspect --url 'https://xxx.feishu.cn/docx/doxcnXXX'

# 检视一个 wiki URL（自动解包到底层文档）
lark-cli drive +inspect --url 'https://xxx.feishu.cn/wiki/wikcnXXX'

# bare token 需要指定 --type
lark-cli drive +inspect --url doxcnXXX --type docx

# 格式化输出
lark-cli drive +inspect --url 'https://xxx.feishu.cn/base/bascnXXX' --format pretty
```

## 输出

JSON 输出包含以下字段：

| 字段 | 说明 |
|------|------|
| `input_url` | 原始输入 URL |
| `type` | 文档类型（docx, doc, sheet, bitable, wiki, file, folder, mindnote, slides） |
| `title` | 文档标题 |
| `token` | canonical file token |
| `url` | 重建的 canonical URL |
| `wiki_node` | 仅 wiki URL：包含 `space_id`, `node_token`, `obj_token`, `obj_type` |

## 典型场景

| 场景 | 命令 |
|------|------|
| 用户给了一个 URL，想知道它是什么类型的文档 | `lark-cli drive +inspect --url '<url>'` |
| wiki 链接需要拿到底层文档的 token 来做后续操作 | `lark-cli drive +inspect --url '<wiki_url>'`，取输出中的 `token` |
| 只有 token 没有 URL | `lark-cli drive +inspect --url <token> --type <type>` |

## 注意事项

- `--url` 为必填参数
- 当 `--url` 是 bare token（非完整 URL）时，`--type` 也是必填的
- wiki URL 会自动调用 `get_node` API 解包，输出中 `type` 和 `token` 是底层文档的类型和 token
- `+inspect` 只用于识别/消歧；如果任务已能通过 URL 路径形态完成路由判断，不必把它作为所有 Drive 操作的通用前置步骤
- `+inspect` 失败后不要自动切到写接口继续尝试，先按错误提示处理权限、scope 或链接问题
- 支持 `--dry-run` 查看将调用的 API 步骤
