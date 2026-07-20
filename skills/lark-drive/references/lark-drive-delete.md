
# drive +delete

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

删除云空间（云盘/云存储）内的文件或文件夹。删除后资源会进入回收站。

> [!CAUTION]
> 这是**高风险写操作**。CLI 层要求显式传 `--yes`；如果用户已经明确要求删除且目标明确，直接执行并带上 `--yes`。
> “目标明确”表示用户给出了可解析为 `file-token` + `type` 的具体 URL/token，或对你刚列出的可解析资源列表逐项/整批确认删除。按“没用的”“临时的”“疑似重复的”“全部旧文件”等描述搜索出来的候选属于待确认目标；这类请求先列候选、说明筛选依据和影响范围，然后停止等待确认。

## 删除前门槛

执行 `drive +delete --yes` 前同时满足：

| 条件 | 可执行信号 |
|------|------------|
| 具体目标 | 单个可解析为 `file-token` + `type` 的 URL/token，或用户确认过且可解析的资源列表 |
| 执行确认 | 用户在本轮明确说确认删除这些具体目标 |

若缺少任一条件，使用 `drive +search`、`drive +inspect` 或只读 API 收集候选并回复待确认清单；启发式规则（打开时间、标题模式、owner、文件类型等）只能作为候选筛选依据，不能升级为删除确认。执行 `drive +delete` 时必须使用解析后的 `--file-token` 和 `--type`。

## 批量删除建议

批量删除文件或文件夹时，建议逐个串行处理，不要并发执行删除命令，并发删除可能触发服务端加锁或冲突，导致部分删除失败；这类失败通常需要等待后对单个失败项重试。

## 命令

```bash
# 删除普通文件（异步操作，会自动有限轮询任务状态）
lark-cli drive +delete \
  --file-token <FILE_TOKEN> \
  --type file \
  --yes

# 删除在线文档（异步操作，会自动有限轮询任务状态）
lark-cli drive +delete \
  --file-token <DOCX_TOKEN> \
  --type docx \
  --yes

# 删除文件夹（异步操作，会自动有限轮询任务状态）
lark-cli drive +delete \
  --file-token <FOLDER_TOKEN> \
  --type folder \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token` | 是 | 需要删除的文件或文件夹 token |
| `--type` | 是 | 文件类型，可选值：`file`、`docx`、`bitable`、`doc`、`sheet`、`mindnote`、`folder`、`shortcut`、`slides` |
| `--yes` | 是 | 确认执行高风险删除操作 |

## 行为说明

- **删除可能需要等待**：删除操作在服务端可能异步处理，shortcut 会在本次命令内自动做有限次数的结果轮询
- **已完成则停止**：如果返回 `deleted=true`，且没有返回 `next_command`，说明删除已经完成，不需要再调用 `drive +task_result`
- **未完成再续查**：如果超过内置轮询次数仍未完成，会返回 `ready=false`、`timed_out=true`、`task_id` 和 `next_command`；此时按 `next_command` 继续查询删除结果
- **task_id 不是成功条件**：`task_id` 只是续查凭据。没有 `task_id` 但返回 `deleted=true` 时，也表示删除已完成
- **失败处理**：如果返回 `failed=true` 或 `status=fail`，按错误信息和 `task_id` 报告删除失败；不要重复删除同一资源

## 常见错误处理

| 错误码 | 含义 | 建议处理 |
|--------|------|----------|
| `1061007` | 文件已删除 | 视为目标已不可用，无需重试删除 |
| `99991400` | 命中接口限频 | 等待一段时间后重试；批量删除时保持串行并降低频率 |
| `99991679` | 缺失 scope | 按错误里的 `missing_scopes`、`hint` 申请/授权所需 scope 后重试 |

## 推荐续跑方式

```bash
# 第一步：先直接删除资源
lark-cli drive +delete \
  --file-token <FILE_OR_FOLDER_TOKEN> \
  --type <TYPE> \
  --yes

# 只有返回 ready=false / timed_out=true 或 next_command 时，才需要继续查
lark-cli drive +task_result \
  --scenario task_check \
  --task-id <TASK_ID>
```

## 限制

- 该 shortcut 仅支持云空间（云盘/云存储）文件或文件夹，不支持 wiki 文档
- 该接口不支持并发调用
- 调用频率上限为 5 QPS 且 10000 次/天

## 权限要求

- 删除文件时，调用身份需要满足以下其一：
- 是文件所有者，并且拥有该文件所在父文件夹的编辑权限
- 不是文件所有者，但拥有该父文件夹的 owner 或 full access 权限

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
