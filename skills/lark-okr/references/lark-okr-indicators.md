# OKR 量化指标管理

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

管理 OKR 目标（Objective）和关键结果（Key Result）的量化指标，包括查询和更新指标。

> **快速更新当前值：** 如果只需要更新指标的当前值，推荐使用 shortcut [`okr +indicator-update`](lark-okr-indicator-update.md)，无需手动查询指标 ID。
>
> 本指南中的原生 API 适用于需要修改指标其他字段（如 `unit`、`target_value`、`status_calculate_type` 等）的场景。

## 草稿读写

- 原生查询通过 `--params '{"prefer_draft":true}'` 草稿优先读取；`false` 仅读取发布态。更新通过 `--data` 中的 `apply_to_draft` 选择写入态。两者的接口默认值均为 `false`，请显式传值。
- `apply_to_draft=true` 更新草稿指标；所属 Objective 尚无草稿时，会先为整个 Objective 创建草稿。草稿与发布态指标共用 ID，更新前应读取对应视图，检查该视图的指标配置。
- 返回的 `indicator.is_draft` 表示本次读取的来源：`true` 为草稿，`false` 为发布态，缺失为未知。以所属 Objective 的状态判断是否已有草稿，不能仅凭指标返回发布态就断言无草稿。
- 草稿仅对 OKR 所有者本人的用户身份和具备相应应用数据权限的 bot 身份可见。写入、确认和发布均按 [草稿工作流](lark-okr-drafts.md) 执行；指标随所属 Objective 一同发布或撤销。

以下示例使用草稿；仅在草稿工作流允许直接修改发布态时，将读取的 `prefer_draft` 和更新的 `apply_to_draft` 都改为 `false`。

---

## 指标字段说明

| 字段                          | 类型   | 说明                                                                 |
|-----------------------------|------|--------------------------------------------------------------------|
| `is_draft` | boolean | 本次返回是否为草稿；缺失表示未知。 |
| `id`                        | string | 指标 ID（更新时需要）                                                     |
| `entity_id` / `entity_type` | string/int | 所属实体 ID 和类型（2=目标，3=关键结果）                                   |
| `current_value`             | number | 当前值                                                                 |
| `target_value`              | number | 目标值                                                                 |
| `start_value`               | number | 起始值                                                                 |
| `indicator_status`          | int    | 状态：-1=未定义，0=正常，1=有风险，2=已延期                                   |
| `status_calculate_type`     | int    | 状态计算方式：0=手动更新，1=基于进度和当前时间自动更新，2=基于风险最高的 KR 状态更新       |
| `current_value_calculate_type` | int | 当前值计算方式：0=手动更新，1=基于 KR 进度自动更新（目标），2=基于拆解 KR 进度更新（KR） |
| `unit`                      | object | 单位，包含 `unit_type`（0=公共，1=自定义）和 `unit_value`（如 PERCENT、YUAN 等）       |
| `owner`                     | object | 所有者                                                                 |

---

## 一、查询目标的量化指标

### 命令

```bash
lark-cli okr objective.indicators list --objective-id "<目标ID>" [flags]
```

### 常用示例

```bash
# 获取目标的量化指标
lark-cli okr objective.indicators list \
  --objective-id 7000000000000000001 \
  --params '{"prefer_draft":true}' \
  --as user

# 指定用户 ID 类型
lark-cli okr objective.indicators list \
  --objective-id 7000000000000000001 \
  --params '{"prefer_draft":true}' \
  --user-id-type "user_id" \
  --as user
```

### 参数

