# Base 读取、创建、复制、设置与 Block 管理

用于读取 Base 信息、创建或复制 Base、修改 Base 时区，以及管理 Base 目录中的 folder、table、docx、dashboard 和 workflow。示例中的 token / ID 替换为真实值，默认使用用户身份。

## 读取 Base：+base-get

读取 Base 本体信息；目录和资源列表用下方 `+base-block-list`，表内数据用 Record 命令。

```bash
lark-cli base +base-get --base-token "<base_token>" --as user
```

`--base-token` 必须是真实 Base token，不接受 Workspace token 或 Wiki 节点 token。返回的 `base` 包含 Base 信息；读取权限状态时检查其中的 `is_advanced`，后续操作按 [权限与角色](lark-base-advanced-permission-and-role.md) 执行。

## 创建 Base：+base-create

用户需要新 Base 且已明确首表字段时，用一次命令同时创建 Base、首表和字段，不要先创建默认表再逐列重建。构造 `--fields` 前读取 [Field JSON](lark-base-field-schema.md)；它是字段对象数组，不是 Record 单元格数据。

```bash
# 创建项目 Base，并定义首表名称和字段
lark-cli base +base-create --name "项目管理" --table-name "任务" \
  --fields '[{"name":"标题","type":"text"},{"name":"状态","type":"select","multiple":false,"options":[{"name":"待处理"},{"name":"完成"}]}]' \
  --time-zone Asia/Shanghai --as user

# 用户只需要使用平台默认首表结构的新 Base
lark-cli base +base-create --name "临时收集" --time-zone Asia/Shanghai --as user
```

- `--name` 必填；自定义首表时一起提供 `--table-name` 和 `--fields`。两者均省略时保留平台默认首表；只提供 `--table-name` 时只改默认首表的名称。
- `--folder-token` 是云空间目标文件夹 token，不是 Base 内 Folder Block 的 ID。已有 Base 内新建 Table 见 [Table 创建与复制](lark-base-table-create-and-copy.md)。
- 保存返回的 Base 和 Table 标识。自定义首表成功但 `default_table_deleted=false` 时，表示默认表清理失败，Base 和指定表已创建；不要重新执行创建。
- 自定义表创建失败时 CLI 会尝试删除本次新建的 Base。按实际错误和恢复提示判断是否已回滚；Base 仍存在时在原 Base 修复，结果不确定时先定位已有资源，不盲目重复创建。
- 用户明确要求 bot 身份时才改用 `--as bot`；输出带 `permission_grant` 时一并说明授权结果。

## 复制 Base：+base-copy

复制整个 Base；只复制某张表时用 [Table 创建与复制](lark-base-table-create-and-copy.md)。本命令不支持 BaseApp / AppMode，不要传入 `app_token` 冒充应用复制。

```bash
# 复制 Base 及内容
lark-cli base +base-copy --base-token "<source_base_token>" \
  --name "项目管理副本" --as user

# 用户只需要结构时，创建不含内容的副本
lark-cli base +base-copy --base-token "<source_base_token>" \
  --name "项目模板" --without-content --as user
```

- `--base-token` 指向源 Base；`--name` 为新 Base 名称。仅在用户要求只复制结构时加 `--without-content`。
- 可选 `--folder-token` 指定云空间目标文件夹，`--time-zone` 指定 IANA 时区。Base 内的 Folder Block ID 不能用作 `--folder-token`。
- 成功结果包含新 Base 的 `base` 信息和 `copied=true`，后续操作使用返回的新 Base token，不继续写源 Base。bot 身份输出带 `permission_grant` 时说明授权结果。
- 选用公开模板时，按 [模板中心](lark-base-template-center.md) 取得模板 token 再复制；不要把应用模板当作 Base 模板。
- 复制响应丢失或结果不确定时，先确认副本是否已经创建，不自动重发复制请求。

## 修改 Base 时区：+base-update

