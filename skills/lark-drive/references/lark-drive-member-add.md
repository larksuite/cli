# drive +member-add（添加协作者/授权成员权限）

> 这是高风险写操作。真实执行会修改文档权限，需要显式加 `--yes`

## 命令

```bash

# 批量添加（同一 member-type 和 perm，最多 10 人）
lark-cli drive +member-add \
  --token "<bare_token_or_url>" \
  --type bitable \
  --member-id "ou_a,ou_b" \
  --member-type openid \
  --perm view \
  --yes
```

## 参数

| 参数 | 必填 | 说明                                                                                                                                                                                  |
|------|----|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--token` | 是 | 裸 token 或完整 URL。路径支持 `/drive/folder/`、`/docx/`、`/doc/`、`/sheets/`、`/base/`、`/bitable/`、`/wiki/`、`/file/`、`/mindnotes/`、`/slides/`、`/minutes/`；URL 输入可从路径推断 `--type`，裸 token 不做前缀推断 |
| `--type` | 必填 | 目标资源类型：`docx` / `doc` / `sheet` / `bitable` / `file` / `folder` / `wiki` / `mindnote` / `slides` / `minutes`。传 URL 时可省略；裸 token 必须显式传；若同时传 URL 和 `--type`，显式 `--type` 覆盖 URL 推断                 |
| `--member-id` | 是 | 协作者 ID；逗号分隔可批量添加，最多 10 个                                                                                                                                                            |
| `--member-type` | 是 | member-id 的类型；支持 `email` / `openid` / `unionid` / `openchat` / `opendepartmentid` / `groupid` / `appid` / `wikispaceid`。在实际使用里，给当前应用授权仍优先推荐 bot `open_id` + `openid`。                               |
| `--member-kind` | 条件必填 | 仅当 `--member-type=wikispaceid` 时填写，映射到请求 body 的 `type` 字段。取值：`wiki_space_member` / `wiki_space_viewer` / `wiki_space_editor`。其他 member-type 禁止传此参数。 |
| `--perm` | 否 | 授权角色：`view`（默认）/ `edit` / `full_access`                                                                                                                                             |
| `--perm-type` | 否 | 只作用 wiki 节点权限范围：`container`（默认，当前页面+子页面）/ `single_page`（仅当前页面）                                                                                                                      |
| `--need-notification` | 否 | 是否通知对方。仅 `--as user` 可用；未传时不会写入 query，`--need-notification=false` 表示显式不通知                                                                                                           |
| `--dry-run` | 否 | 仅打印请求，不实际授权                                                                                                                                                                         |
| `--yes` | 真实执行时是 | 确认高风险写操作                                                                                                                                                                            |

## 输出

批量成功：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "resource_token": "doc_token_or_url",
    "resource_type": "docx",
    "requested_count": 2,
    "succeeded_count": 2,
    "partial": false,
    "members": [
      {"resource_token": "doc_token_or_url", "resource_type": "docx", "member_id": "ou_a", "member_type": "openid", "member_kind": "user", "perm": "view"},
      {"resource_token": "doc_token_or_url", "resource_type": "docx", "member_id": "ou_b", "member_type": "openid", "member_kind": "user", "perm": "view"}
    ],
    "missing_member_ids": []
  }
}
```

批量部分失败时，`partial` 为 `true`，同一份结果以 `ok:false` 部分失败信封写到 **stdout**（stderr 不再输出单独的错误信封），CLI 以非零退出码结束。检查 `data` 中的 `requested_count`、`succeeded_count`、`members`、`missing_member_ids` 和可选的 `mismatched_member_ids`。响应顺序不影响匹配结果。

## 行为说明

- **身份支持**：`--as user` 和 `--as bot` 均可使用。
- **部门协作者**：`--member-type=opendepartmentid` 必须配合 `--as user`；bot 身份不支持添加部门协作者。
- **通知**：`--need-notification` 仅 `--as user` 时有效；`--as bot` 时传此参数会被拒绝。
- **批量约束**：批量请求共享同一 `--member-type`、`--perm` 和 `--perm-type`；混合用户/群组/部门的场景需拆分为多次调用。
- **Wiki 空间 ID**：`--member-type=wikispaceid` 时必须同时传 `--member-kind`，否则 API 会缺少必填的 body `type` 字段。`wiki_space_member` 对应知识库成员角色；若知识库已将成员拆分为可阅读/可编辑成员组，改用 `wiki_space_viewer` 或 `wiki_space_editor`。
- **ID 解析**：优先用 `open_id` + `--member-type openid`；仅在无法解析 `open_id` 时使用 `email`。群组优先用 `openchat`，部门用 `opendepartmentid`。
