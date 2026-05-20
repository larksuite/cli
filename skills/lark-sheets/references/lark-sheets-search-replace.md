# Lark Sheet Search & Replace

## 替换前 dry-run + 范围明确（替换前必做）

`+cells-replace` 的副作用是不可逆的（除非另写代码回滚）。执行前必须：

1. **明确替换范围**：必须显式说明"只替换 X 列 / X 区域，还是全表替换"。**禁止**默认全表替换——容易误改无关列。范围应由用户指令决定，模糊时主动询问。
2. **dry-run 命中数量**：先用 `+cells-search` 在同一范围、同一关键词、同一匹配选项（大小写 / 精确 / 正则）下统计命中数量。把数量和**期望命中数**（用户明示的或基于业务理解推断的）对照——一致才进入 `+cells-replace`，不一致先排查（关键词太宽？范围太大？）。
3. **替换后回读校验**：执行后再次 `+cells-search` 旧关键词，预期为 0；并对替换后的若干代表性单元格回读确认值符合预期。

## 使用场景

读写。在飞书表格中搜索和替换文本。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 搜索/定位文本 | `+cells-search` | 返回匹配的单元格位置，支持正则、精确匹配等 |
| 查找并替换文本 | `+cells-replace` | 批量替换文本，支持正则捕获组引用 |

**常见配置错误（必须注意）**：
- **不要把操作动词当搜索词**：用户说"汇总金额"是一个操作动作（求和），不是要搜索"汇总金额"这个文本。只有当确实需要定位某个文本值的位置时才用 `+cells-search`
- **不要用搜索来了解表格结构**：要了解表头和数据结构时，应使用 `+csv-get` 读取前几行，而不是用 `+cells-search` 逐个猜测字段名
- **注意正则特殊字符**：使用正则匹配时，`.`、`*`、`(`、`)` 等特殊字符需要转义

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cells-search` | read | 单元格 |
| `+cells-replace` | write | 单元格 |

## Flags

### `+cells-search`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--find` | string | 是 | 待查找文本（与 `--regex` 配合时按正则解释） |
| `--range` | string | 否 | 查找范围（A1 格式）；省略时整表 |
| `--match-case` | bool | 否 | 大小写敏感 |
| `--match-entire-cell` | bool | 否 | 完全匹配整个单元格 |
| `--regex` | bool | 否 | 把 `--find` 按正则解释 |
| `--include-formulas` | bool | 否 | 也在公式文本中搜索 |
| `--max-matches` | int + Hidden | 否 | 防爆，默认 5000 |

### `+cells-replace`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--find` | string | 是 | 待替换文本 |
| `--replacement` | string | 是 | 替换为；传空字符串 `""` 等价于「删除内容」 |
| `--range` | string | 否 | 替换范围（A1 格式）；省略时整表 |
| `--match-case` | bool | 否 | 也在公式文本中替换 |
| `--match-entire-cell` | bool | 否 | 也在公式文本中替换 |
| `--regex` | bool | 否 | 也在公式文本中替换 |
| `--include-formulas` | bool | 否 | 也在公式文本中替换 |

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR 规则）。

### `+cells-search`

示例：

```bash
# 普通查找
lark-cli sheets +cells-search --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --find "张三"

# 正则 + 范围限定
lark-cli sheets +cells-search --spreadsheet-token shtXXX --sheet-id "$SID" \
  --find "^[A-Z]{2}-\\d{4}$" --regex --range "A2:A1000"
```

输出契约（envelope.data）：

- `matched_cells` — 命中 cell 列表，每条含 `cell`（A1）+ `value` + `sheet_id`
- `total_matches` — 匹配总数

### `+cells-replace`

示例：

```bash
# 先 dry-run 预览
lark-cli sheets +cells-replace --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --find "v1" --replacement "v2" --dry-run

# 确认后执行
lark-cli sheets +cells-replace --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --find "v1" --replacement "v2"
```

> `+cells-replace` 虽然 Risk = write，但范围大或正则错可能改一堆。**强烈推荐工作流**：先 `+cells-search` 看匹配数，再 `+cells-replace --dry-run` 预览，最后真正执行。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--find` 非空；正则模式下 `--find` 必须是合法正则。
- `DryRun`：`+cells-search` 输出请求模板；`+cells-replace` 额外返回预估替换数（`would_replace_count`）。
- `Execute`：写后自动回读匹配范围抽样验证，`envelope.meta.verification` 给出"预估替换数 vs 实际替换数"对比。
