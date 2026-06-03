# apps +publish-status

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查询指定发布的状态和详情。通常在触发 `apps +publish` 后，用返回的 `release_id` 轮询进度。对应 `GET /open-apis/spark/v1/apps/:app_id/releases/:release_id`。

> **⚠️ 注意：** 这里的「发布ID / release_id」是妙搭**发布** ID（`apps +publish` 返回的 `release_id`），**不是飞书审批实例号**。查发布进度用 `apps +publish-status`、查失败原因用 `apps +publish-error-log`；不要路由到 lark-approval / 审批相关命令。

## 命令

```bash
# 查询发布状态
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy

# 预演（当前可用）
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy --dry-run
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
    "release": {
      "release_id": "release_yyy",
      "status": "finished",
      "created_at": 1748000000000,
      "updated_at": 1748000120000
    }
  }
}
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
| `release.status` | 发布状态字符串：`publishing`（进行中）/ `finished`（成功）/ `failed`（失败）|
| `release.created_at` / `updated_at` | Unix 毫秒时间戳（不做时区格式化）|

## 典型场景

### 场景 1：轮询发布进度

```bash
# 触发发布
lark-cli apps +publish --app-id app_xxx
# 拿到 release_id，查状态
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy
```

- `status=publishing`：发布仍在进行，稍后重试
- `status=finished`：发布成功
- `status=failed`：发布失败，用 `apps +publish-error-log` 获取错误详情

### 场景 2：发布失败后跳转错误日志

```bash
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy
```

## 协同命令

| 场景 | 命令 |
|---|---|
| 触发新发布 | `apps +publish` |
| 查发布历史（获取 release_id） | `apps +publish-history` |
| 查发布错误详情 | `apps +publish-error-log` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
