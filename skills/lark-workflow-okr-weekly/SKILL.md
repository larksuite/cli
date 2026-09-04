---
name: lark-workflow-okr-weekly
version: 1.0.0
description: "OKR 周报工作流：拉取当前生效的 OKR 周期下的目标与关键结果，汇总本周已完成任务和下周待办，把任务对齐到 KR，生成一份结构化的 OKR 周报。当用户说“写一下本周 OKR 周报”“把我这周的任务对齐到 OKR”“生成 OKR 进度总结”时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
---

# OKR 周报工作流

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**。然后阅读 [`../lark-okr/SKILL.md`](../lark-okr/SKILL.md)、[`../lark-okr/references/lark-okr-entities.md`](../lark-okr/references/lark-okr-entities.md)、[`../lark-task/SKILL.md`](../lark-task/SKILL.md)，了解 OKR 实体结构和任务查询。

## 适用场景

- "帮我写一份本周 OKR 周报" / "生成 OKR 进度总结"
- "把我这周的任务对齐到 OKR 上" / "哪些任务推进了我的 KR"
- "这周在 OKR 上做了啥" / "下周 OKR 要推进什么"

## 前置条件

仅支持 **user 身份**。执行前确保已授权：

```bash
lark-cli auth login --domain okr,task            # 基础（读 OKR + 任务）
lark-cli auth login --domain okr,task,docs,drive # 含写出周报为飞书文档（可选）
lark-cli auth login --domain okr,task,im         # 含发送到群（可选）
```

**租户必须启用 OKR 产品**，否则 `okr +cycle-list` 会返回空或无权限错误。

## 工作流

```
当前用户 ─► contact +get-user ──► open_id
              │
              ▼
         okr +cycle-list ──► 周期列表（挑选"当前生效"且覆盖本周的那一个）
              │
              ▼
         okr +cycle-detail ──► 本周期的全部 Objectives + Key Results
              │
              ├─► task +get-my-tasks --complete=true --created_at -7d  → 本周完成任务
              ├─► task +get-my-tasks --complete=false --due-end +7d    → 下周待办
              │
              ▼
         AI 对齐：任务 ↔ KR（基于标题关键词 + 用户确认）
              │
              ▼
         OKR 周报（用户确认）
              │
              ├─► 可选 docs +create  落盘为飞书文档
              └─► 可选 im +messages-send  发送到群
```

### Step 1: 获取当前用户的 open_id

```bash
lark-cli contact +get-user
```

- 从返回中取 `open_id`，作为后续 `okr +cycle-list --user-id` 的入参
- 如果返回 `41050` 类错误，提示用户联系管理员开通组织架构可见性（详见 [`../lark-contact/references/lark-contact-get-user.md`](../lark-contact/references/lark-contact-get-user.md)）

### Step 2: 定位当前生效的 OKR 周期

```bash
lark-cli okr +cycle-list --user-id "ou_xxx"
```

- 结果按 `start_time` / `end_time` 遍历，**挑选同时满足以下条件的周期**：
  1. `cycle_status` 为 `normal`（或 `default`）——见 [`../lark-okr/references/lark-okr-cycle-list.md`](../lark-okr/references/lark-okr-cycle-list.md)
  2. `start_time ≤ 本周一` 且 `end_time ≥ 本周日`（覆盖本周）
- 若同时符合的周期有多个（如季度周期 + 年度周期并行），**必须把候选展示给用户让其选一个**；默认优先选择**覆盖时间最短的那个**（季度周期通常比年度周期更贴近周级颗粒度）
- 若没有匹配周期，诚实告知用户"当前没有处于生效状态的 OKR 周期覆盖本周"，不要硬选一个

> **日期计算用系统命令（`date`）算**，不要心算"本周一/本周日"。

### Step 3: 拉取周期下的 O/KR

```bash
lark-cli okr +cycle-detail --cycle-id "<cycle_id>"
```

- 返回中每个 Objective 的 `content`、每个 KeyResult 的 `content` 都是 **ContentBlock 富文本 JSON 字符串**
- **解析方式**：按 [`../lark-okr/references/lark-okr-contentblock.md`](../lark-okr/references/lark-okr-contentblock.md) 把 ContentBlock 扁平化为**纯文本**（拼接 `text_run.text` 即可），用于后续对齐；富文本样式在周报里无需保留
- 记录每个 KR 的：`id`、扁平化文本、`score`（0.0–1.0）、`weight`、`deadline`、`owner`
- 记录每个 Objective 的：`id`、扁平化文本、`score`、`weight`、`deadline`

### Step 4: 拉取本周完成与下周待办

```bash
# 本周完成的任务（created_at 用作"在本周内活动过"的近似；与用户确认后可改为按 updated_at 过滤）
lark-cli task +get-my-tasks --complete=true --created_at "-7d" --format json

# 下周待办（近 7 天内到期的未完成任务）
lark-cli task +get-my-tasks --complete=false --due-end "+7d" --format json
```

- 详见 [`../lark-task/references/lark-task-get-my-tasks.md`](../lark-task/references/lark-task-get-my-tasks.md)
- `--created_at "-7d"` 只是"本周内创建过/活动过"的代理，**如果用户需要严格"本周完成"**，需要配合 `--complete=true` 并在 AI 层用 `completed_time` 字段二次过滤（若返回中有该字段）
- 对超过 30 条的结果，在周报里按"Top 10 + 其余 N 条"折叠展示，避免输出过长

