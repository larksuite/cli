# Table 创建与复制

用于在已有 Base 中创建数据表、复制表结构或连同记录复制，以及跟踪异步复制结果。创建整个 Base 和首表见 [Base 与 Block 管理](lark-base-resource-management.md)。示例中的 token / ID 替换为真实值。

## 创建表：+table-create

`--base-token`、`--name`、`--fields` 必填。`--fields` 必须是至少包含一个字段对象的 JSON 数组，第一个字段成为主字段。字段类型、属性和复杂字段构造读取 [Field JSON](lark-base-field-schema.md)，不要使用 Record 的 `{字段名: CellValue}` 格式，也不要先创建默认表再逐列补齐已知字段。

```bash
# 创建最小表：一个文本主字段
lark-cli base +table-create --base-token "<base_token>" --name "项目" \
  --fields '[{"name":"项目名称","type":"text"}]' --as user

# 一次创建包含文本、单选和数字字段的业务表
lark-cli base +table-create --base-token "<base_token>" --name "任务" \
  --fields '[{"name":"标题","type":"text"},{"name":"状态","type":"select","multiple":false,"options":[{"name":"待处理"},{"name":"完成"}]},{"name":"工时","type":"number"}]' \
  --as user
```

- 空字符串、空数组以及非对象的字段项都不是有效 schema。复杂字段可继续阅读字段协议链接的 Formula / Lookup 指南。
- 可选 `--view` 接受视图 JSON 对象或数组；需要时先读取 [View 类型与生命周期](lark-base-view.md)。视图在表创建后依次创建，后续视图失败不代表表未创建；先定位已有表和视图，只补失败部分，不要重建整表。
- 本命令不接收 `--parent-id`。需要放入 Folder 时，使用返回的 Table ID 作为 Block ID，再按 [Block 移动](lark-base-resource-management.md#移动和排序base-block-move) 操作。

## 复制表：+table-copy

源表和目标表都在 `--base-token` 指定的 Base 内；`--table-id` 接受源表 ID 或名称，`--name` 是目标表名称。

```bash
# 只复制结构；默认 --range schema，不复制记录
lark-cli base +table-copy --base-token "<base_token>" \
  --table-id "<source_table_id>" --name "任务模板" --as user

# 连同记录复制，提交后根据结果继续查询任务
lark-cli base +table-copy --base-token "<base_token>" \
  --table-id "<source_table_id>" --name "任务备份" --range all --as user
```

- 仅在用户要求复制记录时使用 `--range all`。结构复制同步完成；包含记录的复制可能异步执行。
- 需要本地等待时，在第二条命令加 `--wait`；默认等待最多 5 分钟，可用 `--timeout 10m` 调整，上限 30 分钟且必须大于零。`--wait` 只用于 `--range all`，`--timeout` 必须与 `--wait` 一起使用。
- 检查返回的 `state` 和 `completed`：`init` / `process` 表示尚未完成；`success` 且 `completed=true` 才是完成。HTTP 或命令成功不等于异步复制完成。
- 保存 `task_id` 和 `next_command`。等待超时（`timed_out=true`）、中断或状态查询失败不等于任务取消；已有 task ID 时继续查询原任务，不重新提交复制。提交本身超时而没有收到 task ID 时，先确认目标表是否已创建，再决定后续动作。

## 查询复制结果：+table-copy-status

```bash
lark-cli base +table-copy-status --base-token "<base_token>" \
  --task-id "<task_id_from_copy>" --as user
```

`--task-id` 使用复制响应的原值，不是 Table ID，不自行构造或解码。本命令只查询一次；保持与提交时相同的 Base 和身份，仍为 `init` / `process` 时稍后按返回的 `next_command` 继续查询。结果未完成或查询失败时，如实说明当前状态，不宣称复制成功，也不通过重复复制恢复。
