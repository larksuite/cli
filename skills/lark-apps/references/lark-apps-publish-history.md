# apps +publish-history

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

分页查询指定妙搭应用的发布历史，结果按最新发布排在最前。对应 `GET /open-apis/spark/v1/apps/:app_id/releases`，`--status` / `--limit` / `--page-token` 均映射为 HTTP query 参数。

## 命令

```bash
# 查询发布历史（服务端默认约 50 条）
lark-cli apps +publish-history --app-id app_xxx

# 表格视图
lark-cli apps +publish-history --app-id app_xxx --format table

# 翻页（用上一页响应的 next_page_token）
lark-cli apps +publish-history --app-id app_xxx --page-token eyJxxx

# 指定返回条数（仅在需要不同页大小时才传）
lark-cli apps +publish-history --app-id app_xxx --limit 10

# 只看失败的发布
lark-cli apps +publish-history --app-id app_xxx --status failed

# 预演（当前可用）
lark-cli apps +publish-history --app-id app_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID |
| `--limit <n>` | ❌ | 每页条数，范围 1–500；**不传则使用服务端默认（约 50 条）——通常应省略此参数，除非需要特定页大小或分页** |
| `--page-token <token>` | ❌ | 上一页响应的 `next_page_token`，用于翻页 |
| `--status <status>` | ❌ | 按发布状态过滤；可选值：`publishing` / `finished` / `failed` |

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "releases": [
      {
        "release_id": "release_yyy",
        "status": "finished",
        "created_at": 1748000000000,
        "updated_at": 1748000120000
      }
    ],
    "next_page_token": "eyJxxx",
    "has_more": false
  }
}
```

**`--format table` 视图：**

```
release_id    status    created_at        updated_at
release_yyy   finished  1748000000000     1748000120000
```

## 字段语义

| 字段 | 含义 |
|---|---|
| `releases[].release_id` | 发布ID，即 `+publish-status` / `+publish-error-log` 的 `--release-id` |
| `releases[].status` | 发布状态字符串：`publishing`（进行中）/ `finished`（成功）/ `failed`（失败）|
| `releases[].created_at` / `updated_at` | Unix 毫秒时间戳（不做时区格式化）|
| `next_page_token` | 下一页游标；`has_more=false` 时忽略 |
| `has_more` | 是否还有更多页 |

## 典型场景

### 场景 1：查看最近发布记录

```bash
lark-cli apps +publish-history --app-id app_xxx --format table
```

返回后，找到目标发布的 `release_id`，再用 `+publish-status` 查详情。

### 场景 2：翻页查询

```bash
# 第一页
lark-cli apps +publish-history --app-id app_xxx
# 第二页（用上一页的 next_page_token）
lark-cli apps +publish-history --app-id app_xxx --page-token eyJxxx
```

### 场景 3：只看失败的发布

```bash
lark-cli apps +publish-history --app-id app_xxx --status failed --format table
```

找到目标 `release_id` 后，用 `+publish-error-log` 查失败根因。

## 协同命令

| 场景 | 命令 |
|---|---|
| 触发新发布 | `apps +publish` |
| 查单个发布状态 | `apps +publish-status` |
| 查发布错误日志 | `apps +publish-error-log` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
