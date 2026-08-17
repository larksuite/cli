# mail +auto-reply / +auto-reply-modify

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查看或修改用户邮箱自动回复设置。读写是两个独立 shortcut：读取用 `+auto-reply`，修改用 `+auto-reply-modify`。同一个发件人 4 天内仅会收到一次外出自动回复邮件。该自动回复不与其他邮箱服务同步，请勿重复设置。

## 命令

```bash
# 查看当前自动回复设置
lark-cli mail +auto-reply --as user

# 指定邮箱查看
lark-cli mail +auto-reply --as user --mailbox shared@example.com

# 开启或修改自动回复
lark-cli mail +auto-reply-modify --as user \
  --enable \
  --content '<p>我正在休假，回来后回复。</p>' \
  --start 2026-08-15 \
  --end 2026-08-18 \
  --timezone Asia/Shanghai

# 从文件读取正文
lark-cli mail +auto-reply-modify --as user --content @auto-reply.html

# 正文中的本地图片会自动内嵌
lark-cli mail +auto-reply-modify --as user --content '<p>休假中<img src="./logo.png"></p>'

# 关闭自动回复
lark-cli mail +auto-reply-modify --as user --disable
```

## 参数

| 参数 | 命令 | 必填 | 说明 |
|------|------|------|------|
| `--mailbox <email>` | 两者 | 否 | 邮箱地址，默认 `me` |
| `--enable` | modify | 否 | 开启自动回复；与 `--disable` 互斥 |
| `--disable` | modify | 否 | 关闭自动回复；与 `--enable` 互斥 |
| `--content <html>` | modify | 否 | 自动回复正文；支持直接传值、`@file` 和 `-` stdin；本地 `<img src="./file.png">` 会自动内嵌 |
| `--content-file <path>` | modify | 否 | 从路径读取正文；与 `--content` 互斥；正文里的本地图片会自动内嵌 |
| `--summary <text>` | modify | 否 | 正文摘要；不传时由 `--content` / `--content-file` 自动生成预览 |
| `--start <time>` | modify | 否 | 开始日期，支持 Unix timestamp 或 ISO 8601；按当天开始保存 |
| `--end <time>` | modify | 否 | 结束日期，支持 Unix timestamp 或 ISO 8601；按当天结束保存 |
| `--timezone <tz>` | modify | 否 | 时区，例如 `Asia/Shanghai` |
| `--internal-only` | modify | 否 | 仅对租户内发件人发送自动回复；与 `--external` 互斥 |
| `--external` | modify | 否 | 外部发件人也发送自动回复；与 `--internal-only` 互斥 |

## 行为

- `+auto-reply` 只调用读取接口，需要 `mail:user_mailbox.message:readonly`。
- `+auto-reply-modify` 只更新用户提供的选项，未指定的配置会保留。
- 修改需要 `mail:user_mailbox.message:readonly` 和 `mail:user_mailbox.message:modify`。
- 写操作必须先向用户展示预览并取得明确确认。预览至少包含：`enabled`、时间范围、时区、收件范围和内容摘要。
- 关闭自动回复也要确认，因为内容和时间配置可能仍会保留在设置中。

## 返回值

输出为结构化 envelope，核心字段在 `data.auto_reply`：

```json
{
  "ok": true,
  "data": {
    "auto_reply": {
      "enabled": true,
      "content_html": "<p>我正在休假，回来后回复。</p>",
      "content_summary": "我正在休假，回来后回复。",
      "start_time": "1786723200000",
      "end_time": "1787068799999",
      "time_zone": "Asia/Shanghai",
      "only_send_to_tenant": false
    }
  }
}
```

## 字段说明

| 字段 | 说明 |
|------|------|
| `enabled` | 是否开启自动回复 |
| `content_html` | 自动回复 HTML 正文 |
| `content_summary` | 自动回复摘要 |
| `start_time` | 毫秒级开始日期时间戳 |
| `end_time` | 毫秒级结束日期时间戳 |
| `time_zone` | 自动回复时间范围对应的时区 |
| `only_send_to_tenant` | 是否仅对租户内发件人发送自动回复 |
