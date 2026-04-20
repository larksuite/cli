# lark-slides xml_presentation.slide create

## 用途

在指定的 XML 演示文稿中创建新的幻灯片页面，通常用于给 `slides +create` 创建出的空白 PPT 逐页补充内容。

## 命令

```bash
lark-cli slides xml_presentation.slide create --as user --params '<json_params>' --data '<json_data>'
```

## 参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `--params` | JSON string | 是 | 路径参数与查询参数 |
| `--data` | JSON string | 是 | 请求体，包含新页面内容 |

### params JSON 结构

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "revision_id": -1,
  "tid": "idMock"
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `xml_presentation_id` | string | 是 | 目标演示文稿的唯一标识符 |
| `revision_id` | integer | 否 | 演示文稿版本号，`-1` 表示最新版本 |
| `tid` | string | 否 | 锁的事务 ID |

### data JSON 结构

```json
{
  "slide": {
    "slide_id": "slide_example_id",
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\">...</slide>"
  },
  "before_slide_id": "slide_before_target"
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `slide.slide_id` | string | 否 | 幻灯片页面 short ID |
| `slide.content` | string | 否 | 新幻灯片的 XML 内容 |
| `before_slide_id` | string | 否 | 插入到指定页面之前 |

## slide XML 结构

`slide.content` 是一个完整的 `<slide>` 元素，遵循 SML 2.0 Schema：

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <data>
    <shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
      <content textType="title">
        <p>标题</p>
      </content>
    </shape>
  </data>
</slide>
```

详细格式请参考 [xml-format-guide.md](xml-format-guide.md) 和 [xml-schema-quick-ref.md](xml-schema-quick-ref.md)。

## 使用示例

### 在末尾添加幻灯片

```bash
lark-cli slides xml_presentation.slide create --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新页面标题</p></content></shape><shape type=\"text\" topLeftX=\"80\" topLeftY=\"200\" width=\"800\" height=\"180\"><content textType=\"body\"><p>内容文本</p></content></shape></data></slide>"
  }
}'
```

### 在指定页面前插入幻灯片

```bash
lark-cli slides xml_presentation.slide create --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>插入的标题页</p></content></shape></data></slide>"
  },
  "before_slide_id": "slide_before_target"
}'
```

### 带图形元素的幻灯片

```bash
lark-cli slides xml_presentation.slide create --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id"
}' --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"520\" height=\"120\"><content textType=\"title\"><p>数据展示</p></content></shape><shape type=\"rect\" topLeftX=\"700\" topLeftY=\"100\" width=\"200\" height=\"150\"><fill><fillColor color=\"rgb(100, 149, 237)\"/></fill></shape></data></slide>"
  }
}'
```

### 从文件读取 XML

```bash
# 先创建 slide.xml 文件
cat > slide.xml << 'EOF'
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <data>
    <shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
      <content textType="title">
        <p>从文件加载</p>
      </content>
    </shape>
    <shape type="text" topLeftX="80" topLeftY="200" width="800" height="180">
      <content textType="body">
        <p>这是从文件读取的幻灯片内容</p>
      </content>
    </shape>
  </data>
</slide>
EOF

# 然后创建幻灯片
lark-cli slides xml_presentation.slide create --as user \
  --params '{"xml_presentation_id":"slides_example_presentation_id"}' \
  --data "$(jq -n --arg content "$(cat slide.xml)" '{slide:{content:$content}}')"
```

## 返回值

成功时返回创建的幻灯片信息：

```json
{
  "slide_id": "slide_example_id",
  "revision_id": 100
}
```

### 返回字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `slide_id` | string | 新幻灯片的唯一标识 |
| `revision_id` | integer | 演示文稿最新版本号 |

## slide 元素可用子元素

| 元素 | 说明 |
|------|------|
| `<style>` | 页面样式（背景填充） |
| `<data>` | 图形元素容器（shape、img、table、chart 等） |
| `<note>` | 演讲者备注 |

> [!IMPORTANT]
> **本地图片必须先上传**：`xml_presentation.slide.create` 不识别 `@./local.png` 占位符（那是 `+create --slides` 的语法糖）。直接调本接口添加带图新页时，必须先用 [`slides +media-upload`](lark-slides-media-upload.md) 拿到 `file_token`，再写进 `<img src="<file_token>">`。
>
> 如果是从零开始建带图 PPT，**强烈建议改用 [`slides +create --slides '[...]'`](lark-slides-create.md#本地图片path-占位符)** 一步搞定（自动上传 + 替换 token）。

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 404 | 演示文稿不存在 | 检查 `xml_presentation_id` 是否正确 |
| 400 | XML 格式错误 | 检查 `slide.content` 是否是完整 `<slide>` 元素 |
| 400 | 请求体结构错误 | 检查是否按 `slide.content` 和 `before_slide_id` 包装 |
| 403 | 权限不足 | 检查是否拥有 `slides:presentation:update` 或 `slides:presentation:write_only` scope |

## 注意事项

1. **执行前必做**: 使用 `lark-cli schema slides.xml_presentation.slide.create` 查看最新的参数结构
2. **slide.content 格式**: 必须是完整的 `<slide>` 元素，不是整个 presentation
3. **命名空间建议**: 协议标准写法应带 `xmlns`，例如 `<slide xmlns="http://www.larkoffice.com/sml/2.0">`；当前服务端实现可能兼容不带 `xmlns` 的输入，但不作为协议保证
4. **fill / border 写法**: 颜色填充使用 `<fill><fillColor color="..."/></fill>`，边框常用 `<border color="..." width="2"/>`
5. **插入位置**: 通过 `before_slide_id` 指定插入目标，而不是用 `position`
6. **JSON 转义**: 如果直接内联 XML，需要正确转义双引号
7. **建议**: 先使用 `xml_presentations.get` 获取现有结构，再添加新页面

## 批量添加建议

如果需要添加多张幻灯片，建议先明确每一页的 `before_slide_id`，或直接按最终顺序逐页追加：

```bash
#!/bin/bash

PRESENTATION_ID="slides_example_presentation_id"

declare -a slides=(
  '<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>页面 1</p></content></shape></data></slide>'
  '<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>页面 2</p></content></shape></data></slide>'
  '<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>页面 3</p></content></shape></data></slide>'
)

for slide_xml in "${slides[@]}"; do
  payload=$(jq -n --arg content "$slide_xml" '{slide:{content:$content}}')
  lark-cli slides xml_presentation.slide create --as user --params "{\"xml_presentation_id\":\"$PRESENTATION_ID\"}" --data "$payload"
done
```

## 相关命令

- [slides +create](lark-slides-create.md) - 创建空白 PPT
- [xml_presentations get](lark-slides-xml-presentations-get.md) - 读取 PPT 内容
- [xml_presentation.slide delete](lark-slides-xml-presentation-slide-delete.md) - 删除幻灯片页面
- [xml-format-guide.md](xml-format-guide.md) - XML 格式详细规范
- [xml-schema-quick-ref.md](xml-schema-quick-ref.md) - Schema 快速参考
