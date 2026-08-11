# slides +delete-slide（按 slide_id 删除单页）

从演示文稿删除**一页**，按 `slide_id` 指定。只改一页里的局部内容用 [`+replace-slide`](lark-slides-replace-slide.md)，不要删了重建。

`--presentation` 接受 token / `/slides/` URL / `/wiki/` URL，ID 是普通 flag 而不是 `--params` JSON 串。

> `--slide-id` 只接受单个 ID —— 不支持逗号分隔的列表（`+screenshot` 的 `--slide-id` 支持，这个不支持），也不支持按页号删。

## 命令

```bash
# 直接传 xml_presentation_id
lark-cli slides +delete-slide --as user \
  --presentation "$PID" \
  --slide-id "$SID"

# slides URL / wiki URL 都可以（wiki 会自动解析并校验 obj_type=slides）
lark-cli slides +delete-slide --as user \
  --presentation "https://xxx.feishu.cn/wiki/wikcnXXXXXX" \
  --slide-id "$SID"

# 删之前先确认打到哪份 PPT、哪一页
lark-cli slides +delete-slide --presentation "$PID" --slide-id "$SID" --dry-run
```

## 参数

| 参数 | 必需 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/` URL 或 `/wiki/` URL |
| `--slide-id` | 是 | 要删除的页面 ID |
| `--revision-id` | 否 | 演示文稿版本号，默认 `-1`（最新）；传具体版本号做乐观锁 |
| `--dry-run` | 否 | 打印将要发起的请求，不删除 |

## 成功输出

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id",
  "deleted": true,
  "revision_id": 43
}
```

## 怎么拿 `slide_id`

`slide_id` 是服务端短 ID，**不能从 XML 里推导**。两个来源：

1. `+create` / `+add-slide` 的返回值里存下来；
2. 事后回读：`slides +xml-get --presentation "$PID" --output .lark-slides/plan/<deck>/readback.xml`。

删错页的代价高于多跑一次回读 —— 不确定就先回读 + `+screenshot` 看一眼再删。

## 删错了怎么办

删除在原地不可撤销，但可以走历史版本回滚：`+history-list` 找 `history_version_id` → `+history-revert`（只接受 `history_version_id`，不能传 `revision_id`）→ `+history-revert-status` 轮询。命令用法见 [lark-slides-history.md](lark-slides-history.md)。

## 常见错误

| 现象 | 原因 | 解决 |
|------|------|------|
| `--slide-id cannot be empty` | 传了空串或纯空格 | 检查变量有没有取到值 |
| 3350001 `invalid param` | `slide_id` 写错或该页已被删 | `+xml-get` 回读确认 `slide_id` 还在 |
| 403 / 权限不足 | 当前身份对这份 PPT 没有编辑权限 | 检查是否拥有 `slides:presentation:update` 或 `slides:presentation:write_only` scope；wiki 链接另需 `wiki:node:read`；`--as bot` 还要求该 bot 对目标 PPT 有编辑权限 |
