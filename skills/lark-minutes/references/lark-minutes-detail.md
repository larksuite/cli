
# minutes +detail

通过 `minute_token` 查询妙记详情，按需获取 AI 产物（总结/待办/章节/逐字稿/关键词）。只读，支持 `--as user` / `--as bot`。

> `--summary` / `--todo` / `--chapter` / `--keyword` / `--transcript` 至少一个；不传任何产物 flag 时只返回基础信息（如 `title`），AI 产物字段都不会出现。一次性获取所有产物：`--summary --todo --chapter --keyword --transcript`。

## 命令

```bash
# 仅基础信息
lark-cli minutes +detail --minute-tokens obcxxxxxxxxxx

# 批量（逗号分隔，最多 50 个）
lark-cli minutes +detail --minute-tokens obcxxx,obcyyy --summary --todo

# 全产物
lark-cli minutes +detail --minute-tokens obcxxx --summary --todo --chapter --keyword --transcript

# 仅逐字稿，覆盖已有文件，指定输出目录
lark-cli minutes +detail --minute-tokens obcxxx --transcript --overwrite --output-dir ./out
```

## 输出

`minutes` 数组每条含 `minute_token`、`title`、`note_id`、`artifacts`。`note_id` 仅在该妙记关联了会议纪要时返回，可直接传给 [`note +detail`](../../lark-note/references/lark-note-detail.md) 拿纪要文档 token，无需再绕回 `vc +detail`。`artifacts` 中**只包含本次请求的产物**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `artifacts.summary` | string | AI 总结。 |
| `artifacts.todos` | array | 待办事项列表。 |
| `artifacts.chapters` | array | 章节列表。 |
| `artifacts.keywords` | array | 关键词列表。 |
| `artifacts.transcript_file` | string | 逐字稿本地文件路径。 |

逐字稿默认落地 `./minutes/{minute_token}/transcript.txt`，与 `minutes +download` 同目录便于聚合。指定 `--output-dir <dir>` 时改写到 `<dir>/artifact-{title}-{minute_token}/transcript.txt`。

## minute_token 来源

| 来源 | 取值字段 |
|------|---------|
| 妙记 URL `https://*.feishu.cn/minutes/obcxxx` | 截路径最后一段 `obcxxx` |
| `vc +detail --meeting-ids` | `minute_token` |
| `vc +recording --meeting-ids` | `minute_token` |
| `minutes +search` | `minute_token` |

## 典型链路：从 minute_token 拿纪要文档 token

只持有 `minute_token`（如妙记 URL 入口），又想拿 AI 智能纪要 / 逐字稿文档时；每一步都要沿用同一个 `--as`（完整规则见 [lark-shared](../../lark-shared/SKILL.md) 的「身份延续」）：

```bash
# 1. 取妙记关联的 note_id，没有关联会议纪要则为空
lark-cli minutes +detail --minute-tokens obcnxxxxxxxxxxxxxxxxxxxx --as bot

# 2. 用 note_id 拿 note_doc_token / verbatim_doc_token / shared_doc_tokens
#    沿用第 1 步的身份，不要省略 --as
lark-cli note +detail --note-id <note_id> --as bot

# 3. 读纪要 / 逐字稿正文（同样沿用第 1 步的身份）
lark-cli docs +fetch --api-version v2 --doc <note_doc_token> --doc-format markdown --as bot
```

> `minute_token` 不要直接传给 `note +detail`：必须先用本命令拿到 `note_id` 再调用 `note +detail`。

## 权限缺失 / 被禁用时的降级路径

`+detail --transcript` 依赖 `minutes:minutes.artifacts:read`（以及 `minutes:minutes.basic:read`）。当命令报 `missing_scope` 时，先区分两种情形：

| 情形 | 特征 | 处理 |
|------|------|------|
| 只是未授权 | 授权页能勾选并授予 scope | 按报错 hint 重新授权即可 |
| **企业后台禁用 scope** | 重新授权 N 次仍被拒（报 missing_scope / 授权后 scope 未生效） | 重授权无法解决，走下面的降级路径 |

> 判定技巧：授权后 `lark-cli auth status` 查看实际授予的 scopes 里有没有目标 scope。如果管理员在后台禁用了该 scope，任何授权请求都不会授予它。

### 降级路径（按优先级）

1. **优先路由到智能纪要（smart notes）**：妙记的媒体录制**不会自动授权给参会人**，但智能纪要和逐字稿（verbatim doc）**会后自动授权**。持有 `minute_token` 时：`+detail` 返回 `note_id` → `note +detail --note-id <note_id>` 拿 `verbatim_doc_token` → `docs +fetch --doc <verbatim_doc_token>` 读正文。
2. **下载录音后本地转写**：`minutes +download` 的媒体下载（`minutes:minutes.media:export`）可能仍被允许，即使 transcript 导出 scope 被禁。下载录音后用本地 ASR（如 whisper）转写。
3. **申请权限**：`minutes +apply-permission` 向妙记所有者申请 view/edit 权限，或联系企业管理员开通 scope。

> 相关 issue：[#2368](https://github.com/larksuite/cli/issues/2368)（missing-scope 错误应提示降级路径）
