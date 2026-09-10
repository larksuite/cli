# slides +xml-get（读取演示文稿 XML）

读取全文或单页 XML。全文验证优先将结果保存到本地文件；局部编辑可读取单页 XML，从顶层块的 `id` 属性取得 `+replace-slide` 所需的 `block_id`。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、Slides URL，或可解析为 Slides 的 wiki URL |
| `--revision-id` | 否 | 版本号；`-1` 表示最新版本 |
| `--output` | 否 | XML 保存路径，必须使用 CWD 内相对路径；省略时返回 JSON |
| `--raw` | 否 | 将 XML 直接输出到 stdout；不能与 `--output` 或 `--jq` 一起使用 |
| `--slide-id` | 否 | 只读取指定页面；不能与 `--slide-number` 或 `--remove-attr-id` 一起使用 |
| `--slide-number` | 否 | 只读取指定的 1-based 页码；不能与 `--slide-id` 或 `--remove-attr-id` 一起使用 |
| `--remove-attr-id` | 否 | 仅全文读取可用；移除 XML `id` 属性，不适合后续精确块编辑 |

## 示例

```bash
# 读取全文并保存，用于创建后验证
lark-cli slides +xml-get --as user \
  --presentation "$PRES_ID" \
  --output .lark-slides/plan/<deck>/readback.xml

# 读取单页以获取 block_id
lark-cli slides +xml-get --as user \
  --presentation "$PRES_ID" --slide-id "$SID" --raw

# 读取单页，同时记录 revision_id 用于后续乐观锁
REV=$(lark-cli slides +xml-get --as user \
  --presentation "$PRES_ID" --slide-id "$SID" \
  --jq '.data.revision_id')
```

JSON 输出中，全文 XML 位于 `.data.xml_presentation.content`，单页 XML 位于 `.data.slide.content`；二者的 `.data.revision_id` 都可用于后续写操作。

相关命令：

- [slides +replace-slide](lark-slides-replace-slide.md) — 块级替换 / 插入
- [slides +update-slide](lark-slides-update-slide.md) — 整页覆盖