| 参数                   | 必填 | 默认值            | 说明                                                  |
|----------------------|----|----------------|-----------------------------------------------------|
| `--objective-id`     | 是  | —              | 目标 ID                                               |
| `--user-id-type`     | 否  | `open_id`      | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`     |
| `--department-id-type` | 否 | `open_department_id` | 部门 ID 类型：`open_department_id` \| `department_id` |
| `--params` | 否 | — | JSON query 参数；用 `prefer_draft=true` 草稿优先读取，`false` 仅读发布态。 |

### 返回

返回 `indicator` 字段，包含该目标的量化指标详情。

示例返回值:
有进度时:
```json 
{
   "ok": true,
   "identity": "user",
   "data": {
      "indicator": {
         "create_time": "1782835200000",    // 创建时间
         "current_value": 60,               // 当前值
         "current_value_calculate_type": 0, // 当前值计算方式 0(手动更新)|2(按KR计算)|3(按拆解计算)。 仅当此处为 0 时，允许使用 patch API 更新当前值
         "entity_id": "7000000000000000001",// 指标挂载的 Objective/KR id
         "entity_type": 2,                  // 指标挂载在 Objective还是KR 上 2(Objective)|3(KR)
         "id": "7000000000000000002",       // 指标本身的 ID
         "is_draft": true,                  // 本次返回草稿态指标
         "indicator_status": 0,             // 指标状态 -1(未定义)|0(正常)|1(有风险)|2(延期)
         "owner": {                         // 指标归属的用户
            "owner_type": "user",
            "user_id": "ou_xxx"
         },
         "start_value": 0,                  // 起始值, 默认0
         "status_calculate_type": 0,        // 状态计算方式
         "target_value": 100,               // 目标值, 默认 100
         "unit": {                          // 指标单位，默认是公共的百分比
            "unit_type": 0,                 // 单位类型 0(公共)|1(自定义)
            "unit_value": "PERCENT"         // 单位名
         },
         "update_time": "1782835200000"     // 更新时间
      }
   }
}
```
默认初始进度:
```json 
{
   "ok": true,
   "identity": "user",
   "data": {
      "indicator": {
         "create_time": "1782835200000",
         "entity_id": "7000000000000000001",
         "entity_type": 2,
         "id": "7000000000000000002",
         "is_draft": true,
         "indicator_status": -1,
         "owner": {
            "owner_type": "user",
            "user_id": "ou_xxx"
         },
         "status_calculate_type": 0,
         "update_time": "1782835200000"
      }
   }
}
```

默认初始进度不携带 start_value/current_value/target_value/unit 等信息，若直接设置当前值，则使用百分比作为默认单位。
由于默认单位为百分比，当一定要计算数值时，可以视作 0%，但是向用户汇报默认初始进度时，应当明确对应的 O/KR 未设置进度这一点，以和真正的 0% 区别开。

---

## 二、查询关键结果的量化指标

### 命令

```bash
lark-cli okr key_result.indicators list --key-result-id "<关键结果ID>" [flags]
```

### 常用示例

```bash
# 获取关键结果的量化指标
lark-cli okr key_result.indicators list \
  --key-result-id "7652569715131075780" \
  --params '{"prefer_draft":true}' \
  --as user
```

### 参数

| 参数                   | 必填 | 默认值            | 说明                                                  |
|----------------------|----|----------------|-----------------------------------------------------|
| `--key-result-id`    | 是  | —              | 关键结果 ID                                            |
| `--user-id-type`     | 否  | `open_id`      | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`     |
| `--department-id-type` | 否 | `open_department_id` | 部门 ID 类型：`open_department_id` \| `department_id` |
| `--params` | 否 | — | JSON query 参数；用 `prefer_draft=true` 草稿优先读取，`false` 仅读发布态。 |

### 返回

返回 `indicator` 字段，包含该关键结果的量化指标详情。

---

## 三、更新量化指标

### 命令

```bash
lark-cli okr indicators patch --indicator-id "<指标ID>" --data '<JSON>'
```

### 常用示例

```bash
# 更新指标的当前值（手动更新方式）, 以操作草稿指标为例
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data '{"current_value": 75.5, "current_value_calculate_type": 0, "apply_to_draft": true}' \
  --as user

# 更新指标状态为"有风险"（需 status_calculate_type=0）
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data '{"indicator_status": 1, "status_calculate_type": 0, "apply_to_draft": true}' \
  --as user

# 更新关键结果指标的目标值和单位
lark-cli okr indicators patch \
  --indicator-id "ind-456" \
  --data '{
    "target_value": 100,
    "unit": {"unit_type": 0, "unit_value": "PERCENT"},
    "apply_to_draft": true
  }' \
  --as user

# 从文件读取请求体
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data @indicator_update.json \
  --as user
```

### 参数

