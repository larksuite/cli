# Lark Sheet Changeset

## 使用场景

读取两个版本之间的 **changeset（变更操作清单）**，用于**复核某次编辑（尤其是 AI 编辑）是否真实满足用户诉求**。

典型场景：AI agent 对表格做了一批编辑后，想确认它"说做的"和"真正落到表格上的"是否一致——拉取编辑前版本到编辑后版本之间的 changeset，逐条核对 action 是否覆盖了用户要求的修改、有没有多改 / 漏改。

## 版本（revision）语义

- 这里的"版本"指表格的 **CS revision**（每次提交单调递增的修订号），不是文档历史里的命名版本。
- `--start-revision` 是复核基线，即你认定的"编辑前"版本。
- `--end-revision` 是"编辑后"版本；**省略时默认取最新 revision**，返回从 start 到最新的全部 changeset。
- **版本差上限 20**：`end - start + 1 ≤ 20`，超出会被拒绝（服务端同样以 20 兜底）。复核大跨度变更时请分段拉取。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+changeset-get` | read | 变更记录 |

## Flags

### `+changeset-get`

_公共：URL/token（无 sheet 定位）_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--start-revision` | int | required | 起始版本（编辑前基线，>= 1） |
| `--end-revision` | int | optional | 结束版本（省略取最新） |

## 返回结构

返回一个 JSON 对象，`changesets` 数组按版本顺序排列，每个元素是一次提交的**原始 action 列表**与元信息：

```json
{
  "spreadsheet_token": "shtcnXXXX",
  "latest_revision": 142,
  "start_revision": 120,
  "end_revision": 135,
  "changesets": [
    {
      "revision": 121,
      "create_time": "2026-06-12T10:00:00Z",
      "actions": [
        { "action": "setCellRange", "sheetId": "...", "value": { /* ... */ } }
      ],
      "is_self_edit": false,
      "is_ai_edit": true
    }
  ]
}
```

- 最外层 `latest_revision` 是**当前表格的最新版本号**（与查询区间无关），便于判断表格当前停在哪个版本、`--start-revision` 该取多少。
- `actions` 是**未经语义渲染的原始操作对象**，按提交内的执行顺序排列。复核时逐条比对：每个 action 改了哪个 sheet、哪个区域、改成什么，是否对应用户的诉求。
- `revision` / `create_time` 用于判断"这次改动属于哪个版本、什么时候做的"。
- `is_self_edit` 表示该 changeset 是否由当前请求用户提交（committer 与请求用户相同），即"是不是我自己提交的编辑"。
- `is_ai_edit` 表示该 changeset 是否由 AI 客户端提交（`member_id` 为 10 / 11）。复核时 `is_ai_edit=true` 即为 AI 写入的编辑（而非用户手动编辑），是核对 AI 是否完成诉求的主要对象。

## 复核工作流（判断 AI 是否真实完成诉求）

1. 记下 AI 开始编辑前的 revision（编辑前 `+workbook-info` 或上一次工具返回的 revision 即可作为 `--start-revision`）。
2. AI 编辑完成后，跑 `+changeset-get --url <表格> --start-revision <编辑前版本>`（不传 end → 取到最新）。
3. 遍历 `changesets[].actions`，核对：
   - 用户要求的每一处修改是否都有对应 action；
   - 有没有越权 / 多余的修改（动了用户没让动的 sheet / 区域）；
   - action 的目标区域、值是否与诉求一致。
4. 若版本跨度可能 > 20，分段拉取（如 `start..start+19`、`start+20..` …）。

## 注意

- `+changeset-get` 是**只读**操作，不改动表格。
- 大跨度 / 大批量编辑的 changeset 可能体积较大；输出在传输层已 gzip。必要时缩小版本区间。
- 该工具走只读 scope `sheets:spreadsheet:read`，需要对表格有查看权限。

## Examples

### `+changeset-get`

公共：`--url` / `--spreadsheet-token`（二选一，无 sheet 定位）。changeset 是工作簿级历史，不接受 sheet 定位 flag。

示例：

```bash
# 只传起始版本 → 返回从该版本到最新的全部 changeset（最常用：复核 AI 编辑前后的差异）
lark-cli sheets +changeset-get --url "https://example.feishu.cn/sheets/shtXXX" --start-revision 120

# 传起始 + 结束版本（版本差 end-start+1 ≤ 20）
lark-cli sheets +changeset-get --spreadsheet-token shtXXX --start-revision 120 --end-revision 135
```

输出契约（envelope.data）：

- `latest_revision` — 当前表格最新版本号（与查询区间无关）
- `start_revision` / `end_revision` — 实际查询区间（省略 `--end-revision` 时 `end_revision` = 最新版本）
- `changesets[]` — 按版本顺序排列；每项含 `revision` / `create_time` / `actions`（原始操作列表）/ `is_self_edit` / `is_ai_edit`

### Validate / DryRun / Execute 约束

- `Validate` 阶段只做 XOR 检查（`--url` / `--spreadsheet-token` 二选一）与版本上限校验（`--start-revision ≥ 1`，传了 `--end-revision` 时 `end ≥ start` 且 `end - start + 1 ≤ 20`）；**禁止**联网。
- `DryRun` 输出请求模板，不实际拉取 changeset。
- `Execute` 阶段才发起 changeset 查询；省略 `--end-revision` 时由服务端解析为最新 revision。