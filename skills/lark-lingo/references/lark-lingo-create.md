# lingo +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一条新的词条。**必须 `--as bot`**。

## 命令

```bash
# 最小请求（只主词）
lark-cli lingo +create --as bot \
  --main-key "KYC"

# 常规：主词 + 别名（逗号分隔） + 释义
lark-cli lingo +create --as bot \
  --main-key "飞书" \
  --aliases "Lark,FeiShu,飞书办公" \
  --description "企业协作平台"

# 从文件读释义
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --description @./desc.txt

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
| `--description` | 否 | 释义文本；支持 `@file` 和 `-`（stdin） |
| `--repo-id` | 否 | 词典库 ID；省略时写入全公司共享词典 |
| `--allow-highlight` | 否 | 是否在文档中高亮，默认 `true` |
| `--allow-search` | 否 | 是否参与搜索，默认 `true` |

## 关键约束

- **必须 `--as bot`**：API 端点拒收 user token，user 调用会返 `99991668 "user access token not support"`。
- **主词只能 1 个**：API schema 限制 `main_keys` 数组最多 1 个元素；本 shortcut 只暴露单 `--main-key`。多主词需求请用原生 `lark-cli api POST /open-apis/lingo/v1/entities --data '{...}'`。
- **创建会进审核队列**：除非应用已开通 `baike:entity:exempt_review`，词条要等管理员审批通过才对外可见。创建成功后应告知用户「已提交审核」。
- **幂等性**：本 shortcut 不做存在性检查。"没收录就新建"的流程必须自己先调 [`+match`](lark-lingo-match.md) 判断。

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
      "description": "…"
    }
  }
}
```

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
