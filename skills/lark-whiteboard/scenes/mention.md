# 提及用户 (@用户 / mentionUser)

适用于：文本节点内需要 @ 某个飞书用户（如"负责人：@张三"、"@李四 请跟进"）。mention 不是独立节点，而是文本节点富文本中的一段 run，可与普通文字混排。

> 当用户要插入 @用户提及时阅读本页。

## 取值来源（强约束）

- 本页只讲 @用户（mentionUser）。@文档（mentionDoc）暂不支持。
- `mentionUserId` 必须是**真实的飞书用户 open_id**（形如 `ou_xxxxxxxx`）。
- 用户只给出**姓名**时，先用 `lark-contact` skill 把姓名解析成 open_id，再填入 `mentionUserId`。
- **无法解析出真实 open_id 时，停下向用户确认，禁止臆造 id**。假 id 会写入失败或 @ 到错误的人。

## Content 约束（关键）

- 带 `mentionUserId` 的 run，其 `content` **必须非空**，约定填 `"*"`（单字符占位）。
  - 原因：转换按字符占位来引用样式，`content` 为空串时该 mention 不会产出任何元素（静默丢失）。
- `content` 的字面内容**不会显示**：画板上显示的是按 open_id 反查到的用户名，不是 `content` 的文字。因此不要把用户名写进 `content`，填单个 `"*"` 即可。
- 一个 run 只能是一种类型：`mentionUserId` 与 `hyperlink` **互斥**，不能同时出现在同一个 run（校验会报错）。需要"链接 + @用户"时拆成两个 run。

## 骨架示例

`text` 用 `WBTextRun[]`，把 @用户 拆成独立 run（`content: "*"` + `mentionUserId`），前后再接普通文字 run：

```json
{
  "type": "text",
  "width": "fit-content",
  "height": "fit-content",
  "text": [
    { "content": "负责人：", "fontSize": 14 },
    { "content": "*", "mentionUserId": "ou_xxxxxxxxxxxxxxxx", "fontSize": 14 },
    { "content": " 请本周内跟进", "fontSize": 14 }
  ]
}
```

写入画板走标准 DSL 路径（`npx -y @larksuite/whiteboard-cli@^0.2.13 -i diagram.json --to openapi --format json | lark-cli whiteboard +update ... --input_format raw`），无需手写 raw JSON。

## 正反例

正确：

```json
{ "content": "*", "mentionUserId": "ou_abc123" }
```

错误（content 空串 → 不产出 @用户）：

```json
{ "content": "", "mentionUserId": "ou_abc123" }
```

错误（把用户名写进 content → 多余占位，显示仍由 uid 决定）：

```json
{ "content": "@张三", "mentionUserId": "ou_abc123" }
```

错误（与 hyperlink 同 run → 校验报错，须拆两个 run）：

```json
{ "content": "*", "mentionUserId": "ou_abc123", "hyperlink": "https://xxx.com" }
```

## 陷阱

- **content 为空**：mention 静默丢失，画板上看不到 @用户。必须填 `"*"`。
- **把用户名写进 content**：无意义，显示名由 open_id 反查决定；且多字符会占用多个字符位。
- **mentionUserId + hyperlink 同 run**：一个 run 只能是一种元素类型，会被校验拦截，须拆成两个 run。
- **用假 id 或用户中文名当 id**：`mentionUserId` 只接受真实 open_id，先经 `lark-contact` 解析。
