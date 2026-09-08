# slides +xml-get（读取单页 XML）

读取指定页面的 XML。使用 `--slide-id` 获取页面及其顶层块的 `id`，供 `+replace-slide` 精确替换；需要按页码读取时改用 `--slide-number`。

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PRES_ID" --slide-id "$SID" --raw
```

不使用 `--raw` 时，JSON 输出的 `.data.revision_id` 可用于后续写入：

```bash
REV=$(lark-cli slides +xml-get --as user \
  --presentation "$PRES_ID" --slide-id "$SID" \
  --jq '.data.revision_id')
```

完整参数、全文读取和输出文件约束见 [lark-slides-xml-presentations-get.md](lark-slides-xml-presentations-get.md)。
