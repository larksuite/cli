---
name: lark-okr
version: 1.0.0
description: "飞书 OKR：管理目标与关键结果。查看和编辑 OKR 周期、目标、关键结果、对齐关系、量化指标和进展记录。当用户需要查看或创建 OKR、管理目标和关键结果、查看对齐关系时使用。不负责：待办任务管理（lark-task）、日程/会议安排（lark-calendar）、绩效评估"
metadata:
  requires:
    bins: [ "lark-cli" ]
  cliHelp: "lark-cli okr --help"
---

# okr (v2)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**身份**：OKR 操作默认使用 `--as user`（查看当前用户/上下级的 OKR 时）。也支持 `--as bot` 查看他人 OKR（需相应权限，仅在用户明确要求时才使用）。

## 快速决策

| 用户需求                     | 操作路径                                                                                                                | 参考文档                                                                                                                                                                                                             |
|------------------------------|-------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 查看自己/他人的 OKR          | 获取用户 ID -> `+cycle-list` -> `+cycle-detail` -> 按需查指标/进展记录                                                  | [`cycle-list`](references/lark-okr-cycle-list.md), [`cycle-detail`](references/lark-okr-cycle-detail.md), [`indicators`](references/lark-okr-indicators.md), [`progress-list`](references/lark-okr-progress-list.md) |
| 为自己写一组 OKR             | 优先用 `+batch-create` 创建 Objective/KR 骨架                                                                           | [`batch-create`](references/lark-okr-batch-create.md), [`contentblock`](references/lark-okr-contentblock.md)                                                                                                         |
| 只新增一条 O 或单条 KR       | 用 `+create`                                                                                                            | [`create`](references/lark-okr-create.md)                                                                                                                                                                            |
| 编辑内容/备注/截止时间       | 用 `+patch`                                                                                                             | [`patch`](references/lark-okr-patch.md)                                                                                                                                                                              |
| 修改 OKR 分数                | 只有用户明确说“分数”“评分”“打分”“score”时才用 `+patch --score --apply-to-draft=false`；草稿不支持 score                  | [`patch`](references/lark-okr-patch.md)                                                                                                                                                                              |
| 调整顺序或权重               | 用 `+reorder` / `+weight`                                                                                               | [`reorder`](references/lark-okr-reorder.md), [`weight`](references/lark-okr-weight.md)                                                                                                                               |
| 更新数字进度/完成度          | 百分比或不带单位数字用 `+indicator-update`；需要改单位/目标值时查指标后用 `indicators patch`                            | [`indicator-update`](references/lark-okr-indicator-update.md), [`indicators`](references/lark-okr-indicators.md)                                                                                                     |
| 写文字进展                   | 用 `+progress-create`；如果文本和数字都有，百分比或默认单位可使用 `--progress-percent` 统一改，非百分比单位更新量化指标 | [`progress-create`](references/lark-okr-progress-create.md), [`progress-list`](references/lark-okr-progress-list.md), [`progress-update`](references/lark-okr-progress-update.md)                                    |
| 对齐目标                     | 直接按对齐关系工作流处理                                                                                                | [`alignments`](references/lark-okr-alignments.md)                                                                                                                                                                    |
| 查询/创建/修改/解决 OKR 评论 | 获取周期下全部评论聚合用 `+comment-detail`，查询单个 O/KR/进展或仅查询周期全局评论用 `+comment-list`；                | [`comment`](references/lark-okr-comment-list.md), [`comment-create`](references/lark-okr-comment-create.md), [`comment-solve-reopen`](references/lark-okr-comment-solve-reopen.md)                                   |
| 发布或撤销 Objective 草稿    | 发布用 `+publish-draft`；确认要放弃整份 Objective 草稿后用 `+drop-draft --yes`                                           | [`drafts`](references/lark-okr-drafts.md)                                                                                                                                                                            |

## 草稿操作决策（写操作前必须执行）

先用 `+cycle-detail --prefer-draft=true` 读取目标内容，以所属 Objective 的 `is_draft` 判断有无草稿。只有当前身份可编辑对应的 OKR 时，`true` 才表示已有草稿，`false` 才可判断无草稿。
  - 以 user 身份访问时，仅有归属于当前登陆用户的 OKR 才可读取到草稿，他人的 OKR 总是返回发布态。

