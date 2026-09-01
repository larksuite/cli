# okr +comment-detail
> **前置条件：** 先阅读 [lark-shared/SKILL.md](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则；

获取指定 OKR 周期下 Cycle、Objective、KeyResult 和 Progress 的全部评论，并按评论对象和评论串整理后按时间升序排列。该 shortcut 是跨多个 OKR 接口的聚合查询。

## 推荐命令

```bash
# 获取指定周期下所有 Cycle、Objective、KeyResult 和 Progress 的评论。
lark-cli okr +comment-detail --cycle-id 1234567890123456789

# 获取原始 ContentBlock 格式的评论正文。
lark-cli okr +comment-detail --cycle-id 1234567890123456789 --style richtext

# 预览聚合查询的 API 调用，不实际执行。
lark-cli okr +comment-detail --cycle-id 1234567890123456789 --dry-run
```

## 参数

| 参数       | 必填 | 默认值 | 说明                                                                                       |
|------------|------|--------|--------------------------------------------------------------------------------------------|
| --cycle-id | 是   | —      | OKR 周期 ID，int64 正整数，可从 +cycle-list 获取。                                         |
| --style    | 否   | simple | simple 返回半纯文本格式，不涉及字体/颜色等信息时推荐使用；richtext 返回原始 ContentBlock。 |
| --dry-run  | 否   | —      | 预览聚合查询而不实际执行。                                                                 |
| --format   | 否   | json   | 输出格式。                                                                                 |

## 工作流程

1. 使用 +cycle-list 获取周期 ID；如果用户已经提供周期 ID，直接使用。
2. 执行 +comment-detail --cycle-id "..."。shortcut 会依次获取周期下的 Objective、每个 Objective 下的 KeyResult、每个 Objective/KeyResult 下的 Progress，以及四类对象的评论。
3. 评论接口自动处理分页；对象读取和评论读取使用有界并发。任一底层请求失败时整体返回错误，不返回静默不完整结果。
4. 评论串按首条评论的 create_time 升序排列，串内评论也按 create_time 升序排列。

## 输出

返回 JSON 的核心结构如下：

```json
{
  "cycle_id": "1234567890123456789",
  "comments": {
    "2345678901234567890": [
      [
        {
          "id": "7000000000000000001",
          "target": {"target_type": "objective", "target_id": "2345678901234567890"},
          "commentator_id": "ou_xxx",
          "status": "open",
          "create_time": "2025-01-15 10:30:00",
          "update_time": "2025-01-15 10:30:00",
          "selection": {"id": "8000000000000000001", "selected_text": "提升核心接口稳定性"},
          "content": {"text": "请补充指标", "mention": [], "docs": [], "images": []}
        }
      ]
    ]
  },
  "style": "simple"
}
```

- comments 第一层 key 是 target_id；value 是评论串数组；每个评论串是评论数组。
- simple 风格下 content 是 SemiPlainContent；richtext 风格下 content 是 ContentBlock。
- 评论时间戳会转换为可读日期时间；selection、状态和引用字段会保留。
- `comments` 会为周期遍历到的每个 target 保留一个 target_id key；即使该对象没有评论，对应 value 也会是空的评论串数组。

## 注意事项

- 这是聚合查询，接口调用次数取决于周期下的 Objective、KeyResult 和 Progress 数量。
- +comment-detail 不接受 department-id-type，该接口参数由 shortcut 忽略。
- 该命令只读取评论，不会修改、解决或删除评论。

## 参考

- [lark-okr](../SKILL.md) — OKR 命令、路由和通用约定
- [OKR 实体定义](lark-okr-entities.md) — Cycle、Objective、KeyResult、Progress 和 Comment 的关系
- [ContentBlock 格式](lark-okr-contentblock.md) — ContentBlock 与 SemiPlainContent 格式
- [okr +cycle-detail](lark-okr-cycle-detail.md) — 获取周期下的 Objective 和 KeyResult
- [okr +progress-list](lark-okr-progress-list.md) — 获取 Objective 或 KeyResult 下的 Progress
- [lark-shared](../../lark-shared/SKILL.md) — 认证、身份、权限和安全规则
