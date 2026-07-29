# drive +permission-get-setting（查询权限设置）

本 skill 对应 shortcut：`lark-cli drive +permission-get-setting`。它读取单个 Drive 资源自身的公开访问、分享、协作者管理、安全与评论权限设置，不递归读取文件夹中的子资源。

## 命令

```bash
# 通过 URL 自动推断 type
lark-cli drive +permission-get-setting \
  --token 'https://example.feishu.cn/drive/folder/<folder_token>' \
  --as user --format json

# 通过 bare token 显式指定 type
lark-cli drive +permission-get-setting \
  --token '<folder_token>' \
  --type folder \
  --as user --format json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token` | 是 | bare token 或完整 URL。URL 路径支持 `/folder/`、`/docx/`、`/doc/`、`/sheets/`、`/base/`、`/bitable/`、`/wiki/`、`/file/`、`/mindnotes/`、`/slides/`、`/minutes/`、`/page/`。 |
| `--type` | bare token 必填 | 目标类型：`doc` / `sheet` / `file` / `wiki` / `bitable` / `docx` / `mindnote` / `minutes` / `slides` / `folder` / `apps`。URL 可自动推断；如果同时传 URL 和冲突的 `--type`，CLI 会拒绝。 |
| `--dry-run` | 否 | 只打印请求，不调用 API。 |

## 输出

JSON 输出中的 `data.permission_public` 是目标当前的权限设置；服务端未返回该字段时，命令会报响应结构错误，而不会把其他字段伪装成权限设置。

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "permission_public": {}
  }
}
```

`--format pretty` 会展示完整的 `permission_public` 对象，包括服务端将来新增的字段。

## 行为说明

- **身份支持**：`--as user` 和 `--as bot` 均可用。
- **所需 scope**：`docs:permission.setting:read`。
- **单目标读取**：命令只读取 `--token` 指向资源自身的权限设置；`--type folder` 不会递归读取子资源。
