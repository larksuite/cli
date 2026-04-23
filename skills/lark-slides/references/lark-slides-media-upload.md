
# slides +media-upload（上传本地图片到飞书幻灯片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

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
| `--file` | 是 | 本地图片路径，**必须是 CWD 内的相对路径**（如 `./pic.png`）。**最大 20 MB**（slides upload API 不支持分片上传） |
| `--presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL |

> [!IMPORTANT]
> **路径必须在 CWD 内**：`--file /abs/path/x.png` 或 `--file ../up/x.png` 会被 CLI 拒绝（报 `unsafe file path`）。如果素材在别的目录，先 `cd` 过去再执行。

## 使用流程

### 给已有 PPT 加带图新页

```bash
# 1) 上传图片
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png \
  --presentation $PRES_ID | jq -r .data.file_token)

# 2) 用 file_token 创建带图新页
lark-cli slides xml_presentation.slide create --as user \
  --params "{\"xml_presentation_id\":\"$PRES_ID\"}" \
  --data "{\"slide\":{\"content\":\"<slide xmlns=\\\"http://www.larkoffice.com/sml/2.0\\\"><data><img src=\\\"$TOKEN\\\" topLeftX=\\\"100\\\" topLeftY=\\\"100\\\" width=\\\"320\\\" height=\\\"180\\\"/></data></slide>\"}}"
```

### 新建带图 PPT（推荐用 `+create --slides` 的 `@` 占位符，一步到位）

```bash
# 不需要单独 +media-upload，写 src="@<本地路径>" 即可
lark-cli slides +create --as user --title "图测试" --slides '[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><img src=\"@./pic.png\" topLeftX=\"100\" topLeftY=\"100\" width=\"320\" height=\"180\"/></data></slide>"
]'
```

详见 [+create 文档](lark-slides-create.md#本地图片path-占位符)。

### 给已有 PPT 的已有页加图

拿到 `file_token` 后走 [`+replace-slide`](lark-slides-replace-slide.md) 的 `block_insert`，不用搬原 XML、不改 `slide_id`、不打乱页序：

```bash
PRES_ID=xxx
SID=yyy       # 要加图的那一页

# 1) 上传图片拿 file_token
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png --presentation $PRES_ID | jq -r '.data.file_token')

# 2) block_insert 到页末（或用 insert_before_block_id 指定插入位置）
lark-cli slides +replace-slide --as user \
  --presentation "$PRES_ID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")}]')"
```

注意事项：

1. **`<img>` 坐标避开现有元素** —— 先读现有元素 bbox 挑空白区；空间不够就先用 `block_replace` 挪动/缩小现有元素后再放图
2. **`<img>` 的 `width:height` 对齐原图比例** —— 比例不一致会被裁剪，参见 [xml-schema-quick-ref.md](xml-schema-quick-ref.md) `<img>` 说明

## 工作原理

`+media-upload` 内部调用 `POST /open-apis/drive/v1/medias/upload_all`（单次上传，最大 20 MB），固定使用：

- `parent_type=slide_file`（slides 后端唯一接受的取值，已实测验证）
- `parent_node=<xml_presentation_id>`

**不要尝试用 `slides_image`、`slide_image` 等 parent_type**——后端会返回 1061001 / 1061002 错误。这是 slides 的特殊约定。

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 1061002 | params error / 不支持的 parent_type | 不要用原生 API 自己拼 parent_type；用 `+media-upload` 即可 |
| 1061004 | forbidden：当前身份对该演示文稿无编辑权限 | 确认当前身份（user 或 bot）对目标 PPT 有编辑权限。bot 模式常见原因：PPT 不是该 bot 创建的——可用 `+create --as bot` 新建，或以 user 身份给 bot 授权 `lark-cli drive permission.members create --as user ...` |
| 1061044 | parent node not exist | `--presentation` 给的 token 不对，或不是 slides 类型 |
| 403 | 权限不足 | 检查 `docs:document.media:upload` scope；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- [+create](lark-slides-create.md) — 新建 PPT（支持 `@` 占位符自动上传图片）
- [+replace-slide](lark-slides-replace-slide.md) — 给已有页加图 / 换图（`block_insert` / `block_replace`）
- [xml_presentation.slide create](lark-slides-xml-presentation-slide-create.md) — 创建 slide 页面（拿到 file_token 后塞进 XML）
