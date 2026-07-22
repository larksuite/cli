# Validation Checklist

创建或大幅改写演示文稿后，必须做一次显式验证。目标是发现空白页、XML 损坏、内容截断、明显溢出、弱视觉层级和未验证输出。

小型已有页编辑也要做对应范围的验证：至少读取被改页面或全文 XML，确认目标元素已更新且未破坏周边结构。

## Required Flow

1. 记录创建或编辑返回的 `xml_presentation_id`，以及已知的 `slide_id` / `revision_id`。
2. 用 `slides +xml-get` 回读全文 XML 到本地文件。
3. 检查实际页数是否符合计划或用户要求。
4. 检查每页 `<data>` 内是否有预期主要元素。
5. 检查没有明显空白页、破损页、缺失标题或缺失主视觉。
6. 检查页面不是全部退化为标题加 bullet list。
7. 检查视觉层级：标题、主视觉、支撑信息三者可区分。
8. 检查明显溢出和布局风险：重叠、越界、底部拥挤、长文本框。
9. 在最终回复中给出简短验证记录。

回读命令：

```bash
lark-cli slides +xml-get --as user \
  --presentation "YOUR_ID" \
  --output .lark-slides/plan/<deck-or-task-id>/readback.xml \
  --json
```

## Automated XML Layout Lint

`slides +xml-get` 保存 XML 后，只运行统一版式准出入口。先取得当前已加载 `lark-slides/SKILL.md` 的父目录，记为 `<lark-slides-skill-dir>`；不要猜测全局安装路径。

```bash
python3 "<lark-slides-skill-dir>/scripts/xml_text_overlap_lint.py" --input <presentation.xml>
```

它一次检查 XML/SXSD 合法性、元素越界、文本重叠、空白页、文本高度风险、整页内容稀疏和大卡片内容覆盖率。大卡片自身 `<content>` 的估算文本面积与卡片内平级元素一起参与覆盖率并集计算。

准出规则：

- `summary.error_count > 0` 或 `summary.release_ready == false`：阻断创建、替换或交付，必须先修复。
- `summary.warning_count > 0`：静态检查不直接阻断，但 `summary.screenshot_review_required == true`，必须复核对应页面截图。
- `slides[].status` 为 `blocked`、`needs_screenshot_review` 或 `passed`，可直接决定逐页后续动作。
- CLI 在存在 `error` 时退出码为 1；只有 `warning` 时仍输出 JSON 并退出 0，供截图复核链路继续执行。

每条 `error` / `warning` 都包含：

- `element_ids`：相关 XML 元素 ID；
- `rule`：规则 ID、名称、阈值和比较关系；
- `measurement`：越界量、交叠面积、覆盖率等实测值；
- `related_objects`：相关对象的类型与坐标框；
- `target`、`message`、`hint`：页码、语义说明和处理建议。

当 `sparse_container_content.measurement.content_coverage_ratio < rule.threshold` 时，需要结合同页截图判断留白是否有意设计；不要仅凭 warning 自动扩充内容。

常见 code 的处理方向：

| code | 含义 | 处理方式 |
|------|------|----------|
| `xml_not_well_formed` | XML 语法错误或文本未转义 | 修复标签闭合、属性引号、`&` / `<` / `>` 转义 |
| `sml_prefixed_tag` | SML 元素使用了命名空间前缀，如 `<ns0:slide>` 或 `<sml:shape>` | 使用 `<slide xmlns="http://www.larkoffice.com/sml/2.0">` 的默认命名空间，或使用无前缀标签 |
| `sxsd_unsupported_tag` | 使用了 SXSD 不支持的标签 | 按 lint `hint` 替换为受支持标签；常见如 `textbox -> <shape type="text">`、`image -> <img>` |
| `sxsd_unsupported_attr` | 支持的标签上使用了不支持的属性 | 按 lint `hint` 改为支持的属性；常见如 `x -> topLeftX`、`fontColor -> color` |
| `iconpark_unsupported_icon_type` | `<icon>` 使用了 `iconpark-index.json` 中不存在的 `iconType` | 按 lint `hint` 改为名单内的 `iconType`，或先用 `scripts/iconpark_tool.py` 搜索 |
| `icon_missing_fill_color` | 视觉规范要求 `<icon>` 设置 `<fill><fillColor color="..."/></fill>`，避免图标不可见 | 给 `<icon>` 添加显式非透明填充色，例如 `rgba(37, 99, 235, 1)` |
| `icon_transparent_fill_color` | `<icon>` 的 `fillColor` 是透明色，不满足视觉可见性要求 | 改成与背景有足够对比的非透明颜色 |
| `bbox_overlap` | 文本元素的估算绘制区域明显重叠 | 拉开文本坐标、缩小文本框/字号，或改成明确的分栏/分组结构 |
| `*_out_of_canvas` | 元素边界超出页面画布 | 根据 `measurement.overflow` 移回画布或缩小尺寸 |
| `blank_slide` | 页面没有画布内可见内容 | 补充主体内容；仅有空背景或空形状不能准出 |
| `sparse_container_content` | 大卡片内容覆盖率低于阈值 | 按元素 ID 定位卡片，结合截图判断是否补充或放大内容 |
| `sparse_slide_content` | 全页有效内容覆盖率偏低 | 复核截图，确认是否为有意留白 |