支持草稿的写操作中，`--apply-to-draft=true` 会在所属 Objective 尚无草稿时自动创建其草稿，无需先手动进入草稿态。

- 用户明确要求“不介入”“无需确认”“不要二次确认”或直接完成发布态变更时，使用 `--apply-to-draft=false` 直接操作发布态；
- 其他场景使用 `--apply-to-draft=true` 编辑草稿，展示草稿结果并等待用户确认。只在用户看过当前草稿后明确要求发布时，才调用 `+publish-draft`。
- 一旦本次任务选择了草稿流程，除非有明确指示（如“发布已有的草稿”），否则不应当在无用户确认的情况下发布草稿。
- 若直接操作发布态时相关 Objective 已有草稿，不得自动发布或撤销该草稿；保留草稿，并说明它之后发布时可能覆盖本次发布态修改。
- `+drop-draft` 会撤销整个 Objective 聚合的草稿，不支持仅撤销单个 KR；执行前必须确认，并显式传 `--yes`。
- 评论、文字进展、进展记录和进展图片始终操作发布态，不适用上述草稿参数。

完整流程见 [`OKR 草稿工作流`](references/lark-okr-drafts.md)。命令示例必须显式传 `--prefer-draft=true|false` 或 `--apply-to-draft=true|false`，不要依赖 CLI 默认值。

分类只在用户明确要求分类，或创建 Objective 返回 `invalid parameters` 且怀疑租户强制开启分类时处理：用 `lark-cli okr categories list --params '{"owner_type":"user","page_size":100}' --as user` 查可用分类，选择语义合适且`enabled=true` 的分类 ID；分类可后续调整，不必停下等待用户确认。

获取当前用户用 `contact +get-user`；按姓名/邮箱查他人用 `contact +search-user`，拿到 `open_id` 后再查 OKR。

```bash
lark-cli contact +search-user --query "张三" --has-chatted --as user
```

最常用 OKR 命令示例：

```bash
# 查用户周期，再用周期 ID 查详情
lark-cli okr +cycle-list --user-id "ou_xxx" --as user
lark-cli okr +cycle-detail --cycle-id 7000000000000000001 --prefer-draft=true --as user

# 批量创建 Objective/KR
lark-cli okr +batch-create \
  --cycle-id 7000000000000000001 \
  --input '[{"text":"提升产品用户体验","notes":"关注核心流程和用户反馈","krs":[{"text":"核心流程满意度达到 4.8 分"}]}]' \
  --apply-to-draft=true \
  --as user

# 更新数字进度/完成度
lark-cli okr +indicator-update \
  --level key-result \
  --id 7000000000000000003 \
  --value 75 \
  --apply-to-draft=true \
  --as user
```

分数和进度不要混用：用户说“进度”“完成度”“当前做到 75%”时，通常是在改量化指标或写进展记录，不是在改 `score`。只有明确要求修改 OKR 分数/评分/打分时，才使用 [`+patch --score --apply-to-draft=false`](references/lark-okr-patch.md)；`score` 取值是 0-1，最多一位小数，草稿态不支持修改或展示 score。

