# calendar +join-event

凭**分享 token** 加入日程。

## 命令

```bash
# 用户以自身身份加入（默认场景）
lark-cli calendar +join-event --token <token> --as user

# 以应用身份加入
lark-cli calendar +join-event --token <token> --as bot
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token <token>` | **是** | 分享 token，加入的唯一入参（别名 `--share-token`）。|

## token 从哪来

| token 类型 | 承载来源 | 取值 |
|-----------|---------|------|
| 链接类 | 分享链接 / 二维码 | 链接 `{{domain}}/calendar/share?token=<token>` 里的 `token` |
| 卡片类 | 分享卡片 / RSVP 卡片 | 从 IM 日程分享卡片或 RSVP 卡片消息解析出的日程分享 token |

- **分享链接**：直接取 URL query 里的 `token` 值传入；无需解析日程字段。例如 `{{domain}}/calendar/share?token=29f762bdmsbd82ce9` → `--token 29f762bdmsbd82ce9`。
- **二维码**：先用 OCR/扫码解析成分享链接，再取其中的 `token`——CLI 不承接二维码图像，只承接解析后的链接 token。
- **卡片**：token 落在卡片消息 content（分享卡片 `SHARE_CALENDAR_EVENT`、RSVP 卡片 `GENERAL_CALENDER`）；RSVP 卡片被转发后退化为分享卡片，同样可加入。

## 重复性日程

加入范围取决于 token 反解出的日程本体是「原重复性日程」还是「例外」（参见 [lark-calendar-recurring](lark-calendar-recurring.md) 的关键概念）：

- 分享的是**原重复性日程**（`{event_uid}_0`）：加入的是**整个序列**（含例外）。
- 分享的是某个**例外**（`originalTime > 0` 的单次实例）：只加入这**一个例外日程**。

## 参考

- [lark-calendar](../SKILL.md) -- skill 入口与路由
- [lark-calendar-rsvp](lark-calendar-rsvp.md) -- 已在日程中时回复接受/拒绝/待定（≠ 加入）
- [lark-calendar-recurring](lark-calendar-recurring.md) -- 重复性日程的序列 vs 实例操作规范
