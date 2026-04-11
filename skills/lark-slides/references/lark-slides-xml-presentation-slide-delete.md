# lark-slides xml_presentation.slide delete

## 用途

删除指定 XML 演示文稿中的幻灯片页面。

## 命令

```bash
lark-cli slides xml_presentation.slide delete --as user --params '<json_params>'
```

## 参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `--params` | JSON string | 是 | 路径参数与查询参数 |

### params JSON 结构

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id",
  "revision_id": -1,
  "tid": "idMock"
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `xml_presentation_id` | string | 是 | 演示文稿的唯一标识符 |
| `slide_id` | string | 是 | 要删除的幻灯片唯一标识符 |
| `revision_id` | integer | 否 | 演示文稿版本号，`-1` 表示最新版本 |
| `tid` | string | 否 | 锁的事务 ID |

## 使用示例

### 删除指定幻灯片

```bash
lark-cli slides xml_presentation.slide delete --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id"
}'
```

### 结合查询删除（使用 jq）

```bash
# 先读取 XML 内容，确认待删除页面
lark-cli slides xml_presentations get --as user --params '{"xml_presentation_id":"slides_example_presentation_id"}' | jq -r '.xml_presentation.content'

# 然后按已知 slide_id 删除
lark-cli slides xml_presentation.slide delete --as user --params '{"xml_presentation_id":"slides_example_presentation_id","slide_id":"slide_example_id"}'
```

## 返回值

成功时返回删除确认信息：

```json
{
  "revision_id": 100
}
```

### 返回字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `revision_id` | integer | 删除后的最新版本号 |

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 404 | 演示文稿不存在 | 检查 `xml_presentation_id` 是否正确 |
| 404 | 幻灯片不存在 | 检查 `slide_id` 是否正确，或该幻灯片已被删除 |
| 400 | 无法删除唯一幻灯片 | 演示文稿至少保留一页幻灯片 |
| 403 | 权限不足 | 检查是否拥有 `slides:presentation:update` 或 `slides:presentation:write_only` scope |

## 注意事项

1. **执行前必做**: 使用 `lark-cli schema slides.xml_presentation.slide.delete` 查看最新的参数结构
2. **删除不可逆**: 删除操作无法撤销，请确保已备份重要内容
3. **至少保留一页**: 演示文稿必须至少保留一页幻灯片，删除最后一页会报错
4. **版本控制**: 如果依赖版本号并发控制，删除前先确认 `revision_id`
5. **获取 slide_id**: 创建幻灯片时请保存返回值；仅靠 `get` 返回的 XML 无法直接推导服务端 short ID

## 如何获取 slide_id

### 方法 1: 创建时保存

```bash
lark-cli slides xml_presentation.slide create --as user --params '{"xml_presentation_id":"slides_example_presentation_id"}' --data '{
  "slide": {
    "content": "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新页面</p></content></shape></data></slide>"
  }
}'
```

返回结果中的 `slide_id` 就是后续删除所需的值。

## 批量删除建议

如果需要删除多张幻灯片，建议先整理好待删 `slide_id` 列表，再逐个删除：

```bash
for slide_id in sld_a sld_b sld_c; do
  lark-cli slides xml_presentation.slide delete --as user --params "{\"xml_presentation_id\":\"slides_example_presentation_id\",\"slide_id\":\"$slide_id\"}"
done
```

## 相关命令

- [slides +create](lark-slides-create.md) - 创建空白 PPT
- [xml_presentations get](lark-slides-xml-presentations-get.md) - 读取 PPT 内容
- [xml_presentation.slide create](lark-slides-xml-presentation-slide-create.md) - 添加幻灯片页面
