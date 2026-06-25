# 权限治理 Command Patterns

本文只提供 `permission_governance` workflow 的具体 `lark-cli` 命令样例。只有进入对应 state 且需要拼装命令时才读取本文；命令可用范围仍以 [`lark-drive-workflow-permission-governance.md`](lark-drive-workflow-permission-governance.md) 的 `Command Map` 为准。

## 目录

- `目标解析`
- `目标发现`
- `事实读取`
- `写前确认与执行`

## 目标解析

```bash
lark-cli drive +inspect --url '<url>' --as user --format json
```

`/wiki/space/<space_id>` URL 是 Wiki space 范围，不要用 `drive +inspect` 当作单文档解析；直接提取 `space_id` 后进入 `DISCOVER_TARGETS`。

## 目标发现

发现 Wiki space / node 下目标：

```bash
lark-cli wiki +node-list \
  --space-id '<space_id>' --page-size 50 \
  --page-all --page-limit 0 \
  --as user --format json

lark-cli wiki +node-list \
  --space-id '<space_id>' --parent-node-token '<node_token>' --page-size 50 \
  --page-all --page-limit 0 \
  --as user --format json

lark-cli wiki +node-list \
  --space-id '<space_id>' --page-token '<PAGE_TOKEN>' --page-size 50 \
  --as user --format json
```

解析返回时使用 `data.nodes`，不要读取顶层 `items`。`--page-limit 0` 表示当前层分页不设页数上限；`--page-all` 只覆盖当前 `space-id` / `parent-node-token` 范围内的分页，不会递归子节点。节点 `has_child=true` 时，必须继续以该节点的 `node_token` 作为 `--parent-node-token` 递归读取。

发现 Drive folder 下目标：

```bash
lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","page_size":200}' \
  --as user --format json

lark-cli drive files list \
  --params '{"folder_token":"<folder_token>","page_size":200,"page_token":"<PAGE_TOKEN>"}' \
  --as user --format json
```

## 事实读取

读取 metadata：

```bash
lark-cli drive metas batch_query \
  --data '{"request_docs":[{"doc_token":"<token>","doc_type":"<type>"}],"with_url":true}' \
  --as user --format json
```

读取 public permission：

```bash
lark-cli drive permission.public get \
  --params '{"token":"<token>","type":"<type>"}' \
  --as user --format json
```

按需读取访问统计：

```bash
lark-cli drive file.statistics get \
  --params '{"file_token":"<token>","file_type":"<type>"}' \
  --as user --format json
```

按需读取最近访问记录：

```bash
lark-cli drive file.view_records list \
  --params '{"file_token":"<token>","file_type":"<type>","page_size":50}' \
  --as user --format json
```

## 写前确认与执行

patch 前检查 manage-public permission：

```bash
lark-cli drive permission.members auth \
  --params '{"token":"<token>","type":"<type>","action":"manage_public"}' \
  --as user --format json
```

patch 前读取当前 schema：

```bash
lark-cli schema drive.permission.public.patch --format json
```

只 patch 当前 schema 支持的字段；对 Wiki 目标，必须省略 schema 明确标注为 Wiki 不支持的字段。

显式确认后 patch public permission：

```bash
lark-cli drive permission.public patch \
  --params '{"token":"<token>","type":"<type>"}' \
  --data '{"link_share_entity":"closed","external_access":false}' \
  --as user --yes --format json
```

显式确认后申请访问权限：

```bash
lark-cli drive +apply-permission \
  --token '<url>' \
  --perm view --remark '<reason>' --as user --format json

lark-cli drive +apply-permission \
  --token '<bare-token>' --type '<type>' \
  --perm view --remark '<reason>' --as user --format json
```

owner 转移前读取当前 schema：

```bash
lark-cli schema drive.permission.members.transfer_owner --format json
```

显式确认后转移 owner：

```bash
lark-cli drive permission.members transfer_owner \
  --params '{"token":"<token>","type":"<type>","need_notification":true,"remove_old_owner":false,"old_owner_perm":"full_access","stay_put":true}' \
  --data '{"member_id":"<new_owner_open_id>","member_type":"openid"}' \
  --as user --yes --format json
```

`member_type` 只能使用当前 schema 支持的值：`email`、`openid`、`userid`、`appid`。如果用户只给姓名，必须先解析为明确身份或要求用户补充；不要猜测 `member_id`。批量 owner 转移必须逐个目标顺序执行。

secure label 写前枚举可用标签：

```bash
lark-cli drive +secure-label-list \
  --page-size 10 --lang zh \
  --as user --format json

lark-cli drive +secure-label-list \
  --page-size 10 --page-token '<PAGE_TOKEN>' --lang zh \
  --as user --format json
```

当用户给出的是标签名称、密级文案或不确定的 label ID 时，必须先枚举并解析为 `label-id`；写入确认里展示目标标签名称和 ID。找不到唯一标签时，停止并让用户选择，不要猜测。

显式确认后更新 secure label：

```bash
lark-cli drive +secure-label-update \
  --token '<url>' \
  --label-id '<label-id>' --as user --format json

lark-cli drive +secure-label-update \
  --token '<bare-token>' --type '<type>' \
  --label-id '<label-id>' --as user --format json
```
