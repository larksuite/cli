# okr +create

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建单个 OKR 目标（Objective）或关键结果（Key Result）。这是单条写入场景的首选 shortcut；如果需要一次创建多个 Objective 及其 KR，可使用 [`+batch-create`](lark-okr-batch-create.md)。

## 推荐命令

```bash
# 在指定周期下创建一个 Objective（默认 simple 风格）
lark-cli okr +create \
  --level objective \
  --cycle-id 7000000000000000001 \
  --content '{"text":"提升北极星指标","mention":["ou_xxxxxxxx"]}' \
  --notes '{"text":"重点关注活跃用户和转化漏斗"}' \
  --as user

# 在已有 Objective 下创建一个 KR
lark-cli okr +create \
  --level key-result \
  --objective-id 7000000000000000002 \
  --content '{"text":"季度留存率提升到 45%"}' \
  --as user

# 使用 richtext 风格创建 Objective（完整 ContentBlock JSON）
lark-cli okr +create \
  --level objective \
  --cycle-id 7000000000000000001 \
  --style richtext \
  --content '{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"建立跨部门协作机制"}}]}}]}' \
  --as user

# 预览 API 调用而不实际执行
lark-cli okr +create \
  --level key-result \
  --objective-id 7000000000000000002 \
  --content '{"text":"完成 3 次核心流程优化"}' \
  --dry-run \
  --as user
```

## 参数

| 参数               | 必填 | 默认值       | 说明                                                                                                                 |
|------------------|----|-----------|--------------------------------------------------------------------------------------------------------------------|
| `--level`        | 是  | —         | 创建层级：`objective`（创建目标）\| `key-result`（在已有目标下创建 KR）                                                                 |
| `--cycle-id`     | 条件 | —         | OKR 周期 ID（int64 类型）。当 `--level=objective` 时**必填**。                                                                 |
| `--objective-id` | 条件 | —         | Objective ID（int64 类型）。当 `--level=key-result` 时**必填**。                                                             |
| `--style`        | 否  | `simple`  | 内容输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON）。请参考 [ContentBlock 格式](lark-okr-contentblock.md)。 |
| `--content`      | 是  | —         | 内容。根据 `--style` 指定格式。支持 `@文件路径` 从文件读取或 `-` 从 stdin 读取。                                                             |
| `--notes`        | 否  | —         | Objective 备注，仅 `--level=objective` 支持。根据 `--style` 指定格式，支持 `@文件路径` 或 `-` 从 stdin 读取。                               |
| `--category-id`  | 否  | —         | Objective 分类 ID，仅 `--level=objective` 支持。通常不需要传入，见下方“分类提示”。                                                        |
| `--user-id-type` | 否  | `open_id` | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`。影响 mention 中用户 ID 的解释方式。                                             |
| `--dry-run`      | 否  | —         | 预览 API 调用而不实际执行。                                                                                                   |
| `--format`       | 否  | `json`    | 输出格式。                                                                                                              |

> **分类提示**：当用户明确要求设置 Objective 分类，或创建 Objective 返回 `invalid parameters` 且怀疑租户强制开启分类时，可以配置 --category-id 参数进行创建。先运行 `lark-cli okr categories list --as user` 查看可用分类，然后选择一个语义合适且 `enabled=true` 的分类 ID 作为 `--category-id`。分类创建后可以再调整；不必因为分类选择停下等待用户确认。

## 输入格式

### `--style simple`（默认）

推荐大多数创建场景使用 `simple` 风格。`--content` 和 `--notes` 都使用 `SemiPlainContent` JSON：

```json
{
  "text": "提升北极星指标",
  "mention": ["ou_xxxxxxxx"]
}
```

规则：

- `text` 必填，且不能为空白字符串
- `mention` 可选；如果传入，数组中的每个用户 ID 都不能为空字符串
- `--notes` 仅适用于 Objective；创建 KR 时传 `--notes` 会报错
- 同一条命令只有一个 flag 可以使用 `-` 读取 stdin；如果 `--content -`，`--notes` 请使用内联 JSON 或 `@文件路径`

### `--style richtext`

当你需要精确控制段落结构、插入文档链接，或使用完整富文本块结构时，使用 `richtext` 风格：

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "建立跨部门协作机制"
            }
          }
        ]
      }
    }
  ]
}
```

规则：

- `blocks` 至少需要有一个非空段落或图片块
- 不能传空 `blocks`，也不能传只有空段落元素的内容
- 更多结构说明见 [ContentBlock 富文本格式](lark-okr-contentblock.md)

## 工作流程

1. 如果要创建 Objective，先使用 `+cycle-list` 获取目标周期的 `cycle_id`。
2. 如果要给已有 Objective 新增 KR，先通过 `+cycle-detail` 或其他 OKR 查询命令拿到 `objective_id`。
3. 选择输入风格：
   - **推荐**：`simple`，适合普通文本和 mention。
   - 需要复杂富文本时：`richtext`。
4. 执行 `lark-cli okr +create ...`。
5. 报告结果：
   - 创建 Objective 时返回新的 `objective_id`
   - 创建 KR 时返回新的 `key_result_id`，并附带父 `objective_id`

## Dry-run 对应接口

- `--level=objective`：
  - `POST /open-apis/okr/v2/cycles/:cycle_id/objectives`
- `--level=key-result`：
  - `POST /open-apis/okr/v2/objectives/:objective_id/key_results`

## 输出

### 创建 Objective 成功

```json
{
  "level": "objective",
  "objective_id": "7000000000000000002"
}
```

### 创建 KR 成功

```json
{
  "level": "key-result",
  "objective_id": "7000000000000000002",
  "key_result_id": "7000000000000000003"
}
```

## 常见错误与处理

- `--level=objective` 但未传 `--cycle-id`
  - 补充有效的周期 ID
- `--level=key-result` 但未传 `--objective-id`
  - 补充已有 Objective 的 ID
- `--content` 为空、不是合法 JSON，或内容结构为空
  - 按 `--style` 对应格式修正输入
- 在 `simple` 风格中传了 `docs` 或 `images`
  - 改用 `--style richtext`，或移除这些字段

## 何时用 +create，何时用 +batch-create

| 命令 | 适用场景 |
|------|----------|
| `+create` | 创建单个 Objective，或向已有 Objective 新增单个 KR |
| `+batch-create` | 一次创建多个 Objective，并可同时为每个 Objective 创建多个 KR |

## 参考

- [lark-okr](../SKILL.md) -- 所有 OKR 命令
- [OKR 业务实体](lark-okr-entities.md) -- Objective、KR、周期等基础概念
- [ContentBlock 格式](lark-okr-contentblock.md) -- content/notes 字段的另一种输入风格，支持完整富文本格式
- [okr +batch-create](lark-okr-batch-create.md) -- 批量创建多个 Objective / KR
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