进度判断规则：用户说“进度”“完成度”时，先判断是否是量化数字。数字进度通常对应量化指标；不可量化文本对应进展记录。需要修改指标单位时看 [`lark-okr-indicators.md`](references/lark-okr-indicators.md)

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli okr +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut                                                         | 说明                                                                                                          |
|------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| [`+cycle-list`](references/lark-okr-cycle-list.md)               | 分页获取特定用户的 OKR 周期列表，可以用 `--time-range` 对当前页后置筛选                                       |
| [`+cycle-detail`](references/lark-okr-cycle-detail.md)           | 获取特定 OKR 中所有目标和关键结果的内容                                                                       |
| [`+create`](references/lark-okr-create.md)                       | 创建单个 Objective（可带备注），或向已有 Objective 新增 KR                                                    |
| [`+progress-list`](references/lark-okr-progress-list.md)         | 分页获取目标或关键结果的进展记录列表                                                                          |
| [`+progress-get`](references/lark-okr-progress-get.md)           | 根据 ID 获取单条 OKR 进展记录                                                                                 |
| [`+progress-create`](references/lark-okr-progress-create.md)     | 为目标或关键结果创建进展记录                                                                                  |
| [`+progress-update`](references/lark-okr-progress-update.md)     | 更新指定 ID 的进展记录内容                                                                                    |
| [`+progress-delete`](references/lark-okr-progress-delete.md)     | 删除指定 ID 的进展记录（不可恢复）                                                                            |
| [`+upload-image`](references/lark-okr-image-upload.md)           | 上传图片用于 OKR 进展记录的富文本内容                                                                         |
| [`+batch-create`](references/lark-okr-batch-create.md)           | 批量创建 Objective（可带备注）和 KR                                                                           |
| [`+reorder`](references/lark-okr-reorder.md)                     | 调整 Objective 或 KR 的顺位                                                                                   |
| [`+weight`](references/lark-okr-weight.md)                       | 调整 Objective 或 KR 的权重                                                                                   |
| [`+indicator-update`](references/lark-okr-indicator-update.md)   | 更新 Objective 或 KR 的当前进度指标。更复杂的量化指标操作见 [量化指标管理](references/lark-okr-indicators.md) |
| [`+patch`](references/lark-okr-patch.md)                         | 部分更新 Objective 或 KR（content、notes、score、deadline）                                                   |
| [`+publish-draft`](references/lark-okr-drafts.md)                | 发布整个 Objective 草稿聚合                                                                                   |
| [`+drop-draft`](references/lark-okr-drafts.md)                   | 撤销整个 Objective 草稿聚合（高风险，需 `--yes`）                                                             |
| [`+comment-detail`](references/lark-okr-comment-detail.md)       | 获取周期下 Cycle/Objective/KeyResult/Progress 的全部评论                                                      |
| [`+comment-list`](references/lark-okr-comment-list.md)           | 分页获取单个 OKR 实体下的评论                                                                                 |
| [`+comment-get`](references/lark-okr-comment-get.md)             | 获取单条评论详情                                                                                              |
| [`+comment-create`](references/lark-okr-comment-create.md)       | 创建新评论或回复已有评论(仅支持 --as user)                                                                    |
| [`+comment-patch`](references/lark-okr-comment-patch.md)         | 修改评论内容(仅支持 --as user)                                                                                |
| [`+comment-delete`](references/lark-okr-comment-delete.md)       | 永久删除单条评论(仅支持 --as user)                                                                            |
| [`+comment-solve`](references/lark-okr-comment-solve-reopen.md)  | 解决评论或划词评论串(仅支持 --as user)                                                                        |
| [`+comment-reopen`](references/lark-okr-comment-solve-reopen.md) | 重新打开评论或划词评论串(仅支持 --as user)                                                                    |

### 创建场景选择

- **单条创建优先用 [`+create`](references/lark-okr-create.md)**：适合创建一个 Objective，或给已有 Objective 增加一个 KR。
- **批量创建用 [`+batch-create`](references/lark-okr-batch-create.md)**：适合一次创建多个 Objective，并可同时附带多个 KR。
- 如果你只需要修改已有 Objective / KR 的内容、备注、分数或截止时间，使用 [`+patch`](references/lark-okr-patch.md)。

## 格式说明

- [`OKR 业务实体`](references/lark-okr-entities.md) 获取 OKR 实体结构，定义和关系，帮助你更好的使用 OKR 功能
- [`ContentBlock 富文本格式`](references/lark-okr-contentblock.md) — Objective/KeyResult/Progress 中 Content/Note
  字段使用的富文本格式说明，以及简化的半纯文本（SemiPlainContent）格式的进一步说明。
- **强烈建议** 在操作 OKR 前，阅读[`OKR 业务实体`](references/lark-okr-entities.md)以了解基础概念

## API Resources

带草稿选择参数的原生接口按以下规则显式选择状态：

- 读取接口若暴露 `prefer_draft`，可使用 `--prefer-draft=true` 草稿优先读取草稿态（无草稿态的内容则获取对应的发布态），使用 `--prefer-draft=false` 仅获取发布态内容。
- 写入或删除接口若暴露 `apply_to_draft`，使用 `--apply-to-draft=true` 操作草稿，使用 `--apply-to-draft=false` 操作发布态。

### alignments

