# drive +member-list（查询协作者/授权成员列表）

本 skill 对应 shortcut：`lark-cli drive +member-list`。它读取 Drive 文档、文件、文件夹或 wiki 节点的协作者/授权成员列表。

## 命令

```bash
#  URL 自动推断 type
lark-cli drive +member-list \
  --token 'https://example.feishu.cn/drive/folder/<folder_token>' \
  --as user --format json

# 查询附加字段
lark-cli drive +member-list \
  --token '<token>' \
  --type docx \
  --fields 'name,type,external_label' \
  --as user --format json

```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token` | 是 | 裸 token 或完整 URL。URL 路径支持 `/folder/`、`/docx/`、`/doc/`、`/sheets/`、`/base/`、`/bitable/`、`/wiki/`、`/file/`、`/mindnotes/`、`/slides/`、`/minutes/`、`/page/`。 |
| `--type` | 裸 token 必填 | 目标类型：`doc` / `sheet` / `file` / `wiki` / `bitable` / `docx` / `mindnote` / `minutes` / `slides` / `folder` / `apps`。URL 可自动推断；如果同时传 URL 和冲突的 `--type`，CLI 会拒绝。 |
| `--fields` | 否 | 默认不传。可取 `name` / `type` / `avatar` / `external_label`，支持逗号分隔；也可传 `*` 请求当前支持的所有附加字段。该参数只声明期望返回的字段，不授予字段级权限。 |
| `--perm-type` | 否 | 仅 `--type wiki` 有效；取值 `container` / `single_page`。 |
| `--dry-run` | 否 | 只打印请求，不调用 API。 |

## 输出

JSON 输出原样透传 API 的 `data` ：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "items": [
      {
        "member_type": "openid",
        "member_id": "ou_xxx",
        "perm": "view",
        "perm_type": "container",
        "type": "user",
        "name": "zhangsan",
        "external_label": false
      }
    ]
  }
}
```

`--format pretty` 会轻量展示成员 ID、成员类型、权限、wiki `perm_type` 和已返回的附加字段。机器读取优先使用 `--format json`。

## 行为说明

- **身份支持**：`--as user` 和 `--as bot` 均可用；缺 scope 或目标权限时按统一 permission 错误路径处理。
- **接口 scope**：查询成员列表需要 `docs:permission.member:retrieve`。
- **fields 默认**：不传 `--fields` 时按官方 API 默认，不请求姓名、头像、外部标签等附加字段；需要时显式指定。
- **字段级权限**：`--fields` 只控制请求哪些附加字段，不保证服务端一定返回。请求用户的 `name` / `avatar` 时，应用还需开通 `contact:user.base:readonly`（“获取用户基本信息”；已具备官方兼容的历史通讯录权限也可满足要求）。
- **缺字段语义**：字段级权限或数据可见性不足时，接口仍可能成功，但会省略相应敏感字段。响应中缺少已请求字段表示“服务端未返回”，不能解释为字段值为空，也不能据此认定成员信息完整。
- **folder 支持**：CLI 支持 `--type folder` 并会按需求发送 `type=folder`；部分环境的后端如果尚未放开 folder 枚举，可能返回 `99992402 field validation failed`。
