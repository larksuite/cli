---
name: lark-okr
version: 1.0.0
description: "飞书 OKR：查看和管理 OKR 目标与关键结果。查看当前周期 OKR、更新进展记录、查询 OKR 复盘。当用户需要查看 OKR 进度、添加进展、查看团队 OKR 或进行 OKR 复盘时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli okr --help"
---

# okr (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

> **身份要求**：OKR API 仅支持 **user 身份** (user_access_token)。不支持 bot 身份。
> **重要限制**：OKR、Objective、Key Result 的**创建与修改只能在飞书 UI 中完成**，API 仅支持读取和进展管理。
> **周期识别**：当用户未指定周期时，`+list` 会自动查询当前活跃周期。如需指定周期，先用 `+periods` 获取可用周期列表。
> **用户身份**：当用户提到"我的 OKR"时，无需额外参数，`+list` 默认使用当前登录用户。查看他人 OKR 时需提供 `--user-id`。
> **ID 说明**：OKR API 中的 ID（okr_id、objective_id、kr_id）均为 OKR 系统内部 ID，不是客户端展示的编号。

> **进展记录注意**：
> 1. 创建进展记录需提供 `target_id`（Objective 或 KR 的 ID）和 `--target-type`（`objective` 或 `key_result`）。
> 2. `--text` 提供纯文本输入，会自动转换为飞书富文本格式；`--content` 支持完整的富文本 JSON。
> 3. 进展记录需要 `source_url`（必须是 http/https 开头的 URL），默认使用 lark-cli 的 GitHub 地址。

> **查询注意**：
> 1. OKR 列表接口最多每页返回 10 条（`--limit` 最大为 10）。
> 2. `batch_get` 接口最多同时查询 10 个 OKR ID。
> 3. 复盘查询最多 5 个 user_id 和 5 个 period_id。
> 4. 所有时间戳为**毫秒**级 Unix 时间。

## Shortcuts

- [`+list`](./references/lark-okr-list.md) — 查看用户 OKR（默认当前用户 + 当前周期）
- [`+get`](./references/lark-okr-get.md) — 按 ID 批量获取 OKR 详情
- [`+periods`](./references/lark-okr-periods.md) — 列出 OKR 周期
- [`+progress-add`](./references/lark-okr-progress-add.md) — 添加进展记录
- [`+progress-get`](./references/lark-okr-progress-get.md) — 按 ID 获取进展记录详情
- [`+review`](./references/lark-okr-review.md) — 查询 OKR 复盘信息

## API Resources

```bash
lark-cli schema okr.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli okr <resource> <method> [flags]  # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### okrs

  - `batch_get` — 批量获取 OKR

### users (okr)

  - `okrs` — 获取用户 OKR 列表

### periods

  - `list` — 获取 OKR 周期列表
  - `create` — 创建 OKR 周期
  - `patch` — 更新 OKR 周期状态

### period_rules

  - `list` — 获取 OKR 周期规则列表

### progress_records

  - `create` — 创建进展记录
  - `get` — 获取进展记录详情
  - `update` — 更新进展记录
  - `delete` — 删除进展记录

### images

  - `upload` — 上传进展图片

### reviews

  - `query` — 查询 OKR 复盘信息

### metric_sources

  - `list` — 获取指标源列表
  - `tables` — 获取指标表列表
  - `items` — 获取/更新指标项

## 权限表

| Scope | 说明 |
|-------|------|
| `okr:okr:readonly` | 读取 OKR 信息 |
| `okr:okr` | 读取 + 更新 OKR 信息 |
| `okr:okr.progress:readonly` | 读取进展记录 |
| `okr:okr.progress:writeonly` | 创建/更新进展记录 |
| `okr:okr.progress:delete` | 删除进展记录 |
| `okr:okr.period:readonly` | 读取周期信息 |
| `okr:okr.review:readonly` | 读取复盘信息 |
