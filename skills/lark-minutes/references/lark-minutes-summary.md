# minutes +summary


替换妙记的 AI 总结内容。写操作，会覆盖当前总结。

本 skill 对应 shortcut：`lark-cli minutes +summary`（调用 `PUT /open-apis/minutes/v1/minutes/{minute_token}/summary`）。

## 典型触发表达

- "把这条妙记的总结改成……"
- "更新 / 替换妙记的 AI 总结"
- "修正总结内容后写回妙记"

## 命令

```bash
# 直接传入总结内容（Markdown 子集）
lark-cli minutes +summary --minute-token obcnxxxxxxxxxxxxxxxxxxxx --summary "**会议结论**\n- 方案 A 通过\n- 下周跟进排期"

# 从文件读取总结内容
lark-cli minutes +summary --minute-token obcnxxxxxxxxxxxxxxxxxxxx --summary @summary.md

# 从 stdin 读取
echo "**结论**" | lark-cli minutes +summary --minute-token obcnxxxxxxxxxxxxxxxxxxxx --summary @-

# 预览 API 调用
lark-cli minutes +summary --minute-token obcnxxxxxxxxxxxxxxxxxxxx --summary @summary.md --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记 Token |
| `--summary <text>` | 是 | 替换后的总结内容，支持 `@file` / `@-`（stdin） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 先读后写

替换前建议先用 `lark-cli minutes +detail --minute-tokens <token> --summary` 读取当前总结，确认 `minute_token` 与待替换内容无误。

### 2. Markdown 展示说明

接口接受任意总结文本，**不会因 Markdown 格式校验失败而拒绝请求**。妙记客户端通常只能良好渲染以下 Markdown 子集；不支持的语法（如链接、代码块、四级标题等）会**按原始文本展示**（保留 Markdown 标记字符，不会渲染成对应样式）。Agent 写入时应优先使用可展示语法，避免用户在妙记里看到字面量的 `[链接](url)`、`` `code` `` 等：

| 支持 | 写法 | 示例 |
|------|------|------|
| 纯文本 | 普通段落 | `本次会议讨论了 Q2 预算` |
| 换行 | `\n` 或空行 | 分段落书写 |
| 一级标题 | `# ` + 标题文字 | `# 会议结论` |
| 二级标题 | `## ` + 标题文字 | `## 行动项` |
| 三级标题 | `### ` + 标题文字 | `### 跟进事项` |
| 加粗 | `**文字**` | `**重点结论**` |
| 无序列表 | `- ` 或 `* ` | `- 跟进预算审批` |
| 有序列表 | `1. ` | `1. 确认需求` |

> 标题语法建议：`#` 后保留空格，并优先使用 1～3 级（`#` / `##` / `###`）。四级及以上（`####`）无法渲染，会以原始文本形式展示。

**不建议使用**（会按原始文本展示）：链接、图片、代码块、表格、引用块、斜体、删除线、四级及以上标题等。

合法示例：

```markdown
# 会议结论

## 核心讨论

**方案 A 通过**，下周启动排期。

### 待跟进
- 预算审批
- 排期确认

1. 张三负责预算
2. 李四负责排期
```

### 3. 所需权限

| 身份 | 所需权限 |
|------|---------|
| user | `minutes:minutes:update` |

## 输出结果

```json
{
  "minute_token": "obcnxxxxxxxxxxxxxxxxxxxx",
  "updated": true
}
```

| 字段 | 说明 |
|------|------|
| `minute_token` | 妙记 Token |
| `updated` | 是否已成功更新 |

## 如何获取 minute_token

| 来源 | 获取方式 |
|------|---------|
| 妙记 URL | 从 URL 末尾提取，如 `https://sample.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` |
| 妙记搜索 | `lark-cli minutes +search --query "关键词"` |
| 会议产物查询 | `lark-cli vc +detail --meeting-ids <id>` 或 `vc +recording`, 拿到 `minute_token`, 然后走 `minutes +detail` |

## 常见错误与排查

| 错误现象 | 错误码 | 根本原因 | 解决方案 |
|---------|--------|---------|---------|
| 总结展示为原始 Markdown 文本 | — | 总结含链接、四级标题等妙记端无法渲染的语法 | 改用标题（#～###）、加粗、列表等可展示格式；接口不会因此报错 |
| 参数无效 | — | `minute_token` 缺失或格式错误 | 检查 token 是否完整 |
| 权限不足 | — | 缺少 `minutes:minutes:update` | 运行 `auth login --scope "minutes:minutes:update"` |

## 参考

- [lark-minutes](../SKILL.md) — 妙记全部命令
- [minutes +todo](lark-minutes-todo.md) — 替换待办项
- [minutes +detail](lark-minutes-detail.md) — 读取总结、待办等 AI 产物