| 参数               | 必填 | 说明                                                                 |
|------------------|----|--------------------------------------------------------------------|
| `--indicator-id` | 是  | 指标 ID（从 list 接口获取）                                             |
| `--data`         | 是  | JSON 请求体，包含要更新的字段。支持 `@文件路径` 从文件读取。                        |
| `--user-id-type` | 否  | 用户 ID 类型                                                           |

### 请求体字段

根据需要更新的字段选择传入，支持增量更新：

| 字段                          | 类型   | 适用实体 | 说明                                                                 |
|-----------------------------|------|------|--------------------------------------------------------------------|
| `apply_to_draft` | bool | 全部 | `true` 更新草稿指标，所属 Objective 尚无草稿时先创建其草稿；`false` 更新发布态。通过 `--data` 显式传入。 |
| `current_value`             | number | 全部   | 当前值，范围 -99999999999 到 99999999999                                  |
| `current_value_calculate_type` | int  | 全部   | 当前值计算方式：0=手动，1=基于 KR 进度（目标），2=基于拆解 KR 进度（KR）              |
| `indicator_status`          | int    | 全部   | 状态：-1=未定义，0=正常，1=有风险，2=已延期。仅 `status_calculate_type=0` 时可修改      |
| `status_calculate_type`     | int    | 全部   | 状态计算方式：0=手动，1=自动（进度+时间），2=自动（最高风险 KR）。目标支持 0/1/2，KR 支持 0/1 |
| `start_value`               | number | KR    | 起始值。目标不支持修改                                                   |
| `target_value`              | number | KR    | 目标值。目标不支持修改；有承接记录的 KR 不支持修改                                  |
| `unit`                      | object | KR    | 单位。目标不支持修改；有承接记录的 KR 不支持修改                                  |

### 单位 (`unit`) 格式

```json
{
  "unit": {
    "unit_type": 0,              // 0=公共单位，1=自定义单位
    "unit_value": "PERCENT"      // 公共单位枚举：PERCENT、NONE、YUAN、DOLLAR；自定义单位：最长5字符
  }
}
```

### 限制说明

- **目标指标**：不支持修改 `start_value`、`target_value`、`unit`
- **关键结果指标**：有承接记录的 KR 不支持修改 `target_value`、`unit`
- **自动计算的指标**：`current_value_calculate_type != 0` 时，不能手动修改 `current_value`
- **自动状态的指标**：`status_calculate_type != 0` 时，不能手动修改 `indicator_status`

---

## 完整工作流示例

### 场景：更新关键结果的指标当前值和状态

1. **查询关键结果的指标**（获取 `indicator_id` 和当前配置）
   ```bash
   lark-cli okr key_result.indicators list \
     --key-result-id 7652569715131075780 \
     --params '{"prefer_draft":true}' \
     --as user
   ```

2. **检查指标配置**，确认：
   - `current_value_calculate_type` 为 0（手动更新）才能修改 `current_value`
   - `status_calculate_type` 为 0（手动更新）才能修改 `indicator_status`

3. **更新指标**
   ```bash
   lark-cli okr indicators patch \
     --indicator-id "ind-123" \
     --data '{"current_value":65.0,"current_value_calculate_type":0,"indicator_status":1,"status_calculate_type":0,"apply_to_draft":true}' \
     --as user
   ```

4. **验证更新结果**
   ```bash
   lark-cli okr key_result.indicators list \
     --key-result-id 7652569715131075780 \
     --params '{"prefer_draft":true}' \
     --as user
   ```

### 场景：修改关键结果指标的目标值和单位

```bash
# 1. 查询获取 indicator_id
lark-cli okr key_result.indicators list --key-result-id 7652569715131075780 --params '{"prefer_draft":true}' --as user

# 2. 更新目标值和单位
lark-cli okr indicators patch \
  --indicator-id 7652569715131075781 \
  --data '{"target_value":500,"unit":{"unit_type":0,"unit_value":"YUAN"},"apply_to_draft":true}' \
  --as user
```

以上工作流仅更新草稿。验证后按 [草稿工作流](lark-okr-drafts.md) 发布所属 Objective，使指标变更生效；不能单独发布指标。
若直接更改发布态(携带 "apply_to_draft":false)，则无需另行发布

## 参考

- [lark-okr](../SKILL.md) -- 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
- [okr +indicator-update](lark-okr-indicator-update.md) -- 快捷更新指标当前值（推荐）
