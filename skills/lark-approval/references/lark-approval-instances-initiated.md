
# approval instances initiated

查询当前用户已发起的审批实例列表（用户级只读操作）。适合在需要查看“我发起了哪些审批”、筛选某类审批定义、获取 `instance_code` 供后续 `instances get` / `instances cancel` / `instances cc` 等命令使用时调用。

需要的 scopes: ["approval:instance:read"]

## 命令

```bash
# 查询我发起的审批列表
lark-cli approval instances initiated --params '{"page_size":20}' --as user

# 只看某个审批定义下我发起的实例
lark-cli approval instances initiated --params '{"definition_code":"<DEFINITION_CODE>","page_size":20}' --as user

# 使用 page_token 翻页
lark-cli approval instances initiated --params '{"page_size":20,"page_token":"example_page_token"}' --as user

# 表格格式输出，便于快速浏览
lark-cli approval instances initiated --params '{"page_size":20}' --format table --as user

# 预览 API 调用，不执行
lark-cli approval instances initiated --params '{"page_size":20}' --as user --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 否 | 查询参数，使用 JSON 传入；不传时使用默认分页与筛选 |
| `definition_code` | 否 | 审批定义 Code，用于只查看某个审批定义下我发起的实例 |
| `locale` | 否 | 返回语言：`zh-CN`、`en-US`、`ja-JP` |
| `page_size` | 否 | 分页大小 |
| `page_token` | 否 | 翻页标记；首次请求不填，后续使用上一次返回的 `page_token` |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id` |
| `--as user` | 否 | 建议显式指定用户身份；已发起审批列表查询通常应使用用户身份 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 输出重点字段

返回结果中常见字段：

| 字段 | 说明 |
|------|------|
| `count` | 列表计数，只在第一页返回；大于等于 100 个实例时返回 `99` |
| `has_more` | 是否还有更多数据 |
| `page_token` | 下一页翻页 Token |
| `instances[].instance_code` | 审批实例 Code；后续查询详情或执行撤回 / 抄送时通常需要 |
| `instances[].definition_code` | 审批定义 Code |
| `instances[].definition_name` | 审批定义名称 |
| `instances[].definition_group_id` | 审批定义分组 ID |
| `instances[].definition_group_name` | 审批定义分组名称 |
| `instances[].initiator` | 发起人 ID |
| `instances[].initiator_name` | 发起人姓名 |
| `instances[].instance_status` | 审批实例状态，见下方“instance_status 枚举” |
| `instances[].instance_external_id` | 第三方审批实例 ID（仅第三方审批实例存在） |
| `instances[].link` | 三方审批跳转链接 |
| `instances[].summaries` | 摘要字段列表 |

## instance_status 枚举

| 值 | 含义 |
|----|------|
| `0` | 无流程状态，不展示对应标签 |
| `1` | 流程实例流转中 |
| `2` | 已通过 |
| `3` | 已拒绝 |
| `4` | 已撤销 |
| `5` | 已终止 |

## 常见使用场景

### 1) 找到我要操作的审批实例

```bash
lark-cli approval instances initiated --params '{"page_size":20}' --format table --as user
```

拿到 `instances[].instance_code` 后，可继续：

```bash
# 查看审批实例详情
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user

# 撤回审批实例
lark-cli approval instances cancel --data '{"instance_code":"<INSTANCE_CODE>"}' --as user --yes
```

### 2) 只看某类审批

```bash
lark-cli approval instances initiated \
  --params '{"definition_code":"<DEFINITION_CODE>","page_size":20}' \
  --as user
```


## 使用建议

- **这是定位“我发起的审批实例”的首选命令**：如果你的目标是撤回、抄送、查看某个已发起审批，优先从这里拿 `instance_code`。
- **优先用 `definition_code` 缩小范围**：当你已知审批定义时，先筛掉无关实例，可显著提升可读性。
- **结果很多时优先 `--format table`**：适合人工快速浏览。
- **`count` 只在第一页返回**：做分页处理时不要假设后续页还会带总数。
- **`instance_status` 可直接判断下一步**：例如状态为 `1` 时通常可继续查看详情或考虑撤回，状态为 `4` 表示已经撤销，无需重复撤回。
- **摘要字段 `summaries` 很适合做列表预览**：当审批标题不够明确时，可结合摘要值帮助识别目标实例。

## 输出与后续操作

拿到列表后，常见下一步：

```bash
# 查看单个审批实例详情
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user

# 撤回审批实例
lark-cli approval instances cancel --data '{"instance_code":"<INSTANCE_CODE>"}' --as user --yes

# 给审批实例追加抄送人
lark-cli approval instances cc --data '{"instance_code":"<INSTANCE_CODE>","cc_user_ids":["<USER_ID>"]}' --params '{"user_id_type":"open_id"}' --as user --yes
```