### Step 5: 对齐任务到 KR（本工作流的核心）

这一步是 skill 的真正价值，**不要跳过也不要静默胡编**：

1. **关键词匹配**：对每个任务的 `summary`，逐一与每个 KR 的扁平化文本做关键词/语义匹配
2. **多对一**：同一个任务**最多归到 1 个 KR**；如果任务明显服务多个 KR，主归一个，其它在备注说明
3. **未对齐桶**：与所有 KR 都不匹配的任务放入「未对齐（杂项）」分组——**这是一个诚实的信号，不要强行对齐**
4. **置信度**：对每条任务-KR 的归属，心里给一个置信度；**低置信度的条目必须向用户展示并让用户确认或改派**，不要静默归类
5. **覆盖度指标**：计算 `已对齐任务数 / 总任务数`，写进周报"总结"段

### Step 6: 生成周报

按以下模板生成并展示给用户**确认**；任何数据缺失（如某 KR 本周没有任何任务）必须**显式标注**，不能省略。

```markdown
# OKR 周报：{姓名} · {本周一 YYYY-MM-DD} ~ {本周日 YYYY-MM-DD}

> **OKR 周期**：{周期名称，如"2026 年 4-6 月"}（`cycle_id: xxx`）
> **对齐覆盖率**：{M} / {N} 任务已对齐到 KR

---

## O1: {Objective 文本}（score: {0.75}, weight: {1.0}）

### KR1.1: {KR 文本}（score: {0.8}, weight: {0.5}, deadline: {YYYY-MM-DD}）

**本周进展**：
- ✅ [{task_summary}]({task_url})（完成于 {date}）
- ✅ ...

**下周计划**：
- [ ] [{task_summary}]({task_url})（截止 {date}）
- [ ] ...

**本周无相关任务**：{若为空则显式写这句，不要省略整个 KR}

### KR1.2: ...

## O2: ...

## 未对齐（杂项）

**本周完成但未归到 KR：**
- ✅ {task_summary} —— {link}（原因：{AI 判断的不匹配原因}）

**下周待办但未归到 KR：**
- [ ] ...

## 总结

- 本周共完成 {X} 项任务，其中 {M} 项推进了 OKR
- 进度领先的 KR：{列 1–2 个 score 增长的 KR，若无法从当前数据判断增长则省略本条}
- 进度落后的 KR：{deadline 临近 + score 低的 KR}
- 下周重点：{基于"下周计划"推断出的 2–3 条}
```

### Step 7: 落盘为飞书文档（可选，用户明确要求时）

阅读 [`../lark-doc/SKILL.md`](../lark-doc/SKILL.md) 了解云文档命令。

```bash
lark-cli docs +create \
  --title "OKR 周报 - {姓名} - {本周一}" \
  --markdown "<Step 6 生成的 Markdown>"
```

**写入前必须确认用户意图。**

### Step 8: 发送到群（可选，用户明确要求时）

阅读 [`../lark-im/SKILL.md`](../lark-im/SKILL.md) 了解 IM 命令。

```bash
lark-cli im +messages-send --chat-id "<chat_id>" --markdown $'**OKR 周报已生成**\n\n[查看文档]({doc_url})'
```

**发送消息前必须确认用户意图。**

## 权限表

| 命令 | 所需 scope |
|------|-----------|
| `contact +get-user` | 详见 [`../lark-contact/SKILL.md`](../lark-contact/SKILL.md) |
| `okr +cycle-list` | `okr:okr.period:readonly` |
| `okr +cycle-detail` | `okr:okr.content:readonly` |
| `task +get-my-tasks` | `task:task:read` |
| `docs +create` | 详见 [`../lark-doc/references/lark-doc-create.md`](../lark-doc/references/lark-doc-create.md) |
| `im +messages-send` | 详见 [`../lark-im/references/lark-im-messages-send.md`](../lark-im/references/lark-im-messages-send.md) |

## 安全与范围

- 本工作流默认只读；Step 7、Step 8 是写操作，**执行前必须用户明确确认**
- OKR 是敏感业务数据，**不要把 `open_id`、`cycle_id` 等内部标识暴露到周报正文**——只在"元信息"注释里保留一个即可
- 任务-KR 对齐是 AI 推断，**低置信条目必须由用户复核**，不要让周报显得比实际更"整齐"
- 不要在日期推断上心算；本周起止、任务截止、周期起止都用 `date` 命令算
- 若租户未启用 OKR 产品，直接告知用户并提供手工填写的模板，不要反复重试 API

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证、权限（必读）
- [lark-okr](../lark-okr/SKILL.md) — OKR 全部命令
- [lark-okr-entities](../lark-okr/references/lark-okr-entities.md) — OKR 实体结构（必读）
- [lark-okr-contentblock](../lark-okr/references/lark-okr-contentblock.md) — ContentBlock 富文本解析
- [lark-task](../lark-task/SKILL.md) — `+get-my-tasks` 详细用法
- [lark-contact](../lark-contact/SKILL.md) — `+get-user` 详细用法
- [lark-doc](../lark-doc/SKILL.md) — `+create` 详细用法
- [lark-im](../lark-im/SKILL.md) — `+messages-send` 详细用法
