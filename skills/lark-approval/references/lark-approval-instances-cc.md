
# approval instances cc

给一个审批实例追加抄送人（用户级写操作）。通常先通过 `instances initiated`、`tasks query` 或 `instances get` 确认目标审批实例，拿到 `instance_code` 后，再提供抄送人的用户 ID 执行抄送。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要抄送该审批实例且目标实例、抄送对象都无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:instance:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval instances cc \
  --data '{"instance_code":"<INSTANCE_CODE>","cc_user_ids":["ou_xxx"],"comment":"抄送给项目 owner 了解进展"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --dry-run

# 按 open_id 抄送一个人
lark-cli approval instances cc \
  --data '{"instance_code":"<INSTANCE_CODE>","cc_user_ids":["ou_xxx"],"comment":"抄送给你知悉"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 一次抄送多个人
lark-cli approval instances cc \
  --data '{"instance_code":"<INSTANCE_CODE>","cc_user_ids":["ou_xxx","ou_yyy"],"comment":"请相关同学同步关注"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 按 user_id 抄送
lark-cli approval instances cc \
  --data '{"instance_code":"<INSTANCE_CODE>","cc_user_ids":["123456789"],"comment":"抄送给财务负责人"}' \
  --params '{"user_id_type":"user_id"}' \
  --as user \
  --yes

# 通过文件传入请求体
lark-cli approval instances cc \
  --data @./cc-body.json \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `instances initiated`、`tasks query` 或 `instances get` 获取 |
| `cc_user_ids` | 是 | 抄送人的用户 ID 数组；需要和 `user_id_type` 保持一致 |
| `comment` | 否 | 抄送留言，例如 `抄送给你知悉`、`请同步关注该审批进展` |
| `--params '{"user_id_type":"..."}'` | 否 | 查询参数 JSON；用于声明 `cc_user_ids` 内用户 ID 的类型 |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id`；未显式指定时要特别确认抄送人的 ID 类型 |
| `--as user` | 否 | 建议显式指定用户身份；审批实例抄送通常必须以用户身份执行 |
| `--yes` | 否 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 典型前置步骤

如果你要找“我发起的审批实例”，可先查询已发起列表：

```bash
lark-cli approval instances initiated --params '{"page_size":20}' --as user
```

如果你已经在任务列表中定位到某个审批，也可以从任务里拿到实例 Code：

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

常用到的字段：

| 字段 | 说明 |
|------|------|
| `instances[].instance_code` | 审批实例 Code；抄送时必须提供 |
| `tasks[].instance_code` | 审批任务关联的审批实例 Code；也可作为抄送输入 |
| `tasks[].title` | 任务标题，可用于确认是否是要操作的那个审批 |
| `tasks[].instance_status` | 审批实例状态；可用于判断当前审批是否仍处于进行中 |

如果你手里只有姓名或邮箱，建议先通过联系人能力解析出正确的用户 ID，再执行抄送。

如需先确认审批表单、当前节点、流转状态，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **抄送的是审批实例，不是单个任务**：`instances cc` 只需要 `instance_code`，不需要 `task_id`。
- **`cc_user_ids` 与 `user_id_type` 必须匹配**：例如传 open_id 就把 `user_id_type` 设为 `open_id`；不要混用。
- **`cc_user_ids` 是数组**：即使只抄送一个人，也要按数组形式传入。
- **优先显式传 `user_id_type`**：这样 agent 更容易判断参数含义，也能减少 ID 类型不匹配带来的失败。
- **优先从 `instances initiated` 获取目标实例**：因为抄送常见于“我发起的审批”场景，这个入口最直接。
- **也可从 `tasks query` 反查 `instance_code`**：当你是从某个审批上下文进入时，这样更方便。
- **`comment` 建议简洁明确**：例如 `抄送给你知悉`、`请同步关注审批进展`。避免过长或模糊描述。
- **先 `--dry-run` 再执行**：尤其在抄送对象较多、抄送人来源不明确，或需要让用户先核对实例标题时，先预览更安全。
