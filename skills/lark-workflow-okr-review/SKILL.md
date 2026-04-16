---
name: lark-workflow-okr-review
version: 1.0.0
description: "OKR 复盘工作流：汇总指定周期 OKR 进展，结合任务完成情况，生成结构化复盘报告。适用于 OKR 进展汇总、周期末复盘、团队 OKR 报告。"
metadata:
  requires:
    bins: ["lark-cli"]
---

# OKR 复盘报告工作流

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**。然后阅读 [`../lark-okr/SKILL.md`](../lark-okr/SKILL.md)，了解 OKR 相关操作。

## 适用场景

- "帮我整理这个季度的 OKR 复盘" / "查看我的 OKR 进展"
- "生成 OKR 进展汇总报告" / "OKR 复盘"
- "查看我的 OKR 和相关任务完成情况"
- "帮我写 OKR 复盘总结"

## 前置条件

仅支持 **user 身份**。执行前确保已授权：

```bash
lark-cli auth login --domain okr        # 基础（查看 OKR + 进展）
lark-cli auth login --domain okr,task   # 含关联任务（推荐）
```

## 工作流

```
{周期} ─► okr +periods ──► 获取周期 ID
               │
               ▼
          okr +list ──► OKR 列表（Objective + KR + 进度百分比）
               │
               ▼
          okr +get ──► 各 OKR 详情（含 progress_record_list）
               │
               ▼
          task +get-my-tasks ──► 关联任务完成情况（可选）
               │
               ▼
          AI 汇总 ──► 结构化复盘报告
               │
               ▼
          doc +create ──► 写入飞书文档（可选）
```

### Step 1: 确定周期

默认**当前周期**。如果用户指定了周期（如"Q1"、"上个季度"），需要先列出周期来匹配。

```bash
lark-cli okr +periods --format json
```

从返回结果中找到目标周期的 `id`。`[current]` 标记的是当前活跃周期。

### Step 2: 获取 OKR 列表

```bash
# 当前用户当前周期（默认）
lark-cli okr +list --format json

# 指定周期
lark-cli okr +list --period-id "period_xxx" --format json
```

输出包含：OKR ID、name、objective_list（含 content、progress_rate、kr_list）。

### Step 3: 获取 OKR 详情（可选，需要进展记录时）

```bash
lark-cli okr +get --okr-ids "okr_xxx" --format json
```

输出包含完整的 Objective 和 KR 结构，含 `progress_record_list`（进展记录引用列表）。

### Step 4: 获取关联任务（可选）

```bash
lark-cli task +get-my-tasks --format json
```

将任务与 OKR 的 KR 进行关联匹配，展示哪些任务支撑了哪些 KR。

### Step 5: AI 汇总生成报告

结合以上数据，生成结构化复盘报告：

```markdown
# OKR 复盘 — {周期名称}

## 概览
- 目标数：N 个
- 整体进度：XX%

## 各目标详情

### O1: {目标内容} — {进度}%
- KR1: {内容} — {进度}% ({状态})
- KR2: {内容} — {进度}% ({状态})

### O2: ...

## 关联任务完成情况（可选）
- 已完成：N 个
- 进行中：N 个

## 总结与反思
{AI 基于数据生成的分析}
```

### Step 6: 写入飞书文档（可选）

```bash
lark-cli docs +create --title "OKR 复盘 — Q1 2026" --markdown "<report_content>"
```

## 注意事项

1. **OKR 不可通过 API 修改**：如果用户要求修改 OKR 内容（如调整 Objective 描述、修改 KR），需引导用户在飞书 UI 中操作。
2. **进展记录可通过 API 创建**：如果复盘过程中用户想添加进展，可使用 `okr +progress-add`。
3. **时间戳转换**：API 返回的时间戳为毫秒级，展示时需转换为本地时间。
4. **权限**：查看他人 OKR 需要对方的 OKR 设置允许被查看。
