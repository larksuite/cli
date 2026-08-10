
# drive +copy

> **认证与确认：** 普通复制直接执行本页命令。用户明确要求检查登录态或当前身份，或命令返回认证、授权、scope、权限、`1061005 auth failed` 或 `confirmation_required` 时，再阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

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

## 按标题复制并在 Docx 副本开头插入文本

1. 按标题定位源文件时，先按 [`lark-drive-search.md`](lark-drive-search.md) 使用 `drive +search` 得到唯一匹配的源资源。
2. 使用已确认的源 URL，或真实 token 与 type，执行一次 `drive +copy`。目标位置使用用户给出的文件夹 URL/token；复制到“我的空间”或未指定其他目标位置时使用 `--folder-token my_space`。
3. 从成功响应的 `data.file_token` 或 `data.url` 取得新副本；后续只操作该副本，不重新搜索副本，也不操作源 token。
4. 在 Docx 副本开头插入已知内容时，直接使用文档开头的固定锚点，无需读取正文或获取真实 block ID：

   ```bash
   lark-cli docs +update --doc "<COPY_FILE_TOKEN>" --command block_insert_after --block-id 0 --content '<p>要插入的内容</p>'
   ```

5. 更新返回 `data.result=success` 且 `data.warnings` 为空时任务完成。只有用户明确要求验证、返回 `partial_success`、`data.warnings` 非空，或后续编辑需要当前正文结构时，才读取副本。
6. 如果复制后的编辑失败，保留 `data.file_token`，只重试编辑步骤；不要重新执行 `drive +copy`，避免创建重复副本。

上述固定路径仅适用于“新复制的 Docx + 在开头插入已知内容”。替换已有内容、按章节修改、保留复杂结构或需要真实 block ID 时，进入 `lark-doc` 的通用更新流程。

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

- `invalid token`、`not found`、`unsupported type` 等确定性资源错误：如实报告并停止。
- 参数校验错误：按本页公开参数和命令返回的结构化错误修正输入，继续使用 `drive +copy`。
- timeout、connection reset 或 5xx 发生在请求可能已送达服务端之后：结果视为不确定，不自动再次复制；向用户说明重复执行可能产生第二个副本，并在确认后再处理。

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-wiki](../../lark-wiki/SKILL.md) -- 知识库节点复制（`wiki +node-copy`）
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
