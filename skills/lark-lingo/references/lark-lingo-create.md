# lingo +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一条新的词条。**必须 `--as bot`**。

## 命令

```bash
# 最小请求（只主词）
lark-cli lingo +create --as bot \
  --main-key "KYC"

# 常规：主词 + 别名（逗号分隔） + 纯文本释义
lark-cli lingo +create --as bot \
  --main-key "飞书" \
  --aliases "Lark,FeiShu,飞书办公" \
  --description "企业协作平台"

# 富文本释义（HTML，与 --description 互斥）
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --aliases "Know Your Customer" \
  --rich-text '<p><b>Know Your Customer 了解你的客户</b><span>，是金融机构在为客户提供服务前，对其</span><span>身份</span><span>、背景、风险等级进行核实的合规流程。</span></p>'

# 携带相关元数据：分类 / 关联词条 / 关联用户 / 文档 / 链接 / 图片 / 群 / 值班
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description "Know Your Customer" \
  --related-meta '{"classifications":[{"id":"7517595051844222977","father_id":"7517595051644862466"}],"abbreviations":[{"id":"enterprise_xxx"}]}'

# related_meta 长可走 @file
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description "Know Your Customer" \
  --related-meta @./related-meta.json

# 从文件读释义
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description @./desc.txt

# 从文件读富文本
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --rich-text @./desc.html

# 从 stdin 读释义
printf "Know Your Customer …" | \
  lark-cli lingo +create --as bot \
    --main-key "KYC" \
    --description -

# 写到私有词典库
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description "…" \
  --repo-id "<repo_id>"

# 关闭搜索参与 / 关闭高亮
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description "…" \
  --allow-highlight=false \
  --allow-search=false

# 预览底层请求
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description "…" \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--main-key` | 是 | 主词（词条显示的主关键词） |
| `--aliases` | 否 | 别名列表，**逗号分隔**；空格会被自动 trim |
| `--description` | 否 | **纯文本**释义；与 `--rich-text` 互斥；支持 `@file` 和 `-`（stdin） |
| `--rich-text` | 否 | **HTML 富文本**释义；与 `--description` 互斥；支持 `@file` 和 `-`（stdin） |
| `--related-meta` | 否 | 相关元数据 **JSON 对象**（分类、关联词条、用户、文档等）；支持 `@file` 和 `-`（stdin）；详见下表 |
| `--repo-id` | 否 | 词典库 ID；省略时写入全公司共享词典 |
| `--allow-highlight` | 否 | 是否在文档中高亮，默认 `true` |
| `--allow-search` | 否 | 是否参与搜索，默认 `true` |

## 关键约束

- **必须 `--as bot`**：API 端点拒收 user token，user 调用会返 `99991668 "user access token not support"`。
- **`--description` 与 `--rich-text` 互斥**：同时传 CLI 直接报错。两者都不传时词条无释义。
- **主词只能 1 个**：API schema 限制 `main_keys` 数组最多 1 个元素；本 shortcut 只暴露单 `--main-key`。多主词需求请用原生 `lark-cli api POST /open-apis/lingo/v1/entities --data '{...}'`。
- **创建会进审核队列**：除非应用已开通 `baike:entity:exempt_review`，词条要等管理员审批通过才对外可见。创建成功后应告知用户「已提交审核」。
- **幂等性**：本 shortcut 不做存在性检查。"没收录就新建"的流程必须自己先调 [`+match`](lark-lingo-match.md) 判断。

## rich_text HTML 格式

实测从飞书 Web UI 创建的词条，`rich_text` 是 HTML 字符串，常见标签：

- `<p>` — 段落（最外层包裹）
- `<b>` — 加粗（约定主词部分加粗）
- `<span>` — 文本片段（飞书 Web UI 倾向把每个文本块都包成 `<span>`）
- `\n` — 换行（在 HTML 内嵌入）

**惯例样式**：`<p><b>主词 中文名</b><span>，释义内容…</span></p>`

不传 `rich_text` 而只传 `description` 时，飞书后端会自动把纯文本包装成上述结构（主词部分自动加粗）。需要更精细排版（多段、换行、自定义高亮位置）时再用 `--rich-text`。

## related_meta JSON 格式

`--related-meta` 接受一个 JSON 对象，键为下表的字段名，每个值是子结构数组。**所有字段都可选**，按需填写。

| 字段 | 元素结构 | 含义 |
|------|---------|------|
| `classifications` | `{"id":"<二级分类id>","father_id":"<一级分类id>"}` | 词条所属分类。**只能选二级分类**，且每个一级分类下只能选一个二级 |
| `abbreviations` | `{"id":"enterprise_xxx"}` | 关联词条 ID（如缩写 ↔ 全称互链） |
| `users` | `{"id":"ou_xxx","title":"人名"}` | 相关联系人（open_id），`title` 可选 |
| `chats` | `{"id":"oc_xxx","title":"群名"}` | 相关公开群 |
| `docs` | `{"title":"标题","url":"https://feishu.cn/docs/xxx"}` | 相关云文档 |
| `links` | `{"title":"标题","url":"https://…"}` | 相关外部链接 |
| `oncalls` | `{"id":"<值班号>","title":"标题"}` | 相关值班号 |
| `images` | `{"token":"box_xxx"}` | 图片 token（先用文件接口上传图片），**最多 10 张** |

**完整示例**（多个字段同时设置）：

```json
{
  "classifications": [
    {"id":"7517595051844222977","father_id":"7517595051644862466"}
  ],
  "abbreviations": [
    {"id":"enterprise_7611747915522264030"}
  ],
  "users": [
    {"id":"ou_xxx","title":"人名"}
  ],
  "docs": [
    {"title":"KYC 合规流程","url":"https://feishu.cn/docs/yyy"}
  ]
}
```

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "entity": {
      "id": "enterprise_xxxx",
      "main_keys": [{"key": "KYC"}],
      "aliases": [{"key": "Know Your Customer"}],
      "description": "…",
      "rich_text": "<p>…</p>"
    }
  }
}
```

> 即使创建时只传 `--description`，返回的 `entity` 里也会有 `rich_text`（飞书自动转换）；反之只传 `--rich-text` 时返回里也会有 strip 标签后的 `description`。

## 错误排查

| 错误码 | 含义 | 修复 |
|--------|------|------|
| `99991668` `user access token not support` | 用了 user 身份 | 加 `--as bot` |
| `99991672` `Permission denied`（bot） | 应用缺 `baike:entity` scope | 飞书开发者后台 → 权限管理勾选 → 管理员审批 |

## 权限

| 操作 | 所需 scope |
|------|-----------|
| 创建（走审核） | `baike:entity` |
| 免审创建 | `baike:entity` + `baike:entity:exempt_review`（仅自建应用） |

> bot scope 在飞书开发者后台 app 配置页勾选 + 管理员审批，**不是**通过 `lark-cli auth login --scope` 申请。

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +match](lark-lingo-match.md) — 创建前先判断是否已收录
- [lark-lingo +update](lark-lingo-update.md) — 已收录时走更新
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
