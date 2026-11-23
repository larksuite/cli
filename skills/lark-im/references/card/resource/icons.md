# 图标枚举

用于 `header.icon`、`div.icon`、`markdown` 的 `<link icon=...>` 等。

## 结构

```json
// 系统图标（推荐）：用 token
{ "tag": "standard_icon", "token": "info_outlined", "color": "blue" }
// 自定义图标：用上传的 img_key
{ "tag": "custom_icon", "img_key": "img_v3_xxx" }
```

`color` 取颜色枚举（见 `colors.md`），仅对 `standard_icon` 生效。

## token 命名

- 线性：后缀 `_outlined`；面性（实心）：后缀 `_filled`。
- 主体 kebab-case，如 `calendar-add_outlined`、`delete-trash_outlined`。

## 常用 token（业务卡片）

| 含义 | token | 含义 | token |
|---|---|---|---|
| 完成/对勾 | `done_outlined` | 关闭/叉 | `close_outlined` |
| 新增 | `add_outlined` | 编辑 | `edit_outlined` |
| 删除 | `delete-trash_outlined` | 搜索 | `search_outlined` |
| 设置 | `setting_outlined` | 信息 | `info_outlined` |
| 警告 | `warning_outlined` | 时间 | `time_outlined` |
| 日历 | `calendar_outlined` | 成员 | `member_outlined` |
| 群组 | `group_outlined` | 会话 | `chat_outlined` |
| 邮件 | `mail_outlined` | 链接 | `link-copy_outlined` |
| 分享 | `share_outlined` | 下载 | `download_outlined` |
| 通知/铃铛 | `bell_outlined` | 定位 | `pin_outlined` |
| 附件 | `attachment_outlined` | 审批 | `approval_outlined` |

## 彩色图标（精确 token）

彩色图标必须从下表按**完整字符串**选择，禁止根据名称规律自行拼接。彩色 token 自带颜色，不要再推导其他后缀或变体。

| 含义 | token | 含义 | token |
|---|---|---|---|
| 日历 | `calendar_colorful` | 待办 | `todo_colorful` |
| 投票 | `vote_colorful` | 飞书妙记 | `file-lark-minutes_colorful` |
| 多维表格 | `wiki-bitable_colorful` | 表单 | `file-form_colorful` |
| 飞书社区 | `larkcommunity_colorful` | 招聘 | `hirelogo_colorful` |
| 飞书品牌 | `lark-logo_colorful` | Meego | `meego_colorful` |
| AI | `myai_colorful` | aPaaS | `apaas_colorful` |
| 审批 | `approval_colorful` | 通用 AI | `ai-common_colorful` |

> token 必须与官方完全一致，否则图标不渲染。上表为常用项，全量（数百个，分系统/商务/沟通/用户/媒体/文档等类目）以官方图标库为准：
> https://open.larkoffice.com/document/feishu-cards/enumerations-for-icons
