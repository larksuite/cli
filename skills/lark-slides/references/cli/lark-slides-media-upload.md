# slides +media-upload（上传本地图片到飞书幻灯片）

把本地图片上传到指定演示文稿的 drive 媒体库，返回 `file_token`。**返回的 token 作为 `<img src="...">` 的值塞进 slide XML 即可显示图片。**

## 命令

```bash
# 直接传 xml_presentation_id
lark-cli slides +media-upload --as user \
  --file ./pic.png \
  --presentation slidesXXXXXXXXXXXXXXXXXXXXXX

# 传 slides URL 也行
lark-cli slides +media-upload --as user \
  --file ./chart.png \
  --presentation "https://xxx.feishu.cn/slides/slidesXXXXXXXXXXXXXXXXXXXXXX"

# 传 wiki URL（CLI 自动 wiki.spaces.get_node 解析为真实 token，校验 obj_type=slides）
lark-cli slides +media-upload --as user \
  --file ./pic.png \
  --presentation "https://xxx.feishu.cn/wiki/wikcnXXXXXX"

# 预览（不实际上传）
lark-cli slides +media-upload --file ./pic.png --presentation $PRES_ID --dry-run
```

## 返回值

```json
{
  "file_token": "boxcnXXXXXXXXXXXXXXXXXXXXXX",
  "file_name": "pic.png",
  "size": 12345,
  "presentation_id": "slidesXXXXXXXXXXXXXXXXXXXXXX"
}
```

- **`file_token`**：把它写进 `<img src="...">`
- **`file_name` / `size`**：上传文件元信息
- **`presentation_id`**：解析后的真实 `xml_presentation_id`（wiki URL 解析后会变化）

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file` | 是 | 本地图片路径，**必须是 CWD 内的相对路径**（如 `./pic.png`）。**最大 20 MB**（媒体上传不支持分片）。**仅支持 png / jpeg / gif / bmp / tiff / webp** |
| `--presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL |

> [!IMPORTANT]
> **路径必须在 CWD 内**：`--file /abs/path/x.png` 或 `--file ../up/x.png` 会被 CLI 拒绝（报 `unsafe file path`）。如果素材在别的目录，先 `cd` 过去再执行。

## 使用流程

> 新建 PPT（[`+create --slides`](lark-slides-create.md)）或给已有 PPT 加新页（[`+add-slide`](lark-slides-add-slide.md)）都不需要单独上传：XML 里把 `<img src>` 写成 `@<本地路径>`，CLI 会自动上传并替换成 `file_token`。
> 本命令用于往**已有页**里加图，或需要自己拿着 `file_token` 拼 XML 的场景。

### 给已有 PPT 的已有页加图

拿到 `file_token` 后走 [`+replace-slide`](lark-slides-replace-slide.md) 的 `block_insert`，不用搬原 XML、不改 `slide_id`、不打乱页序：

```bash
PRES_ID=xxx
SID=yyy       # 要加图的那一页

# 1) 上传图片拿 file_token
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png --presentation $PRES_ID --jq '.data.file_token')

# 2) block_insert 到页末（或用 insert_before_block_id 指定插入位置）
lark-cli slides +replace-slide --as user \
  --presentation "$PRES_ID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")}]')"
```

注意事项：

1. **`<img>` 坐标避开现有元素** —— 先读现有元素 bbox 挑空白区；空间不够就先用 `block_replace` 挪动/缩小现有元素后再放图
2. **`<img>` 的 `width:height` 对齐原图比例** —— 比例不一致会被裁剪，参见 [xml-schema-quick-ref.md](../xml/xml-schema-quick-ref.md) `<img>` 说明

## 上传约束

`+media-upload` 会处理 Slides 所需的媒体归属参数；调用者只需传入 `--file` 和 `--presentation`。单张图片最大 20 MB。

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 1061002 | params error / 不支持的 parent_type | 使用 `+media-upload`；它会采用 Slides 所需的 `parent_type` |
| 1061004 | forbidden：当前身份对该演示文稿无编辑权限 | 确认当前身份（user 或 bot）对目标 PPT 有编辑权限。bot 模式常见原因：PPT 不是该 bot 创建的——可用 `+create --as bot` 新建，或以 user 身份执行 `lark-cli drive +member-add --token "$PRES_ID" --type slides --member-id "$BOT_OPEN_ID" --member-type openid --perm full_access --yes` 给 bot 授权 |
| 1061044 | parent node not exist | `--presentation` 给的 token 不对，或不是 slides 类型 |
| 403 | 权限不足 | 检查 `docs:document.media:upload` scope；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- [+create](lark-slides-create.md) — 新建 PPT（支持 `@` 占位符自动上传图片）
- [+replace-slide](lark-slides-replace-slide.md) — 给已有页加图 / 换图（`block_insert` / `block_replace`）
- [+add-slide](lark-slides-add-slide.md) — 追加/插入单页（同样支持 `@` 占位符自动上传）
