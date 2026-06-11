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

## 修改文档密级

```bash
lark-cli drive +secure-label-update \
  --token "https://example.feishu.cn/docx/doxcnxxxx" \
  --label-id "7217780879644737539"
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
