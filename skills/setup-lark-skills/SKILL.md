---
name: setup-lark-skills
version: 1.0.0
description: "配置飞书 skills 的激活状态：把低频 lark-* 转为 user-invoked 休眠以节省 system prompt，或恢复为自动注入。首次使用或需要调整时输入 /skill:setup-lark-skills。"
disable-model-invocation: true
metadata:
  role: setup
---

# Setup Lark Skills

默认状态：27 个 lark-* skill 全部自动注入（model-invoked），每条 description 常驻 system prompt（实测 27 条合计约 11.8KB，单条平均约 440B）。本 skill 的作用是把**低频**的 lark-* 转为 user-invoked 休眠——休眠后 description 不再注入 system prompt，需要时输入 `/skill:lark-<name>` 临时加载（本次会话有效）；也可以随时删除该行恢复激活。

`disable-model-invocation` 是 Agent Skills 标准的可选 frontmatter 字段；不识别它的 harness 会忽略该行（无害，skill 保持激活）。

## 配置流程

### 1. 定位已安装的 skill 文件

操作对象是用户机器上**已安装**的 skill 文件，安装路径因 harness 而异，先探测：

```bash
# 常见 harness 的 skill 安装目录
for d in ~/.claude/skills ~/.agents/skills ~/.codex/skills ~/.cursor/skills; do
  [ -f "$d/lark-im/SKILL.md" ] && echo "$d"
done
# 以上都找不到时兜底（范围较大，可能较慢）
find ~ -maxdepth 5 -path '*skills/lark-im/SKILL.md' 2>/dev/null | head -5
```

下文统一用 `$ROOT` 指代探测到的安装根目录。

### 2. 读取真实状态

唯一事实源是各 SKILL.md 的 frontmatter，**不是** `.active-profile`（后者仅是记录）：

```bash
for f in "$ROOT"/lark-*/SKILL.md; do
  n=$(basename $(dirname "$f"))
  if grep -q '^disable-model-invocation: true' "$f"; then echo "$n: 休眠"; else echo "$n: 激活"; fi
done
```

### 3. 展示能力菜单

向用户展示以下分组，让用户勾选**日常高频、需要保持自动注入**的；未勾选的低频 skill 全部转为休眠：

**📋 核心办公（推荐保持激活）**
- [ ] lark-im — 消息/群聊/文件传输
- [ ] lark-doc — 云文档读写
- [ ] lark-calendar — 日程/会议室
- [ ] lark-task — 任务/待办

**📊 数据与文档**
- [ ] lark-sheets — 电子表格
- [ ] lark-base — 多维表格
- [ ] lark-drive — 云盘文件管理
- [ ] lark-wiki — 知识库
- [ ] lark-markdown — Markdown 文件
- [ ] lark-slides — 幻灯片
- [ ] lark-whiteboard — 画板

**🤝 协作与沟通**
- [ ] lark-approval — 审批
- [ ] lark-mail — 邮件
- [ ] lark-contact — 通讯录/查人
- [ ] lark-vc — 已结束会议记录查询
- [ ] lark-minutes — 妙记/音视频转写
- [ ] lark-vc-agent — 会中实时能力
- [ ] lark-note — 已知 note_id 纪要直查

**🏢 组织与效率**
- [ ] lark-okr — OKR 管理
- [ ] lark-attendance — 考勤打卡
- [ ] lark-apps — 妙搭应用开发

**🔧 开发者工具**
- [ ] lark-event — 实时事件监听
- [ ] lark-shared — 认证/权限管理
- [ ] lark-skill-maker — 创建新 skill
- [ ] lark-openapi-explorer — 探索原生 API

**🔄 工作流**
- [ ] lark-workflow-meeting-summary — 会议纪要汇总报告
- [ ] lark-workflow-standup-report — 日程待办摘要

### 4. 确认选择

展示用户的选择（保持激活的集合 + 将转休眠的集合），确认后再执行修改。

### 5. 执行休眠/激活（幂等）

```bash
# 休眠（幂等：不存在才插入；插在 name: 行之后，避开多行 description 陷阱）
grep -q '^disable-model-invocation: true' "$ROOT/lark-<name>/SKILL.md" || \
sed -i '' '/^name:/a\
disable-model-invocation: true' "$ROOT/lark-<name>/SKILL.md"

# 激活（幂等：删除所有匹配行，跑多次无副作用）
sed -i '' '/^disable-model-invocation: true$/d' "$ROOT/lark-<name>/SKILL.md"
```

（以上 `sed -i ''` 是 macOS 语法；Linux 下改为 `sed -i`，去掉 `''`。）

### 6. 写入配置记录

将选择结果写入 `$ROOT/lark/.active-profile`，格式：

```
# Lark Skills Active Profile
# Updated: <timestamp>
lark-im
lark-doc
```

下次运行时以第 2 步的扫描结果为准，`.active-profile` 仅供人类参考。

## 预设方案

预设含义 = **保持激活**的集合，其余低频 skill 转为休眠：

| 预设 | 保持激活 |
|------|------|
| `minimal` | lark-im, lark-doc, lark-calendar |
| `office` | minimal + lark-task, lark-approval, lark-drive, lark-contact |
| `full` | 全部保持激活（默认状态，27 条 description 常驻，约 11.8KB） |
| `dev` | lark-shared, lark-event, lark-skill-maker, lark-openapi-explorer, lark-apps |

## 注意事项

- 修改后需**重启当前 agent 会话**才能生效（skill 列表一般在会话启动时加载）。
- 任何时候都可以用 `/skill:lark-<name>` 临时加载休眠的 skill（本次会话有效）。
- `lark`（路由）与本 skill 在仓库中即带 `disable-model-invocation: true`，永远保持 user-invoked，不占 system prompt；配置时不要改动这两个文件。
