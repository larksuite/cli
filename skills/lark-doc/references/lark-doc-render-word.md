# DOCX 渲染 PDF

用服务端 Word 渲染链路把本地 `.docx` 转成 PDF。该链路保留物理分页，并尽可能返回 Heading 1–9 对应的 PDF 页码。

## 创建、等待并下载

```bash
lark-cli docs +render-word \
  --file ./report.docx \
  --output ./report.pdf \
  --as user
```

- 输入必须是普通 `.docx` 文件，非空且不超过 20 MiB；加密、OLE、损坏或不安全的 ZIP/XML 会被服务端拒绝。
- 命令创建异步任务，并在最多 600 秒内按服务端建议间隔轮询。`--wait-timeout-seconds 0` 可只创建任务、不等待。
- `--if-exists` 默认为 `error`；需要覆盖或保留两个副本时显式选择 `overwrite` 或 `rename`。
- `--idempotency-key` 省略时由 CLI 生成。同一 key 与同一文件会复用任务；把同一 key 用于不同文件会失败。

成功结果包含绝对 `output_path`、`downloaded_size`、`page_count`、`headings` 和 `warnings`。标题页码无法确定时 PDF 仍可成功，相关标题没有 `page_index` / `page_number`，并返回 warning；不要把 warning 当作转换失败。

## 超时后恢复

等待超时是可恢复状态，不表示渲染失败。保存输出中的 `task_id`，稍后只查询一次：

```bash
lark-cli docs +render-word-status \
  --task-id render_xxx \
  --output ./report.pdf \
  --as user
```

- `processing`：保留 `task_id`，稍后重试；不要重新上传同一文件。
- `succeeded`：带 `--output` 时安全下载 PDF；不带时只返回任务和短时下载信息。
- `failed` / `expired`：命令返回 typed error。失败后用新的 idempotency key 重建任务；过期任务不能恢复。

## 安全边界

- 仅支持 user 身份。
- PDF 下载 URL 短时有效。CLI 会校验 HTTPS 目标和所有重定向，不向预签名 URL 附加 Lark Authorization，并在保存前校验 `%PDF-` 文件头。
- 服务可用时使用该链路。只有服务不可用且用户明确要求本地转换时，才采用显式的本地 LibreOffice fallback；不得静默切换。