## Screenshot QA

获取页面截图后，必须做视觉验收；不要只凭 XML 回读或静态 lint 结论声称截图验收通过。验收时假设页面存在问题，主动寻找并报告所有风险，包括轻微问题。

```text
请逐页目视检查这些幻灯片截图。先假设存在问题，并尽量找出它们。

重点检查：
- 元素重叠：文字与形状、图片或图表互相遮挡，线条穿过文字，卡片或标签堆叠。
- 文本溢出或被裁切：靠近页面边缘、文本框边界或卡片边界处被截断。
- 装饰元素位置错误：分割线、强调线或标签底板按单行文字布置，但标题或正文换行后压住文字或距离异常。
- 来源标注、页脚或页码与上方内容碰撞。
- 元素距离过近：相邻元素间距明显不足，卡片或分区几乎贴在一起；按 960x540 画布估算，小于约 15 px 的间隔通常要标记。
- 间距不均：局部留白过大，另一处过于拥挤。
- 页面边距不足：主体内容贴近幻灯片边缘；按 960x540 画布估算，小于约 30 px 的外边距通常要标记。
- 列、卡片、图标或同类元素没有稳定对齐。
- 图片或图表渲染异常：空白、变形、低清、关键内容不可读或预期图形缺失。
- 文本对比度不足，例如浅灰文字放在米色或浅色背景上。
- 图标对比度不足，例如深色图标放在深色背景上，且没有浅色圆形或底板承托。
- 文本框过窄，导致不必要的频繁换行。
- 残留占位符、模板默认文字或未替换内容。

对每一页分别列出发现的问题或可疑区域，即使只是轻微问题也要记录。

报告所有发现的问题，包括轻微问题。
```

必须根据问题严重度决定是否修复：空白页、破图、文字遮挡、明显裁切、低对比不可读、占位符残留等必须先修复再交付；轻微间距或对齐问题如果不修复，最终验证记录要说明已知风险。

## Page Count And Structure

- 实际页数必须等于用户要求或 `slide_plan.json` 的页数。
- 如果创建过程部分失败，先记录已创建的 `xml_presentation_id`，再回读确认哪些页已写入。
- 每页都应包含 `<data>`，且 `<data>` 内至少有一个非背景主体元素。
- 封面、章节页、总结页可以文字较少，但不能只有空背景。
- 技术解释页、对比页、流程页、架构页必须有匹配的结构元素，例如分组框、连线、时间轴、表格或图形化区域。

## Expected Elements

按 `slide_plan.json` 和用户要求逐页核对：

- 标题或主结论存在，并能对应 `key_message`。
- `layout_type` 对应的主要结构已生成。
- `visual_focus` 是页面中最醒目或最大的信息区域之一。
- `text_density` 影响了文本量，没有用长 bullet 框替代规划。
- `asset_need` 有真实素材时已放入正确区域；没有真实素材时，`fallback_if_missing` 已用 XML 形状、线条、标签、表格或图表兜底。

如果用户指定了关键页，例如“架构解释”“Self-Attention 机制解释”“对比或演进视角”“总结页”，最终验证记录必须逐项说明这些页已存在。

## Blank Or Broken Page Signals

把下面情况视为需要修复后再交付：

- `<data/>` 为空，或只有背景、装饰线、空 `<content/>`。
- 关键文本没有出现在回读 XML 中。
- 图片仍是 `@./path`，或 `<img src>` 是 http(s) 外链。
- 页面依赖的图片区域为空，且没有 fallback visual。
- 返回 XML 缺页、页序明显错误，或某页内容被 shell 截断。
- 大量形状坐标完全相同，导致主体内容重叠。
- 渐变背景回退成空白或白底，导致文字不可读。

## Layout And Overflow Risk

优先修复这些明显风险：

- 正文或标签框高度不足，文本很可能被截断。
- 多个主体元素在同一区域重叠，而不是有意叠加背景。
- 重要内容越过画布边界，或贴近底部超过 `y=500`。
- 高密度页使用单个长 bullet list，没有分栏、表格或分组。
- 标题、主视觉、正文的字号和颜色差异太弱，视觉层级不清。
- 所有内容页都是同一套标题加 bullets 坐标。

## Verification Record

最终回复必须包含简短验证记录，建议格式：

```text
验证记录：
- 回读：已执行 slides +xml-get，实际页数 N / 预期 N。
- 关键页：架构解释 / Self-Attention / 对比或演进 / 总结页均存在。
- 结构：检查了主要 shape/img/table/chart 元素，无明显空白页或破损页。
- 布局：检查了标题层级、主视觉、重叠/越界/文本溢出风险。
```

不要声称完成了人工视觉验收，除非确实打开或获取了可视化结果。仅从 XML 静态检查得出的结论，应表述为“静态检查未发现明显问题”。
