
# drive +copy

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

复制一个 Drive 文件（在线文档、表格、多维表格、幻灯片、思维笔记或普通文件）到目标文件夹，生成一个内容相同的新副本。

## 命令

```bash
# 源文档传 URL（自动识别类型和 token）
lark-cli drive +copy --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --name '副本名称' --folder-token <TARGET_FOLDER_TOKEN>

# Wiki URL（自动解包底层资源后复制到 Drive）
lark-cli drive +copy --url "https://example.larksuite.com/wiki/<WIKI_TOKEN>" --name '副本名称' --folder-token <TARGET_FOLDER_TOKEN>

# Wiki token
lark-cli drive +copy --token <WIKI_TOKEN> --type wiki --name '副本名称' --folder-token my_space
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 与 `--token` 二选一 | 源文档 URL，支持 `doc` / `docx` / `sheet` / `file` / `mindnote` / `slides` / `base` / `bitable` / `wiki` 路径；wiki 会自动解包底层资源 |
| `--token` | 与 `--url` 二选一 | 源文档 token 或 URL；裸 token 必须配合 `--type` |
| `--type` | 裸 token 时必填 | 源文件类型：`doc`、`docx`、`sheet`、`file`、`mindnote`、`slides`、`bitable`（`base` 为兼容别名）或 `wiki`；传 URL 时可省略，显式传入时必须与 URL 类型一致 |
| `--name` | 是 | 副本名称，最长 256 字节 |
| `--folder-token` | 是 | 目标文件夹 token、文件夹 URL，或常量 `my_space`（复制到当前身份"我的空间"根目录，内部自动解析根 token） |
| `--extra` | 否 | 可重复的 `key=value` 对，原样透传给 API 的 `extra` 自定义复制参数；典型用法 `--extra target_type=docx`（复制旧版 doc 时转换为 docx 副本） |

## 输入规则

- `--url` 与 `--token` 互斥，只传一个
- `--type` 必须与源文件真实类型一致，类型不匹配时服务端会返回失败
- `base` 与 `bitable` 是同一概念，CLI 会把 `base` 归一化为 `bitable` 后发给服务端
- 目标文件夹必须是云空间（云盘/云存储）文件夹 token，不能传 wiki 节点 token

## Wiki 场景

`drive +copy` 接受 wiki URL，也接受 `--token <WIKI_TOKEN> --type wiki`。目标仅支持云盘（Drive）文件夹或 `my_space` 根目录；要把副本留在知识库中，使用 `wiki +node-copy`。

## 行为说明

- bot 身份复制成功后，CLI 会自动尝试给当前 CLI 用户授予新副本的 `full_access`，结果在输出的 `data.permission_grant` 字段中；授权失败不影响复制本身的成功状态

## 输出

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "copied": true,
    "file_token": "<new_file_token>",
    "file_type": "docx",
    "name": "副本名称",
    "url": "https://example.larksuite.com/docx/<new_file_token>",
    "source_file_token": "<source_file_token>",
    "source_type": "docx",
    "source_wiki_token": "<source_wiki_token, only for wiki input>",
    "folder_token": "<target_folder_token>",
    "permission_grant": {
      "status": "granted",
      "perm": "full_access",
      "member_type": "openid",
      "user_open_id": "<current_user_open_id>",
      "message": "Granted the current CLI user full_access on the new document."
    }
  }
}
```

`source_wiki_token` 仅 wiki 输入出现；`permission_grant` 仅 bot 身份出现，user 身份复制时 `data` 下没有该字段。

## 常见错误

| 错误码 | 含义 | 处理 |
|---|---|---|
| `99991672` / `99991679` | 缺失 scope | 按错误里的 `missing_scopes`、`hint` 申请/授权所需 scope 后重试 |
| `99991400` | 命中接口限频 | 等待一段时间后重试；批量复制时保持串行并降低频率 |

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-wiki](../../lark-wiki/SKILL.md) -- 知识库节点复制（`wiki +node-copy`）
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
