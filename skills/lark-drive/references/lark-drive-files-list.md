# drive files list（原生 API：读取 Drive 文件夹清单）

`drive files list` 是原生 API 命令，不是 shortcut。它用于读取 Drive 根目录或某个 Drive 文件夹的直接子项；如果要递归盘点目录树，Agent 必须基于返回的子文件夹 token 继续调用本命令。

## 什么时候使用

| 场景 | 是否使用 | 说明 |
|------|----------|------|
| 盘点一个已确认的 Drive 文件夹树 | 使用 | 从目标 `folder_token` 开始递归列取 |
| 盘点用户明确确认的 Drive 根目录 | 使用 | 第一层用空 `folder_token`，子文件夹继续按普通文件夹递归 |
| 验证移动 / 创建后的实际位置 | 使用 | 读取目标目录直接子项，再按需递归验证 |
| 根据关键词、标题、时间、owner 找资源 | 不使用 | 优先用 `drive +search` |
| 读取 Docx 正文内容 | 不使用 | 用 `docs +fetch` |
| 读取 Sheet / Base 内部数据 | 不使用 | 切到 `lark-sheets` / `lark-base` |

## 标准命令模板

读取普通文件夹：

```bash
lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","page_size":200}' \
  --format json
```

继续翻页：

```bash
lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","page_size":200,"page_token":"<PAGE_TOKEN>"}' \
  --format json
```

读取当前用户 Drive 根目录的直接子项：

```bash
lark-cli drive files list \
  --params '{"folder_token":"","page_size":200}' \
  --format json
```

也可以省略 `folder_token` 字段来请求根目录，但在 Agent 编排中建议显式传空字符串，避免把“忘记传参数”和“确认请求根目录”混在一起。

## 按时间排序

默认不要传 `order_by` / `direction`；服务端会按默认顺序返回。只有用户明确要求按创建时间或编辑时间排序时，才使用服务端排序参数。

按创建时间升序列出当前文件夹直接子项：

```bash
lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","order_by":"CreatedTime","direction":"ASC","page_size":200}' \
  --format json
```

按编辑时间降序列出当前文件夹直接子项：

```bash
lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","order_by":"EditedTime","direction":"DESC","page_size":200}' \
  --format json
```

以上示例返回排序后的当前页；如果返回 `has_more=true`，保持相同 `folder_token` / `order_by` / `direction` / `page_size`，把 `next_page_token` 放入 `page_token` 继续翻页。

## 参数规则

1. `folder_token` 必须放在 `--params` JSON 里；不要使用不存在的 `--folder-token` flag。
2. `page_token` 必须放在 `--params` JSON 里；不要依赖 shell 变量拼接不完整的 JSON。
3. 默认不要传 `order_by` / `direction`；只有用户明确要求按创建时间 / 编辑时间排序时才使用服务端排序参数。
4. 排序参数映射：创建时间 -> `order_by:"CreatedTime"`；编辑时间 / 修改时间 -> `order_by:"EditedTime"`；升序 -> `direction:"ASC"`；降序 -> `direction:"DESC"`。不要省略排序参数后再用 Python / shell 客户端排序替代。
5. 排序查询建议带 `page_size:200` 减少翻页；只有用户要求完整分页、递归盘点、大目录全量导出，或当前页返回 `has_more=true` 后继续翻页时，才加入 `page_token`。
6. `page_size` 在分页、递归盘点或全量导出时建议显式设置为 `200`。如果服务端或环境返回参数错误，再降级到服务端允许的值，并记录降级原因。
7. 调用前如果不确定字段结构，先运行 `lark-cli schema drive.files.list` 查看 `--params` 结构。

## 返回结构与解析

`--format json` 输出中，Agent 只使用 `data` 中符合 `schema drive.files.list` 的 API 返回字段。

常用字段：

| 字段 | 用途 |
|------|------|
| `data.files` | 当前页直接子项列表 |
| `data.has_more` | 当前目录是否还有下一页 |
| `data.next_page_token` | 下一页 token；当 `has_more=true` 时放回 `--params.page_token` |
| `data.files[].type` | 文件类型；等于 `folder` 时可递归 |
| `data.files[].token` | 当前资源 token；文件夹递归时作为下一层 `folder_token` |
| `data.files[].name` | 生成路径和展示标题 |
| `data.files[].url` | 资源浏览器链接 |
| `data.files[].owner_id` | 资源所有者 |
| `data.files[].created_time` / `data.files[].modified_time` | 创建 / 更新时间 |

字段名以 `schema drive.files.list` 为准。Agent MUST 以实际返回为准；如果字段缺失，先用 `schema drive.files.list` 或一页样本确认结构，不要猜测。

## 根目录语义

1. `folder_token` 为空字符串或省略时，请求的是当前调用用户的 Drive 根目录直接子项。
2. 根目录返回值不是递归结果；不能把根目录第一页或直接子项数量当作整个云空间资源总量。
3. 根目录只作为目录树起点。返回的子文件夹必须用其自己的 `folder_token` 继续调用 `drive files list`。
4. 根据 schema 描述，根目录第一层清单不支持分页且不返回快捷方式；不要基于根目录响应推断子文件夹内容、根目录第一层快捷方式或无法分页的根目录剩余项已经被覆盖。

## 递归盘点规则

