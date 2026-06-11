
# note +detail

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过已知 `note_id` 查询纪要元信息、展示类型和关联文档 token。只读操作，仅支持 `user` 身份。

本 skill 对应 shortcut：`lark-cli note +detail`。

> **`note_id` 来源：** 从文档入口进入时，先执行 `docs +fetch --api-version v2 --doc <doc_token>`；返回里的 `<vc-transcribe-tab vc-node-id="..."></vc-transcribe-tab>` 表示 VC 原始记录 block，其中 `vc-node-id` 属性值就是这里的 `NOTE_ID`。如果没有该 block，但正文里有“文字记录/逐字稿”等明确 Docx 链接，那是普通纪要的独立逐字稿文档，直接用 `docs +fetch --api-version v2 --doc <verbatim_doc_token>` 读取，不要从 `doc_token` 反推 `note_id`。

## 命令

```bash
lark-cli note +detail --note-id NOTE_ID
lark-cli note +detail --note-id NOTE_ID --format json
lark-cli note +detail --note-id NOTE_ID --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--note-id <id>` | 是 | Note ID；如果来自 `docs +fetch --api-version v2`，取 `<vc-transcribe-tab>` 的 `vc-node-id` 属性值 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 输出结果

返回 `note` 对象，包含：

| 字段 | 说明 |
|------|------|
| `note_id` | 输入的 Note ID（显式回显） |
| `note_display_type` | `unknown` / `normal` / `unified`，区分普通纪要和 unified 纪要 |
| `note_doc_token` | AI 智能纪要主文档 token |
| `verbatim_doc_token` | 普通纪要逐字稿文档 token |
| `shared_doc_tokens` | 会中共享文档 token 列表（为空时省略） |
| `creator_id` | 创建者 ID |
| `create_time` | 创建时间（格式化） |

输出示例：

```json
{
  "note": {
    "note_id": "note_xxxx",
    "note_display_type": "unified",
    "note_doc_token": "doxcnxxxx",
    "verbatim_doc_token": "doxcnyyyy",
    "shared_doc_tokens": ["doxcnzzzz"],
    "creator_id": "ou_xxxx",
    "create_time": "2026-06-04 10:00"
  }
}
```

## 拿到结果后的路由

| 用户意图 | 后续动作 |
|---------|---------|
| 读纪要正文 / 总结 / 待办 / 章节 | `docs +fetch --api-version v2 --doc <note_doc_token>` |
| `note_display_type=normal` + 要逐字稿 | `docs +fetch --api-version v2 --doc <verbatim_doc_token>` |
| `note_display_type=unified` + 要逐字稿 / 原始记录 | `note +transcript --note-id <note_id>`（见 [lark-note-transcript.md](lark-note-transcript.md)） |

> **判别键是 `note_display_type`。** 即使 unified 纪要也返回了非空 `verbatim_doc_token`，unified 的逐字稿仍应走 `note +transcript`（内容更结构化）。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--note-id is required` | 未传入 note_id | 补全 `--note-id` |
| `no read permission for this note` | 调用者无该纪要阅读权限 | 向纪要所有者申请权限 |
| `missing required scope(s)` | 缺少 `vc:note:read` | 按提示运行 `auth login --scope vc:note:read` |

## 参考

- [lark-note](../SKILL.md) — Note 域总入口
- [lark-note-transcript](lark-note-transcript.md) — unified 原始记录查询
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