- `delete` — 删除对齐关系；可通过 `--apply-to-draft=true|false` 选择操作草稿态还是发布态
- `get` — 获取对齐关系；可通过 `--prefer-draft=true|false` 选择是否优先获取草稿态数据

> **操作指南：** [OKR 对齐关系管理](references/lark-okr-alignments.md) 包含 list/create/delete 完整工作流

### categories

- `list` — 批量获取分类

### cycles

- `list` — 批量获取用户周期

### cycle.objectives

- `create` — 创建目标；可通过 `--apply-to-draft=true|false` 选择是否创建草稿 Objective
  - 建议直接使用 [lark-okr-create.md](references/lark-okr-create.md) 或 [lark-okr-batch-create.md](references/lark-okr-batch-create.md)
- `list` — 批量获取用户周期下的目标；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据
  - 建议直接使用 [lark-okr-cycle-detail.md](references/lark-okr-cycle-detail.md)

### indicators

- `patch` — 更新量化指标；草稿选择：`--apply-to-draft=true|false`

> **操作指南：** [OKR 量化指标管理](references/lark-okr-indicators.md) 包含目标/KR 指标查询和 patch 更新完整工作流

### key_results

- `delete` — 删除关键结果；可通过 `--apply-to-draft=true|false` 选择是否删除草稿 KR
  - 草稿态/发布态下的 KR 互不干涉，尝试删除在对应状态下不存在的 KR 时，会返回 1001004: okr data not found 错误
- `get` — 获取关键结果；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据
- `patch` — 更新关键结果；可通过 `--apply-to-draft=true|false` 选择是否编辑草稿 KR
  - 建议直接使用 [lark-okr-patch.md](references/lark-okr-patch.md)

### key_result.indicators

- `list` — 获取关键结果的量化指标；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据

> **操作指南：** [OKR 量化指标管理](references/lark-okr-indicators.md)

### objectives

- `delete` — 删除目标，会一并删除草稿和发布态 objective，仅草稿的 objective 亦可删除
- `get` — 获取目标；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据
- `key_results_position` — 更新全部关键结果的位置；可通过 `--apply-to-draft=true|false` 选择是否操作草稿数据
    - 请求中必须携带对应周期下全部关键结果的 ID，否则会参数校验失败。以传入的关键结果ID顺序重新排列关键结果。
    - 建议直接使用 [lark-okr-reorder.md](references/lark-okr-reorder.md)
- `key_results_weight` — 更新全部关键结果的权重；可通过 `--apply-to-draft=true|false` 选择是否操作草稿数据
    - 类似 `objectives_weight`, 请求中必须同时修改对应目标下全部关键结果的权重，且所有权重值的和必须等于 1 ，否则会参数校验失败。
    - 建议直接使用 [lark-okr-weight.md](references/lark-okr-weight.md)
- `patch` — 更新目标；可通过 `--apply-to-draft=true|false` 选择是否编辑草稿 Objective
    - 建议直接使用 [lark-okr-patch.md](references/lark-okr-patch.md)

### objective.alignments

- `create` — 创建对齐关系；可通过 `--apply-to-draft=true|false` 选择是否创建草稿对齐
    - 对齐不允许对齐自己的目标，且发起对齐的目标和被对齐的目标所在周期时间上必须有重叠，否则会参数校验失败。
- `list` — 批量获取目标下的对齐关系；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据
- 可参考 [lark-okr-alignments.md](references/lark-okr-alignments.md) 了解工作流详情

### objective.indicators

- `list` — 获取目标的量化指标；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据

### objective.key_results

- `create` — 创建关键结果；可通过 `--apply-to-draft=true|false` 选择是否创建草稿 KR
  - 建议直接使用 [lark-okr-create.md](references/lark-okr-create.md) 或 [lark-okr-batch-create.md](references/lark-okr-batch-create.md)
- `list` — 批量获取目标下的关键结果；可通过 `--prefer-draft=true|false` 选择是否优先读取草稿数据

## 不在本 skill 范围

- 待办任务管理 → 使用 [`lark-task`](../lark-task/SKILL.md)
- 日程安排 → 使用 [`lark-calendar`](../lark-calendar/SKILL.md)
- 绩效评估 → 使用 [`lark-openapi-explorer`](../lark-openapi-explorer/SKILL.md) 查找原生接口


