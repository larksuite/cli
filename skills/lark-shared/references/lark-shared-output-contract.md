# JSON 输出契约

`--format json`（默认）下，成功与错误的信封结构不同：

成功信封写入 **stdout**（退出码 0）：

```json
{ "ok": true, "identity": "user", "data": { "guid": "..." }, "meta": { "count": 1 } }
```

错误信封写入 **stderr**（退出码非 0）：

```json
{ "ok": false, "identity": "user", "error": { "type": "authorization", "subtype": "missing_scope", "code": 99991679, "message": "...", "hint": "...", "missing_scopes": ["..."] } }
```

**判断成功必须用 `ok == true`（或进程退出码 0），不要用 `code == 0`**：成功信封没有顶层 `code` / `msg` 字段，`code` 只出现在错误信封的 `error` 内，含义是上游 OpenAPI 的 numeric code。按 OpenAPI 老格式 `{"code": 0, "msg": "ok"}` 判断会把所有成功调用误判为失败；封装写入类命令（如 `task +create`）时尤其危险，误判会绕过幂等逻辑导致重复创建。

传输或信封成功不等于业务已经完成；当所选业务域提供明确的完成契约（例如 `data.completion`、`meta.pagination.complete` 和配套 `hint`）时，必须服从该域定义的完成态与恢复指引。

## 调用方持有的幂等键

当命令提供 `--idempotency-key`、`--uuid` 或 `client_token` 让调用方防止重复写入时，优先复用上层 workflow/job 已持久化的稳定 ID。没有可复用 ID 时，在首次请求前通过代码或 UUID 工具单独生成一次，例如：

```bash
python3 -c 'import uuid; print(uuid.uuid4())'
```

把生成结果作为字面量传给命令。同一次逻辑写入的重试必须复用该字面量以及相同参数、身份和 profile；新的逻辑写入生成新值。禁止在可能重试的命令中使用 `$(uuidgen)`、反引号或其他每次执行都会生成新值的表达式。命令是否必传、长度限制和有效窗口以当前 leaf `--help` 为准。
