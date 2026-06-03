# apps +publish-error-log

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

获取指定发布的错误日志，列出失败步骤及其错误原因。通常在 `apps +publish-status` 返回 `status=failed` 后调用，定位失败根因。对应 `GET /open-apis/spark/v1/apps/:app_id/releases/:release_id/error_logs`。

> **⚠️ 注意：** 这里的「发布ID / release_id」是妙搭**发布** ID（`apps +publish` 返回的 `release_id`），**不是飞书审批实例号**。查发布进度用 `apps +publish-status`、查失败原因用 `apps +publish-error-log`；不要路由到 lark-approval / 审批相关命令。

## 命令

```bash
# 查询发布错误日志
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy

# 表格视图
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy --format table

# 预演（当前可用）
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 应用 ID |
| `--release-id <id>` | ✅ | 发布ID（即 `apps +publish` 响应的 `release_id`）|

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "status": "failed",
    "error_logs": [
      {
        "step": "frontend-build",
        "error_log": "dependency conflict: react@18 vs react@17"
      }
    ]
  }
}
```

**`--format table` 视图：**

```
status: Failed

step             error_log
frontend-build   dependency conflict: react@18 vs react@17
```

**Validate 失败：**

```json
{
  "ok": false,
  "error": { "type": "validation", "message": "--release-id is required" }
}
```

## 字段语义

| 字段 | 含义 |
|---|---|
| `status` | 发布状态字符串：`publishing`（进行中）/ `finished`（成功）/ `failed`（失败）|
| `error_logs` | 失败步骤列表；若无失败记录则为空数组 `[]` |
| `error_logs[].step` | 失败步骤名 |
| `error_logs[].error_log` | 错误日志，**转述给用户帮助定位问题** |

## 典型场景

### 场景 1：发布失败后查根因

```bash
# 先确认状态
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy
# status=failed 时查错误日志
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy --format table
```

把 `error_logs[].error_log` 转述给用户：

> 发布失败，步骤 `{step}` 报错：{error_log}

### 场景 2：error_logs 为空但 status=failed

说明失败发生在步骤记录之前（如参数校验阶段），转述 `status` 给用户并建议检查发布配置。

## 协同命令

| 场景 | 命令 |
|---|---|
| 触发新发布 | `apps +publish` |
| 查单个发布状态 | `apps +publish-status` |
| 查发布历史 | `apps +publish-history` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
