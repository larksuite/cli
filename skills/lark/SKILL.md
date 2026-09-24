---
name: lark
version: 1.0.0
description: "飞书/Lark 能力路由器：27 个 lark-* skill 的权威用途索引与调用方式。当不确定某个飞书需求该用哪个 skill、或一个任务需要组合多个飞书 skill 时，输入 /skill:lark 加载本文件查询。"
disable-model-invocation: true
metadata:
  role: router
---

# Lark 能力路由

本文件是 27 个 lark-* skill 的唯一权威路由索引（description 不复述触发词，避免双写漂移）。27 个 skill 默认全部自动注入（model-invoked）；想把飞书 skill 与日常开发隔离的用户，可用 `setup-lark-skills` 将低频 skill 转为 user-invoked 休眠。

## 调用方式

- **查路由**：输入 `/skill:lark` 加载本文件，按下表选择。
- **直接用**：已知目标时输入 `/skill:lark-<name>`，无需经过本文件。
- **Agent 备用路径**：agent 在用户明确要求飞书能力、但相关 skill 未加载时，可直接 read 本文件获取路由，再 read 目标 skill 的 SKILL.md 执行（安装路径因 harness 而异，探测方法见 `setup-lark-skills`）。
- **隔离配置**：运行 `/skill:setup-lark-skills` 把低频 skill 转为 user-invoked 休眠、或恢复自动注入（重启会话后生效）。

**多意图任务**按链路依次加载多个 skill。例：「把妙记整理成文档发到群里」→ `lark-minutes`（取内容）→ `lark-doc`（成文）→ `lark-im`（发送）。

## 路由表

| 用户意图 | Skill | 命令 | 区分 |
|---------|-------|------|------|
| 发消息/聊天记录/群聊/文件图片传输/加急/交互卡片 | lark-im | `/skill:lark-im` | 按姓名发消息先经 lark-contact 解析 open_id |
| 云文档正文读写（Docx/Wiki 文档、doubao.com /docx/ 链接） | lark-doc | `/skill:lark-doc` | 知识空间与节点管理走 lark-wiki；文档内嵌的表格/Base/画板先取 token 再切对应 skill |
| 电子表格 | lark-sheets | `/skill:lark-sheets` | 多维表格走 lark-base |
| 多维表格/Base/bitable、/base/ 链接 | lark-base | `/skill:lark-base` | 文件导入走 lark-drive；认证授权走 lark-shared |
| 日历/日程/会议室/忙闲查询 | lark-calendar | `/skill:lark-calendar` | 会议类记录见下方「会议相关怎么选」；待办走 lark-task |
| 任务/待办/清单 | lark-task | `/skill:lark-task` | 审批待办走 lark-approval |
| 审批（待办/已办/实例/发起原生审批） | lark-approval | `/skill:lark-approval` | 非审批类待办走 lark-task |
| 云盘/文件文件夹管理/上传下载/本地文件导入/链接 token 判断 | lark-drive | `/skill:lark-drive` | .md 文件内容编辑走 lark-markdown |
| 邮件 | lark-mail | `/skill:lark-mail` | 仅邮件意图 |
| 已结束会议：搜索历史会议/纪要（总结/待办/章节/逐字稿）/参会人快照 | lark-vc | `/skill:lark-vc` | 见下方「会议相关怎么选」 |
| 妙记：搜索/产物读写/音视频上传下载/本地音视频转纪要逐字稿 | lark-minutes | `/skill:lark-minutes` | 见下方「会议相关怎么选」 |
| 已知 note_id 直查纪要详情/unified 逐字记录 | lark-note | `/skill:lark-note` | 见下方「会议相关怎么选」 |
| 会中实时：机器人入会/离会、会中事件、谁在发言 | lark-vc-agent | `/skill:lark-vc-agent` | 见下方「会议相关怎么选」 |
| OKR 周期/目标/KR/对齐关系 | lark-okr | `/skill:lark-okr` | 待办走 lark-task；日程走 lark-calendar |
| 知识库：知识空间/空间成员/节点层级管理、doubao.com /wiki/ 链接 | lark-wiki | `/skill:lark-wiki` | wiki 文档正文编辑走 lark-doc |
| 幻灯片创建/读取/页面编辑、doubao.com /slides/ 链接 | lark-slides | `/skill:lark-slides` | — |
| 画板 | lark-whiteboard | `/skill:lark-whiteboard` | — |
| Markdown 文件（.md 查看/创建/编辑/patch/diff） | lark-markdown | `/skill:lark-markdown` | 转在线文档走 lark-drive 导入 + lark-doc；云空间管理走 lark-drive |
| 通讯录：姓名/邮箱 ↔ open_id、查人信息 | lark-contact | `/skill:lark-contact` | 部门树/组织架构遍历走 lark-openapi-explorer |
| 妙搭应用开发/托管/监控/触发器 | lark-apps | `/skill:lark-apps` | — |
| 考勤打卡记录查询 | lark-attendance | `/skill:lark-attendance` | — |
| 实时事件监听（NDJSON 流） | lark-event | `/skill:lark-event` | 一次性查询走对应业务 skill |
| 认证/登录/身份/权限 scope 问题 | lark-shared | `/skill:lark-shared` | 任何 skill 报权限错误时也先查这里 |
| 把飞书 API 封装成新 skill | lark-skill-maker | `/skill:lark-skill-maker` | — |
| 现有 skill/CLI 不满足时探索原生 OpenAPI | lark-openapi-explorer | `/skill:lark-openapi-explorer` | 兜底入口 |
| 跨会议汇总：时间范围内会议纪要 → 结构化报告/会议周报 | lark-workflow-meeting-summary | `/skill:lark-workflow-meeting-summary` | 见下方「会议相关怎么选」 |
| 日程+待办摘要（今天/明天/本周安排） | lark-workflow-standup-report | `/skill:lark-workflow-standup-report` | 单日日程详情走 lark-calendar |

## 会议相关怎么选（高频歧义区，会议类技能区分的唯一权威出处）

- **进行中的会议**实时情况 → `lark-vc-agent`
- **已结束会议**的搜索与纪要 → `lark-vc`
- **妙记产物**（逐字稿等）或**本地音视频**转写 → `lark-minutes`
- **已知 note_id** → `lark-note`
- **一段时间内所有会议**的汇总报告 → `lark-workflow-meeting-summary`

## 非飞书需求

与飞书无关的通用编码、互联网搜索等需求不使用本路由及任何 lark-* skill。
