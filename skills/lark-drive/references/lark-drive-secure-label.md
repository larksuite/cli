# drive +secure-label-list / +secure-label-update（云文档密级标签）

## 何时使用

- `drive +secure-label-list`：查询当前用户可用的密级标签，先拿到目标 `id`。
- `drive +secure-label-update`：把目标云文档调整为指定密级标签。

这两个 shortcut 都使用用户身份（`--as user`）。修改密级前，通常先执行 `+secure-label-list` 确认可用标签 ID。

## 查询可用密级标签

```bash
lark-cli drive +secure-label-list --page-size 10 --lang zh
```

可选参数：

| 参数 | 说明 |
|------|------|
| `--page-size` | 分页大小，范围 `1..10`，默认 `10` |
| `--page-token` | 上一页响应里的 `page_token` |
| `--lang` | 标签语言：`zh`、`en`、`ja` |

底层接口：`GET /open-apis/drive/v2/my_secure_labels`。

### 将密级描述解析为标签 ID

当需要把用户描述的密级解析为标签 ID 时：

1. 使用后续命令所用的同一 user 身份和 profile 调用 `drive +secure-label-list`，并保持语言一致。
2. 如果 `has_more=true`，携带返回的 `page_token` 继续查询；完成全部分页后才能判断没有匹配。
3. 先按完整展示名匹配；用户明确给出等级 token（例如 `L4`）时，再以大小写不敏感、边界清晰的独立等级 token 匹配返回的 `name`。不要用宽泛包含关系猜测标签。
4. 恰好一个候选时使用其数字 `id`；没有候选或存在多个候选时，向用户列出可见的标签名称和 ID 并请求确认，不要继续调用下游命令。

标签列表属于当前用户可见的运行时数据，不要硬编码名称或 ID，也不要把展示名当作标签 ID 传给下游命令。

## 修改文档密级

```bash
lark-cli drive +secure-label-update \
  --token "https://example.feishu.cn/docx/doxcnxxxx" \
  --label-id '<label-id>' # replace $LABEL_ID before running
```

参数：

| 参数 | 说明 |
|------|------|
| `--token` | 目标文档 URL 或 bare token；URL 可自动推断 `--type` |
| `--type` | bare token 必填；URL 输入时可省略。可选：`doc`、`docx`、`sheet`、`file`、`bitable`、`mindnote`、`slides` |
| `--label-id` | 要设置的密级标签 ID |

底层接口：`PATCH /open-apis/drive/v2/files/:file_token/secure_label`，query 参数 `type`，请求体 `{ "id": "<label-id>" }`。

## 错误处理

CLI 不会在 shortcut 中为密级错误码追加专用 hint；agent 必须根据返回的 `error.code` 做以下引导。

| 错误码 | 含义 | 引导 |
|--------|------|------|
| `1063013` | 密级降级需要审批 | 提示用户打开目标文档，在文档界面完成密级降级审批后重试；如果用户传入的是文档 URL，必须把该 URL 一并给用户作为操作入口 |

遇到 `1063013` 时，不要继续重试 API，也不要提示补 scope；这是文档侧审批流程要求，需要用户到文档里操作。
