
# vc +detail

通过会议 ID 获取会议详情，包括基本信息、关联的纪要 ID（`note_id`）和妙记 Token（`minute_token`）。只读，支持 `--as user` / `--as bot`。

## 命令

```bash
# 单个 / 批量（逗号分隔，最多 50 个）
lark-cli vc +detail --meeting-ids <meeting_id1>,<meeting_id2>

# bot 身份（只能查 bot 有权限的会议）
lark-cli vc +detail --meeting-ids <meeting_id1>,<meeting_id2> --as bot
```

## 输出字段

| 字段 | 说明 |
|------|------|
| `meeting_id` | 会议 ID |
| `meeting_no` | 会议 9 位号码 |
| `topic` | 会议主题 |
| `start_time` | 开始时间 |
| `end_time` | 结束时间 |
| `note_id` | 关联的纪要 ID。 |
| `minute_token` | 关联的妙记 Token。 |

## 典型场景

### 场景 1：获取会议的纪要和妙记关联

`vc +detail` 只能拿到 `note_id` 和 `minute_token`，不直接返回纪要文档 token 与妙记产物内容。要获取实际产物，需根据用户诉求继续调用 `note +detail` 或 `minutes +detail`，**并沿用第 1 步同一个 `--as`**（完整规则见 [lark-shared](../../lark-shared/SKILL.md) 的「身份延续」）：

```bash
# 1. 获取会议详情，拿到 note_id 和 minute_token
lark-cli vc +detail --meeting-ids <meeting_id> --as bot

# 2. 用 note_id 获取纪要文档 Token（note_doc_token / verbatim_doc_token / shared_doc_tokens）
#    显式沿用第 1 步的身份，不要省略 --as
lark-cli note +detail --note-id <note_id> --as bot

# 3. 用 minute_token 获取妙记产物（同样沿用第 1 步的身份）
# ⚠️ 必须显式指定 --summary / --todo / --chapter / --keyword / --transcript 中至少一个 flag，
# 不传任何 flag 则不会返回任何产物内容。
lark-cli minutes +detail --minute-tokens obcnxxxxxxxxxxxxxxxxxxxx --todo --transcript --as bot
```

> **路由建议**：当用户未明确指定使用妙记时，**优先**走 `note +detail` 链路（纪要文档信息更完整、含逐字稿原文），仅在 `note_id` 为空或用户要求妙记产物时才走 `minutes +detail`。
> **身份边界**：`note +transcript` 目前仅支持 `--as user`。若上面第 2 步用 `--as bot` 拿到 `note_display_type=unified`，停下来向用户说明该纪要逐字稿只能以 user 身份读取，只有用户明确同意才切换身份重试。
