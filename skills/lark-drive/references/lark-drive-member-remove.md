# drive +member-remove（移除协作者权限）

> 这是高风险写操作。真实执行会移除权限，需要核对资源和成员后显式加 `--yes`。

## 命令

```bash
lark-cli drive +member-remove \
  --token "<bare_token_or_url>" \
  --type docx \
  --member-id "ou_xxx" \
  --member-type openid \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token` | 是 | 裸 token 或完整 URL。路径支持 `/drive/folder/`、`/docx/`、`/doc/`、`/sheets/`、`/base/`、`/bitable/`、`/wiki/`、`/file/`、`/mindnotes/`、`/slides/`、`/minutes/`、`/page/`；URL 可从路径推断类型，裸 token 必须同时传 `--type`。 |
| `--type` | 条件必填 | 资源类型：`docx` / `doc` / `sheet` / `bitable` / `file` / `folder` / `wiki` / `mindnote` / `slides` / `minutes` / `apps`。完整 URL 可省略。 |
| `--member-id` | 是 | 要移除的单个协作者 ID。逗号分隔的多成员输入会被拒绝；批量场景应逐个调用。 |
| `--member-type` | 是 | ID 类型：`email` / `openid` / `openchat` / `opendepartmentid` / `userid` / `unionid` / `groupid` / `wikispaceid`。 |
| `--member-kind` | 条件必填 | 仅 `--member-type=wikispaceid` 使用：未启用知识库成员分组时传 `wiki_space_member`，启用后根据权限传 `wiki_space_viewer` 或 `wiki_space_editor`。 |
| `--perm-type` | 否 | 仅 wiki 协作者使用：`container`（默认，当前页面及子页面）或 `single_page`（仅当前页面）。 |
| `--dry-run` | 否 | 只预览 DELETE URL、query 和 body，不调用接口。 |
| `--yes` | 真实执行时是 | 确认高风险权限移除操作。 |

## 输出

以移除 `openid` 类型的用户协作者为例，成功后返回：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "removed": true,
    "resource_token": "doxcnxxx",
    "resource_type": "docx",
    "member_id": "ou_xxx",
    "member_type": "openid",
    "member_kind": "user"
  }
}
```

Wiki 普通协作者还会返回 `perm_type`；`wikispaceid` 返回所传的 `member_kind`。

`removed: true` 表示删除请求成功完成，不保证该权限此前一定存在。

## 行为说明

- **身份支持**：支持 `--as user` 和 `--as bot`。
- **部门协作者**：`--member-type=opendepartmentid` 只能配合 `--as user`；bot 身份会在客户端提前拒绝。
- **安全编码**：资源 token 和 member ID 都作为独立 path segment 编码。
- **Wiki 范围**：普通 wiki 协作者默认删除 `container` 权限；只删除当前页面权限时显式传 `single_page`。
- **Wiki 空间成员**：`--member-type=wikispaceid` 仅支持 `--type=wiki`；必须用 `--member-kind` 指明成员角色，并且不能同时传 `--perm-type`。
- **错误处理**：OpenAPI 返回的 typed error 原样透传，可根据错误信封中的 subtype、code、hint 和权限信息处理。