修改 Base 级时区，未涉及的设置保持不变。它不用于重命名 Table 或其他 Block。

```bash
lark-cli base +base-update --base-token "<base_token>" \
  --time-zone Asia/Shanghai --as user
```

`--time-zone` 必填，使用 IANA 时区名称。

## 发现 Block：+base-block-list

```bash
# 列出 Base 全部 Block
lark-cli base +base-block-list --base-token "<base_token>" --as user

# 只列出某个 Folder 的直接子项
lark-cli base +base-block-list --base-token "<base_token>" \
  --parent-id "<folder_block_id>" --as user
```

- 省略 `--parent-id` 返回全部 Block，不是只返回根目录；指定时仅返回该 Folder 的直接子项，不递归展开。
- 可加 `--type folder|table|docx|dashboard|workflow` 中的一个值筛选类型。本命令返回后端完整列表，不提供 `--limit` / `--offset`。
- 从返回的 `id`、`type`、`name`、`parent_id` 定位资源。Table、Dashboard、Workflow 的 `id` 分别作为对应模块 ID；Docx 正文使用额外返回的 `docx_token`，转 `lark-doc`。
- 这里只管理 Base 资源树。仪表盘内部组件走 `+dashboard-block-*`；BaseApp 页面组件走 `+app-block-*`。

## 创建 Block：+base-block-create

```bash
# 在 Base 根目录创建 Folder
lark-cli base +base-block-create --base-token "<base_token>" \
  --type folder --name "项目资料" --as user

# 在已有 Folder 内创建 Docx 资源
lark-cli base +base-block-create --base-token "<base_token>" \
  --type docx --name "项目说明" --parent-id "<folder_block_id>" --as user
```

`--type`、`--name` 必填；省略 `--parent-id` 创建在根目录，不要传字面值 `null`。创建资源入口不等于完成其内部内容。

如果已经明确类型专属初始内容，直接用对应构造命令，不要先创建同名空 Block：

- Table：`+table-create` 同时定义字段，见 [Table 创建与复制](lark-base-table-create-and-copy.md)。
- Dashboard：`+dashboard-create` 设置名称、主题；内部组件建设见 [Dashboard](lark-base-dashboard.md)。
- Workflow：`+workflow-create` 提交完整流程定义，见 [Workflow](lark-base-workflow.md)。
- Folder / Docx：使用本节命令；Docx 正文交给 `lark-doc`。

## 重命名 Block：+base-block-rename

```bash
lark-cli base +base-block-rename --base-token "<base_token>" \
  --block-id "<block_id>" --name "项目归档资料" --as user
```

用已定位的 Block ID 操作；新名称不能与同一 Base 中其他 Block 重名。

## 移动和排序：+base-block-move

```bash
# 移入 Folder，并排在目标目录中的指定同级资源之后
lark-cli base +base-block-move --base-token "<base_token>" \
  --block-id "<block_id>" --parent-id "<folder_block_id>" \
  --after-id "<sibling_block_id>" --as user

# 移回 Base 根目录
lark-cli base +base-block-move --base-token "<base_token>" \
  --block-id "<block_id>" --as user
```

- **省略 `--parent-id` 表示移到根目录，不是保留原目录。** 在 Folder 内只调整顺序时，也要带该 Folder ID。
- `--before-id` / `--after-id` 二选一，指向目标目录中的同级 Block；不要同时传入。
- 移动 Folder 时，其子项仍保留在该 Folder 下。

## 删除 Block：+base-block-delete

```bash
# 已确认用户要删除的具体资源后执行
lark-cli base +base-block-delete --base-token "<base_token>" \
  --block-id "<block_id>" --yes --as user
```

删除是高风险写操作，先确认目标和影响；用户已明确授权同一删除目标时直接使用 `--yes`。不支持递归删除非空 Folder，需先按用户意图移走或删除其子项。不同类型的底层资源按服务端语义删除，不要把资源删除当作仅隐藏目录入口。
