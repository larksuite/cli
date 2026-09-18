# slides +replace-slide（块级替换 / 插入）

对指定页面的已知块做替换或插入。先用 `+xml-get --slide-id` 获取最新 `block_id`，再用 `+replace-slide` 写入；该操作不改变页面顺序。

```bash
lark-cli slides +replace-slide --as user \
  --presentation "$PRES_ID" --slide-id "$SID" \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\" fontSize=\"32\"><p>新标题</p></content></shape>"}]'
```

`block_replace` 使用 `block_id` 和 `replacement`；追加元素使用 `block_insert` 和 `insertion`。完整 parts 结构、验证和限制见 [lark-slides-replace-slide.md](lark-slides-replace-slide.md)。
