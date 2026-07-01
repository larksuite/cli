# 文档统计：总字数 / 总字符数

当用户需要统计 Docx / Wiki 文档的总字数或总字符数时，使用本 skill 附带脚本 `scripts/doc_word_stat.py`。统计口径以该脚本为准，不要改用其他方式自行计算，也不要只读取 simple 摘要后统计。

## 调用方式

在线文档使用 XML full 内容，并让脚本读取 `docs +fetch --format json` 的 envelope：

```bash
lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \
  | python3 skills/lark-doc/scripts/doc_word_stat.py --protocol xml --lark-json --pretty
```

`$URL` 可以是用户给出的 docx/wiki URL，也可以是可被 `docs +fetch` 解析的 token。

如需在自动化或回归验证中发现未覆盖块类型，追加严格参数：

```bash
lark-cli docs +fetch --doc "$URL" --doc-format xml --detail full --format json \
  | python3 skills/lark-doc/scripts/doc_word_stat.py --protocol xml --lark-json --pretty --fail-on-unsupported --fail-on-unknown
```

## 如何读取结果

脚本输出 JSON。对用户汇报时默认只读两个核心字段：

- `word_count`：总字数。按语义单位统计汉字、英文单词、数字、中文标点；英文标点不计入。
- `char_count`：总字符数。统计汉字、英文字母、数字、中英文标点；空格不计入。

其余字段用于排查或解释：

- `breakdown`：拆分统计来源，例如 `han_chars`、`english_words`、`digits`、`chinese_punctuations`。
- `unknown_blocks`：脚本遇到未知 XML/Markdown 块类型；通常表示需要扩展解析规则。
- `unsupported_blocks`：脚本识别到块类型，但当前无法可靠提取可见文本。
- `diagnostics.has_unknown` / `diagnostics.has_unsupported`：快速判断统计是否存在覆盖风险。

如果 `unknown_blocks` 或 `unsupported_blocks` 非空，回复用户时要说明“已统计可提取文本，但存在未覆盖块，结果可能偏低”，并列出对应块类型。为空时可直接给出结果。

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
