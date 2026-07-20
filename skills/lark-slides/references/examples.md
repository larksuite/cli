# 完整操作示例

本文档提供与 CLI schema 一致的调用示例，XML 内容均遵循 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)。

> **重要**：新建 PPT 请使用 `slides +create --slides`，传入由 `<slide>` XML 字符串组成的 JSON 数组；每个元素必须是一页完整的 `<slide>`。复杂内容建议先创建空白 PPT，再通过 `xml_presentation.slide.create` 逐页添加。完整 `<presentation>` XML 可用于本地 lint 或读取，但不能直接作为 `+create` 的提交参数。

## 目录

- [示例 1：可靠创建 6 页 PPT](#示例-1可靠创建-6-页-ppt)
- [示例 7: +replace-slide + block_insert 给已有页加图](#示例-7-replace-slide--block_insert-给已有页加图)
- [示例 8: +replace-slide + block_replace 替换一个块](#示例-8-replace-slide--block_replace-替换一个块)

## 示例 1：可靠创建 6 页 PPT

### 1. 写入规划文件

```bash
DECK_DIR=".lark-slides/plan/reliable-six-page-ppt"
mkdir -p "$DECK_DIR"

# 按 planning-layer.md 写入 "$DECK_DIR/slide_plan.json"，
# 至少记录 6 页的顺序和标题。
```

### 2. 为每页保存独立 XML

每个文件都是完整的 `<slide>`。下面的循环会生成 6 个独立 XML 文件；实际项目中可将每页主体替换为规划内容。

```bash
titles=("主题与结论" "问题背景" "核心方法" "关键数据" "执行计划" "总结与行动")
for i in {1..6}; do
  printf -v page '%02d' "$i"
  cat > "$DECK_DIR/slide-$page.xml" <<XML
<slide xmlns="http://www.larkoffice.com/sml/2.0"><style><fill><fillColor color="rgb(248,250,252)"/></fill></style><data><shape type="rect" topLeftX="56" topLeftY="56" width="12" height="428"><fill><fillColor color="rgb(37,99,235)"/></fill></shape><shape type="text" topLeftX="100" topLeftY="160" width="760" height="90"><content textType="title" autoFit="normal-auto-fit"><p>${titles[$((i-1))]}</p></content></shape><shape type="text" topLeftX="100" topLeftY="290" width="700" height="70"><content textType="body" autoFit="normal-auto-fit"><p>页面主体内容。</p></content></shape></data></slide>
XML
done
```

### 3. 逐页运行 lint

提交前检查每个独立 XML。`summary.error_count` 必须为 `0`，否则先修复 XML 或布局问题。

```bash
for slide_xml in "$DECK_DIR"/slide-0{1,2,3,4,5,6}.xml; do
  python3 skills/lark-slides/scripts/xml_text_overlap_lint.py \
    --input "$slide_xml" | tee "${slide_xml%.xml}.lint.json"
done

test "$(jq -s 'map(.summary.error_count) | add' "$DECK_DIR"/slide-0{1,2,3,4,5,6}.lint.json)" = "0"
```

### 4. 使用 `+create` 创建 6 页 PPT

`--slides` 接收由 6 个完整 `<slide>` XML 字符串组成的 JSON 数组；使用 `jq --rawfile` 避免手动处理 XML 引号和换行。

```bash
lark-cli slides +create --as user \
  --title "可靠创建 6 页 PPT" \
  --slides "$(jq -n \
    --rawfile s1 "$DECK_DIR/slide-01.xml" \
    --rawfile s2 "$DECK_DIR/slide-02.xml" \
    --rawfile s3 "$DECK_DIR/slide-03.xml" \
    --rawfile s4 "$DECK_DIR/slide-04.xml" \
    --rawfile s5 "$DECK_DIR/slide-05.xml" \
    --rawfile s6 "$DECK_DIR/slide-06.xml" \
    '[$s1, $s2, $s3, $s4, $s5, $s6]')" \
  > "$DECK_DIR/create.json"
create_status=$?

if [ "$create_status" -ne 0 ]; then
  exit "$create_status"
fi

if ! PRESENTATION_ID=$(jq -er '.data.xml_presentation_id | strings | select(length > 0)' "$DECK_DIR/create.json"); then
  echo "missing non-empty data.xml_presentation_id in $DECK_DIR/create.json" >&2
  exit 1
fi
echo "$PRESENTATION_ID" > "$DECK_DIR/xml_presentation_id"
```

如果创建中途失败，先保存已经返回的 `xml_presentation_id`，再回读确认实际已创建页数。

### 5. 用 `+xml-get` 回读全文 XML

```bash
lark-cli slides +xml-get --as user \
  --presentation "$PRESENTATION_ID" \
  --output "$DECK_DIR/readback.xml" \
  --json | tee "$DECK_DIR/readback.json"
```