1. 只对返回项中的 `folder` 类型继续递归。
2. 每个目录独立维护分页状态；一个目录的 `page_token` 不可复用于其他目录。
3. 对每个目录持续请求，直到返回 `has_more=false`。非根目录的普通文件夹清单可能返回 `type=shortcut` 条目；不要假设这些条目会携带 `shortcut_info` 目标信息。
4. 递归过程中生成稳定 `path`；不要只保存标题，否则同名资源无法区分。
5. URL、owner、创建时间和更新时间优先使用 `files.list` 返回字段；如果字段缺失或需要批量补齐，再使用 `drive metas batch_query`。不要从标题或路径猜元数据。
6. 深度、数量、每目录页数等限制只能作为内部批次 checkpoint；不能作为递归完成条件。
7. 达到深度 checkpoint 时，把更深层子文件夹加入 continuation queue，并在下一批从这些子文件夹继续，保留原始 `path`。
8. 达到数量 checkpoint 时，保存当前目录、当前页 token、剩余目录队列和已收集资源计数，并立即继续下一批；不要进入分析或规划阶段。

### 递归算法

Agent 盘点 Drive 文件夹树时，按以下顺序执行：

1. 初始化待处理队列，放入起点目录：
   - 普通文件夹：`{folder_token:"<folder_token>", path:"<folder_name>"}`
   - Drive 根目录：`{folder_token:"", path:""}`
2. 从队列取出一个目录，请求第一页。
3. 用 `(folder_token, page_token)` 生成当前页 key；同一页 key 只允许追加一次，避免 retry 时重复计数。
4. 从 `data.files` 取当前页直接子项，按 `dedupe_key` 去重后生成 `path` 并加入结果集。
5. 如果新追加的子项是 `folder`，把子文件夹 token、子路径和 depth 加入队列。
6. 如果 `has_more=true`，取 `data.next_page_token` 继续请求同一目录下一页。
7. 同一目录分页结束后，再处理队列中的下一个目录。
8. 如果达到深度、数量或每目录页数 checkpoint，把当前目录 / 页 token / 剩余队列 / 已访问页 key / dedupe key 写入 continuation queue，并继续下一批。
9. 普通队列和 continuation queue 都为空，且没有分页 blocker 时，才可以认为本次确认范围盘点完成。

简化伪代码：

```text
queue = [root_or_start_folder]
visited_pages = set()
dedupe_keys = set()
while queue not empty:
  folder = queue.pop()
  page_token = folder.page_token or ""
  retry_without_token = 0
  while true:
    page_key = (folder.folder_token, page_token or "first")
    page = drive files list(folder.folder_token, page_token)
    if page_key not in visited_pages:
      append only files whose dedupe_key is not in dedupe_keys
      enqueue newly appended child folders with folder_token, path, and depth
      add page_key to visited_pages
    if page.has_more != true:
      break
    next = page.next_page_token
    if next is empty:
      retry_without_token += 1
      if retry_without_token >= 3:
        record pagination blocker for folder
        break
      continue
    page_token = next
    retry_without_token = 0
```

## 分页与异常

1. 默认手动处理 `has_more` 和返回中的 `next_page_token`。
2. 不要使用 `--page-all` 作为脚本 JSON 解析输入；自动翻页输出可能不适合直接 `json.loads`。
3. 如果 `has_more=true` 但没有可用的 `next_page_token`，重试同一页最多 3 次。
4. 重试后仍无 continuation token 时，记录受影响的目录和 pagination blocker，停止扩展该目录；不要无限循环，也不要宣称该目录已完整覆盖。
5. 如果触发深度、数量或每目录页数限制，把它视为批处理 checkpoint；在确认范围内继续下一批，而不是把当前结果说成完整。
6. 不要因为达到 `max_depth=3`、`max_items=500` 或类似单批阈值就结束盘点；只有队列耗尽或遇到权限 / API / 工具预算 blocker 才能结束当前确认范围的盘点。

## JSON 解析规则

1. stdout 是数据通道。脚本解析 JSON 时只读取 stdout。
2. stderr 可能包含刷新 token、进度、warning 或其他提示；不要把 stderr 合并进 JSON 输入，例如不要用 `2>&1` 后再 `json.loads`。
3. 使用 `--format json` 保持 stdout 为结构化 JSON；解析 Drive 文件清单时只读取 `data.files` / `data.has_more` / `data.next_page_token` 等 schema 字段。
4. 不要用根目录响应数量或当前页数量推断递归总量；递归总量必须由实际遍历并去重后的资源集合计算。

## 常见错误

| 错误用法 | 问题 | 正确做法 |
|----------|------|----------|
| `lark-cli drive files list --folder-token <token>` | `files.list` 不提供 `--folder-token` flag | 使用 `--params '{"folder_token":"<token>"}'` |
| 根目录返回 N 项就认为云空间只有 N 项 | 根目录只返回直接子项，不是递归结果 | 对返回的子文件夹继续递归 |
| `--page-all \| python json.loads(...)` | 自动翻页输出不适合作为单个 JSON 对象解析 | 手动使用 `page_token` 翻页并逐页解析 |
| `cmd 2>&1` 后解析 JSON | stderr 提示污染 JSON 输入 | 只解析 stdout，stderr 作为日志处理 |
