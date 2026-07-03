# 文档统计：总字数 / 总字符数

当用户需要统计 Docx / Wiki 文档的总字数或总字符数时，使用本 skill 附带脚本 `scripts/doc_word_stat.py`。统计口径以该脚本为准，不要改用其他方式自行计算，也不要只读取 simple 摘要后统计。

## 调用方式

在线文档使用 XML full 内容，并让脚本读取 `docs +fetch --format json` 的 envelope：

```bash
lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \
  | python3 skills/lark-doc/scripts/doc_word_stat.py --protocol xml --lark-json --pretty
```

`$URL` 可以是用户给出的 docx/wiki URL，也可以是可被 `docs +fetch` 解析的 token。

## 统计范围

先判断用户要求的是**整篇文档**还是**局部内容**：

- 整篇文档的总字数 / 总字符数：按上方「调用方式」抓取 `full` 内容后统计。
- 本次新增 / 替换 / 改写片段的字数：优先统计拟写内容本身；内容已写入文档时，只 fetch 对应 block / range 后统计。不得用整篇文档字数对比局部目标。

如需在自动化或回归验证中发现未覆盖块类型，追加严格参数：

```bash
lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \
  | python3 skills/lark-doc/scripts/doc_word_stat.py --protocol xml --lark-json --pretty --fail-on-unsupported --fail-on-unknown
```

## 如何读取结果

脚本输出 JSON。对用户汇报时默认只读两个核心字段：

- `word_count`：总字数。按语义单位统计汉字、英文单词/URL/code path、数字、中文标点；普通贴着英文的英文标点不计入，但独立 ASCII 符号、中文之间的 `/` 等以脚本结果为准。
- `char_count`：总字符数。统计汉字、英文字母、数字、中英文标点和脚本识别的可见符号；空格不计入。

其余字段用于排查或解释：

- `breakdown`：拆分统计来源，例如 `han_chars`、`english_words`、`digits`、`chinese_punctuations`。
- `unknown_blocks`：脚本遇到未知 XML/Markdown 块类型；通常表示需要扩展解析规则。
- `unsupported_blocks`：脚本识别到块类型，但当前无法可靠提取可见文本。
- `diagnostics.has_unknown` / `diagnostics.has_unsupported`：快速判断统计是否存在覆盖风险。

如果 `unknown_blocks` 或 `unsupported_blocks` 非空，回复用户时要说明“已统计可提取文本，但存在未覆盖块，结果可能偏低”，并列出对应块类型。为空时可直接给出结果。

## 字数遵循校验

当用户给了明确字数要求（写 N 字 / x-y 字 / x 字左右 / 上下浮动）时执行；没有明确字数要求则跳过。字数必须按本文流程用脚本统计，不要自己估。

1. 先按「统计范围」确认统计对象，再把要求归一成目标区间：`>x`→`[x+1, +∞)`；`<y`→`(-∞, y-1]`；`x-y`→`[x, y]`；`x 字左右`→`[round(0.9x), round(1.1x)]`
2. 按统计对象选择对应输入并调用脚本统计实际字数，读取输出里的 `word_count`
3. 对比 `word_count` 与目标区间：区间内即通过；低于下限 → 补充**实质内容**（非注水）；高于上限 → 删减冗余内容。改完重新统计
4. **最多 2 轮**。2 轮后仍不达标：停止，不得为达标而注水或删关键内容；如实汇报【目标区间 / 当前字数 / 差值与方向 / 已试 2 轮 / 未达原因】，**禁止谎称达标**

## 输出示例

输入正文等价于：`标题` + `一个苹果是 an apple。` 时，输出形态如下：

```json
{
  "word_count": 10,
  "char_count": 15,
  "breakdown": {
    "han_chars": 7,
    "english_words": 2,
    "number_words": 0,
    "chinese_punctuations": 1,
    "english_letters": 7,
    "digits": 0,
    "english_punctuations": 0,
    "symbol_words": 0,
    "symbol_chars": 0
  },
  "protocol": "xml",
  "unknown_blocks": [],
  "unsupported_blocks": [],
  "diagnostics": {
    "has_unknown": false,
    "has_unsupported": false,
    "types": {},
    "unknown_types": {},
    "unsupported_types": {},
    "actions": {}
  }
}
```

面向用户的回复可简化为：

```text
总字数：10
总字符数：15
```
