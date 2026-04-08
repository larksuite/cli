# base +record-batch-add

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量新增记录。

## 适用场景（重点）

- 适合大量新增写入场景，例如导入 CSV / Excel、外部系统一次性灌入新数据。
- 当输入是长表格或长文本数据时，先按 [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md) 做字段映射和类型规范化，再组装 `fields + rows` 调用本命令写入。

## 推荐命令

```bash
lark-cli base +record-batch-add \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --json '{"fields":["标题","状态"],"rows":[["任务 A","Open"],["任务 B","Done"]]}'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 批量新增请求体，必须是 JSON 对象 |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch
```

## `--json` 结构

- 对象形态：`{"fields":[...],"rows":[...]}`。
- `fields`：字段 id 或字段名数组。
- `rows`：二维数组，每一行按 `fields` 的同序列给值。

## 返回重点

- 返回对象键：
  - `fields`
  - `field_id_list`
  - `record_id_list`
  - `data`
  - `ignored_fields`（可选）

## 坑点

- ⚠️ `--json` 必须是对象。
- ⚠️ 写 `rows` 前必须先阅读 [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md)，按字段类型填值，禁止按自然语言猜测 value 结构。
- ⚠️ `fields` 与 `rows` 列顺序必须一一对应。
- ⚠️ 单次最多 200 行，超出需分批写入。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md) — 记录值格式规范
