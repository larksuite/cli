---
name: lark-workflow-meeting-summary
version: 1.0.0
description: "会议纪要整理工作流：汇总指定时间范围内的会议纪要并生成结构化报告。当用户需要整理会议纪要、生成会议周报、回顾一段时间内的会议内容时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
    skills: ["lark-meeting"]
---

# 会议纪要汇总工作流

**CRITICAL — 开始前 MUST 先完整读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 和 [`../lark-meeting/SKILL.md`](../lark-meeting/SKILL.md)**。认证、身份和权限以 lark-shared 为准；会议与产物关系、产物选择和逐字稿路由以 lark-meeting 为准。

## 适用场景

- "帮我整理这周的会议纪要" / "总结最近的会议" / "生成会议周报"
- "看看今天开了哪些会" / "回顾过去一周开了哪些会"

## 前置条件

仅支持 **user 身份**。执行前确保已授权：

```bash
lark-cli auth login --domain vc        # 基础（查询+纪要）
lark-cli auth login --domain vc,drive   # 含读取纪要文档正文、生成文档
lark-cli auth login --domain vc,drive,minutes  # 含无 note_id 时的妙记备选路径
```

## 工作流

```
{时间范围} ─► vc +search ──► 会议列表 (meeting_ids)
                   │
                   ▼
               vc +detail ──► 获取 note_id 
                   │
                   ▼
               note +detail ──► 纪要文档 tokens
                   │
                   ▼
               drive metas batch_query 纪要元数据
                   │
                   ▼
               结构化报告
```

### Step 1: 确定时间范围

默认**过去 7 天**。推断规则："今天"→当天，"这周"→本周一~now，"上周"→上周一~上周日，"这个月"→1日~now。

> **注意**：日期转换必须调用系统命令（如 `date`），不要心算。时间范围参数需根据 CLI 实际要求格式化（通常为 `YYYY-MM-DD` 或 ISO 8601）。

### Step 2: 查询会议记录

```bash
# page-size 最大为 30
lark-cli vc +search --start "<YYYY-MM-DD>" --end "<YYYY-MM-DD>" --format json --page-size 30
```

- 时间范围拆分：搜索的时间范围最大为 1 个月。搜索更长时间范围的会议，需要拆分为多次时间范围为一个月查询。
- `--end` 为**包含当天**的日期（即查"今天"时 start 和 end 都填今天）
- `--format json` 输出 JSON 格式，你更佳擅长解析 JSON 数据。
- `--page-size 30` 每页最多 30 条。
- 有 `page_token` 时必须继续翻页，收集所有 `id` 字段（meeting-id）

### Step 3: 获取纪要元数据

1. 查询会议关联的纪要信息
```bash
# 首先获取 note_id 和 minute_token
lark-cli vc +detail --meeting-ids "id1,id2,...,idN"

# 然后用 note_id 获取文档 tokens（如有多个需分别获取）
lark-cli note +detail --note-id "note_id"
```
- 根据上一步搜集到的 `meeting-id` 查询。
- 单次最多查询 50 个，超过 50 个需分批调用。
- 部分会议没有 `note_id` 或报错 `no notes available`，**不要直接标注"无纪要"**：先看 `vc +detail` 是否返回了 `minute_token`，有则走下面的妙记备选路径；`note_id` 和 `minute_token` 都没有时才标注"无纪要"。
- 记录每个纪要的 `note_id`（纪要 ID）、`note_display_type`（展示类型：`unknown` / `normal` / `unified`）、`note_doc_token`（纪要文档 Token）和 `verbatim_doc_token`（逐字稿文档 Token）。

> **妙记备选路径（无 `note_id`、有 `minute_token` 时）**：智能纪要与妙记是两条独立产物链路，缺少智能纪要不代表这场会没有内容。
>
> ```bash
> # --minute-tokens 是复数形式（+download 同）；--output-dir 只接受相对路径
> lark-cli minutes +detail --minute-tokens "<minute_token>" --transcript --output-dir ./transcripts --as user
> ```
>
> 逐字稿会落盘，供 Step 4 基于原始发言独立提炼（不要照搬 AI 总结）。若返回 `No read permission`（`2091005`），先把无权限事实告知用户，用户明确同意后再用单数 flag 申请：`lark-cli minutes +apply-permission --minute-token "<minute_token>" --perm view --as user`；申请需 owner 在客户端批准后才可重试。详见 [基于 minute_token 查询妙记及关联产物](../lark-meeting/scenes/query-minutes-and-artifacts.md)。

> **逐字稿路由按 `note_display_type` 决定**（详见 [基于 note_id 查询智能纪要及关联产物](../lark-meeting/scenes/query-note-and-artifacts.md)）：
> - `normal`：逐字稿是独立文档，链接/正文走 `verbatim_doc_token`。
> - `unified`：逐字稿**不是独立文档**，没有可分享的逐字稿文档链接；需要逐字稿内容时用 `note +transcript --note-id <note_id>`（[lark-meeting](../lark-meeting/SKILL.md)）拉取到本地，报告中标注"unified 纪要"即可。

2. 获取纪要文档和逐字稿文档链接
```bash
# 学习命令使用方式
lark-cli schema drive.metas.batch_query

# 批量获取纪要文档与逐字稿链接: 一次最多查询 10 个文档
# 仅对 note_doc_token 与 normal 纪要的 verbatim_doc_token 查询链接
lark-cli drive metas batch_query --data '{"request_docs": [{"doc_type": "docx", "doc_token": "<doc_token>"}], "with_url": true}'
```

### Step 4: 整理纪要报告

根据时间跨度选择输出格式：

- **单日汇总**（"今天"/"昨天"）：用"今日会议概览"标题，逐会议列出会议时间、主题、纪要链接、逐字稿链接（`unified` 纪要无逐字稿链接，标注"unified 纪要，逐字稿需 `note +transcript` 拉取"）。
- **多日/周报**（"这周"/"过去 7 天"等）：用"会议纪要周报"标题，含概览统计、逐会议详情。

### Step 5: 生成文档（可选，用户要求时）

阅读 [`../lark-doc/SKILL.md`](../lark-doc/SKILL.md) 学习云文档技能。

```bash
lark-cli docs +create --doc-format markdown --content $'<title>会议纪要汇总 (<start> - <end>)</title>\n<内容>'
# 或追加到已有文档
lark-cli docs +update --doc "<url_or_token>" --command append --doc-format markdown --content $'<内容>'
```

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证、权限（必读）
- [lark-meeting](../lark-meeting/SKILL.md) — 会议与产物统一路由
- [查询会议与会议产物](../lark-meeting/scenes/query-meeting-and-artifacts.md) — 搜索、消歧、产物获取与逐字稿分析流程
- [查询妙记及关联产物](../lark-meeting/scenes/query-minutes-and-artifacts.md) — 无 `note_id` 时的妙记备选路径
- [`vc +search`](../lark-meeting/references/lark-vc-search.md)、[`vc +detail`](../lark-meeting/references/lark-vc-detail.md)、[`note +detail`](../lark-meeting/references/lark-note-detail.md)、[`note +transcript`](../lark-meeting/references/lark-note-transcript.md)、[`minutes +detail`](../lark-meeting/references/lark-minutes-detail.md)、[`minutes +apply-permission`](../lark-meeting/references/lark-minutes-apply-permission.md) — 命令细节
- [lark-doc](../lark-doc/SKILL.md) — `+fetch`、`+create`、`+update` 详细用法
