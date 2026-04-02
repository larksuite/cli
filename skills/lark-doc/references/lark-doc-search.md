
# docs +search（云空间搜索：文档 / Wiki / 电子表格）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

基于 Search v2 接口 `POST /open-apis/search/v2/doc_wiki/search`，以**用户身份**统一搜索云空间对象。

虽然接口名是 `doc_wiki/search`，但命中结果不只限于文档 / Wiki，也会返回 `SHEET`。因此它适合作为云空间对象的资源发现入口：先定位文档、知识库节点、电子表格，以及用户以“表格 / 报表”方式描述的相关对象，再切回对应业务 skill 做对象内部操作。

该 shortcut 会：

- 自动补齐 `doc_filter` / `wiki_filter`（API 必填）
- 支持在 `filter.open_time` / `filter.create_time` 中使用 ISO 8601 时间，并自动转换为 Unix 秒
- 在返回结果中为 `*_time` 字段补充 `*_time_iso`（便于阅读）
- `title_highlighted` / `summary_highlighted` 可能包含高亮标签（如 `<h>` / `<hb>`）

## 命令

> **关键约束：搜索关键词必须通过 `--query` 传递。**
> 正确：`lark-cli docs +search --query "方案"`
> 错误：`lark-cli docs +search 方案`
> `+search` 不接受“搜索词位置参数”这种写法；如果把关键词直接跟在命令后面，不会进入 `query`，会变成空搜或返回不符合预期的结果。

```bash
# 关键词搜索
lark-cli docs +search --query "季度总结"

# 搜标题里带“评测结果”的电子表格 / 文档
lark-cli docs +search --query "评测结果"

# 标题包含关键词（默认按关键词检索，不做精确标题匹配）
lark-cli docs +search --query "方案"

# 按最近打开时间过滤
lark-cli docs +search \
  --query "方案" \
  --filter '{"open_time":{"start":"2025-09-24T00:00:00+08:00","end":"2025-12-24T23:59:59+08:00"}}'

# 空搜（不传 query 或传空字符串）：按最近浏览等默认规则返回
lark-cli docs +search

# 人类可读格式输出
lark-cli docs +search --query "OKR" --format pretty

# 返回原始 JSON，并用 page_token 翻页
lark-cli docs +search --query "方案" --format json
lark-cli docs +search --query "方案" --format json --page-token '<PAGE_TOKEN>'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--query <text>` | 否 | 搜索关键词。**支持高级 Boolean 语法**以提升搜索精度：<br>1. 使用空格表示 AND（如 `方案 设计`）。<br>2. 使用 `OR` 表示逻辑或（如 `方案 OR 草稿`）。<br>3. 使用 `-` 表示排除（如 `方案 -草稿`）。<br>4. 使用双引号 `""` 表示精确匹配短语。<br>5. 使用 `intitle:` 限定关键词出现在标题中（如 `intitle:总结` 或 `intitle:"季度 总结"`）。不传/空字符串表示空搜。**凡是有关键词，都要显式通过 `--query` 传递，不要写成位置参数。** |
| `--filter <json>` | 否 | JSON 对象，会同时应用到 `doc_filter` 与 `wiki_filter` |
| `--page-size <n>` | 否 | 每页数量（默认 15，最大 20） |
| `--page-token <token>` | 否 | 翻页标记（配合 `has_more` 使用） |
| `--format` | 否 | 输出格式：json（默认） \| pretty |

## 结果判别

- `result_meta.doc_types == SHEET`：电子表格，后续切到 `lark-sheets`
- 其他类型：继续按对应 skill 或 API 处理

## 决策规则

- 参数传递：只要用户给了搜索关键词，就必须显式使用 `--query "<关键词>"`。不要生成 `lark-cli docs +search 方案`、`lark-cli docs +search xxx（搜索关键词）` 这种位置参数写法。
- 查询语义：必须优先利用 --query 的高级语法（如 intitle:、""、-）将过滤逻辑下推给服务端。当用户要求“标题精确等于 X”时，直接使用 --query "intitle:\"X\""，严禁先进行模糊搜索再做客户端二次筛选。只有在遇到服务端语法无法覆盖的复杂本地比对场景时，才允许在客户端过滤，且比对前必须先去掉 title_highlighted 里的高亮标签。
- 入口选择：用户说“找表格标题”“找名为 `X` 的电子表格”“搜某个报表”时，也默认走 `docs +search`。不要误用 `sheets +find` 做跨文件搜索。
- 分页策略：默认只返回**第一页**，并说明 `has_more` / `page_token`。只有当用户明确要求“全部结果”“继续翻页”“全量扫描”“所有结果”“完整列表”时，才继续翻页。
- 翻页上限：即使用户要求全量，单轮也最多先拉 **5 页**（按默认 `page-size=20` 约等于最多 100 条结果）。达到上限后，先回报当前进度和是否还有更多页，再让用户决定是否继续下一批。
- 总数口径：`total` 是 OpenAPI 的搜索结果总数，不一定等于客户端二次筛选后的精确数量。凡是依赖本地过滤、去重、精确标题匹配的场景，都不要默认承诺“精确总数”。
- 原始返回：如果用户要求“直接返回接口数据 / 原始返回”，优先使用 `--format json`，不要额外做精确标题过滤或摘要重写。
- 时间表达：用户如果说“3 到 6 个月前”“最近半年内”等相对时间，先转换成明确的绝对时间，再写入 `filter.open_time` / `filter.create_time`。
- 跨 skill handoff：如果搜索的目标是某个 spreadsheet，返回命中的标题、URL、token 等定位信息后，应切换到 `lark-sheets` 继续后续操作，不要把 `docs +search` 当成对象内部查询。

## 权限

| 操作 | 所需 scope |
|------|-----------|
| 搜索云空间对象（含文档 / Wiki / 表格资源发现） | `search:docs:read` |
