
# slides +create（创建飞书幻灯片）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一个新的飞书幻灯片演示文稿，可选一步添加页面内容。

## 命令

```bash
# 创建空白 PPT
lark-cli slides +create --title "项目汇报"

# 创建 PPT + 添加 slide 页面
lark-cli slides +create --title "项目汇报" --slides '[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>封面</p></content></shape></data></slide>",
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>第二页</p></content></shape></data></slide>"
]'

# 以应用身份创建（自动授权当前用户）
lark-cli slides +create --title "项目汇报" --as bot

# 预览（不执行）
lark-cli slides +create --title "项目汇报" --slides '[...]' --dry-run
```

## 返回值

工具成功执行后，返回一个 JSON 对象，包含以下字段：

- **`xml_presentation_id`**（string）：演示文稿的唯一标识符，后续添加页面时需要此 ID
- **`title`**（string）：演示文稿标题
- **`url`**（string，可选）：演示文稿的在线链接，如有返回则务必展示给用户（需要 drive 相关权限；若获取失败则不返回此字段）
- **`revision_id`**（integer）：演示文稿版本号
- **`slide_ids`**（string[]，可选）：仅传 `--slides` 时返回，成功添加的页面 ID 列表
- **`slides_added`**（integer，可选）：仅传 `--slides` 时返回，成功添加的页面数量
- **`images_uploaded`**（integer，可选）：仅 `--slides` 中含 `@<本地路径>` 占位符时返回，已上传的去重后图片数量
- **`permission_grant`**（object，可选）：仅 `--as bot` 时返回，说明是否已自动为当前 CLI 用户授予可管理权限

> [!IMPORTANT]
> 不传 `--slides` 时，`slides +create` 只创建空白演示文稿。创建后需要使用 `xml_presentation.slide create` 逐页添加 slide 内容。
>
> 传了 `--slides` 时，CLI 先创建空白演示文稿，再逐页调用 `xml_presentation.slide create` 添加页面。如果某一页添加失败，CLI 会停止并报错，已创建的演示文稿和已添加的页面会保留。
>
> 如果演示文稿是**以应用身份（bot）创建**的，如 `lark-cli slides +create --as bot`，CLI 会**尝试为当前 CLI 用户自动授予该演示文稿的 `full_access`（可管理权限）**。
>
> 以应用身份创建时，结果里会额外返回 `permission_grant` 字段，明确说明授权结果：
> - `status = granted`：当前 CLI 用户已获得该演示文稿的可管理权限
> - `status = skipped`：本地没有可用的当前用户 `open_id`，因此不会自动授权
> - `status = failed`：演示文稿已创建成功，但自动授权用户失败
>
> **不要擅自执行 owner 转移。** 如果用户需要把 owner 转给自己，必须单独确认。

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--title` | 否 | 演示文稿标题（不传则默认 "Untitled"） |
| `--slides` | 否 | slide 内容 JSON 数组，每个元素是一个 `<slide>` XML 字符串（最多 10 个；超过 10 页请先用 `+create` 创建空白 PPT，再用 `xml_presentation.slide create` 逐页添加） |

## `--slides` 参数格式

```json
[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\">...第1页XML...</slide>",
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\">...第2页XML...</slide>"
]
```

JSON string 数组，每个元素是一页 slide 的完整 XML。CLI 内部负责包装成 API 所需的 `{"slide": {"content": "..."}}` 格式并逐页调用。

### 本地图片：`@<path>` 占位符

`<img>` 元素的 `src` 属性如果以 `@` 开头，CLI 会把它当作本地文件路径，自动上传到当前演示文稿，并把占位符替换为返回的 `file_token`。

```bash
lark-cli slides +create --as user --title "图测试" --slides '[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><img src=\"@./assets/chart.png\" topLeftX=\"100\" topLeftY=\"100\" width=\"320\" height=\"180\"/></data></slide>"
]'
```

行为：

- 路径相对于**当前工作目录**（CWD）解析；**必须是 CWD 内的相对路径**（如 `./pic.png`、`./assets/x.png`）
- 同一份图被多次引用时**只上传一次**（按路径去重）
- `src` 不以 `@` 开头的会原样保留，但**只允许写 `slides +media-upload` 拿到的 `file_token`**；**禁止写 http(s) 外链 URL**：飞书 slides 渲染端不会代理外链图片，外链 src 通常显示破图。要用网图必须先下载到 CWD 内、再走上传流程
- 单张图片最大 20 MB（slides upload API 不支持分片上传）
- 校验阶段就会检查所有占位符文件存在及大小；缺文件或超限直接报错，不会创建空白 PPT 占位
- 创空白 PPT → 上传所有图 → 替换 token → 逐页创建 slide，按这个顺序执行

> [!IMPORTANT]
> **路径必须在 CWD 内**：`@/abs/path/x.png` 或 `@../up/x.png` 这种会被 CLI 拒绝（报 `unsafe file path`）。如果素材在别的目录，先 `cd` 过去再执行。

### 给已有 PPT 加带图新页

`+create --slides` 只在新建 PPT 时使用 `@` 占位符。给已有 PPT 加带图新页要分两步（CLI 没封装这个组合）：

```bash
# 1) 上传图片
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png --presentation $PRES_ID | jq -r .data.file_token)

# 2) 用返回的 file_token 创建带图新页
lark-cli slides xml_presentation.slide create --as user \
  --params "{\"xml_presentation_id\":\"$PRES_ID\"}" \
  --data "{\"slide\":{\"content\":\"<slide xmlns=\\\"http://www.larkoffice.com/sml/2.0\\\"><data><img src=\\\"$TOKEN\\\" topLeftX=\\\"100\\\" topLeftY=\\\"100\\\" width=\\\"200\\\" height=\\\"200\\\"/></data></slide>\"}}"
```

## 创建后续步骤

如果没有使用 `--slides`，`slides +create` 返回的 `xml_presentation_id` 用于后续操作：

```bash
# 第 1 步：创建空白 PPT
PRES_ID=$(lark-cli slides +create --title "项目汇报" | jq -r '.data.xml_presentation_id')

# 第 2 步：添加页面（使用返回的 xml_presentation_id）
lark-cli slides xml_presentation.slide create --as user \
  --params "{\"xml_presentation_id\":\"$PRES_ID\"}" \
  --data '{
    "slide": {
      "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\">...</slide>"
    }
  }'
```

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 400 | 参数错误 | 检查参数格式是否正确 |
| 403 | 权限不足 | 检查是否拥有 `slides:presentation:create` 和 `slides:presentation:write_only` scope |

## 相关命令

- [xml_presentation.slide create](lark-slides-xml-presentation-slide-create.md) — 添加幻灯片页面
- [xml_presentations get](lark-slides-xml-presentations-get.md) — 读取 PPT 内容
