# OKR 量化指标管理

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

管理 OKR 目标（Objective）和关键结果（Key Result）的量化指标，包括查询和更新指标。

> **快速更新当前值：** 如果只需要更新指标的当前值，推荐使用 shortcut [`okr +indicator-update`](lark-okr-indicator-update.md)，无需手动查询指标 ID。
>
> 本指南中的原生 API 适用于需要修改指标其他字段（如 `unit`、`target_value`、`status_calculate_type` 等）的场景。

---

## 指标字段说明

| 字段                          | 类型   | 说明                                                                 |
|-----------------------------|------|--------------------------------------------------------------------|
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
  --objective-id 7000000000000000001

# 指定用户 ID 类型
lark-cli okr objective.indicators list \
  --objective-id 7000000000000000001 \
  --user-id-type "user_id"
```

### 参数

| 参数                   | 必填 | 默认值            | 说明                                                  |
|----------------------|----|----------------|-----------------------------------------------------|
| `--objective-id`     | 是  | —              | 目标 ID                                               |
| `--user-id-type`     | 否  | `open_id`      | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`     |
| `--department-id-type` | 否 | `open_department_id` | 部门 ID 类型：`open_department_id` \| `department_id` |

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
  --key-result-id "7652569715131075780"
```

### 参数

| 参数                   | 必填 | 默认值            | 说明                                                  |
|----------------------|----|----------------|-----------------------------------------------------|
| `--key-result-id`    | 是  | —              | 关键结果 ID                                            |
| `--user-id-type`     | 否  | `open_id`      | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`     |
| `--department-id-type` | 否 | `open_department_id` | 部门 ID 类型：`open_department_id` \| `department_id` |

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
# 更新指标的当前值（手动更新方式）
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data '{"current_value": 75.5, "current_value_calculate_type": 0}'

# 更新指标状态为"有风险"（需 status_calculate_type=0）
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data '{"indicator_status": 1, "status_calculate_type": 0}'

# 更新关键结果指标的目标值和单位
lark-cli okr indicators patch \
  --indicator-id "ind-456" \
  --data '{
    "target_value": 100,
    "unit": {"unit_type": 0, "unit_value": "PERCENT"}
  }'

# 从文件读取请求体
lark-cli okr indicators patch \
  --indicator-id "ind-123" \
  --data @indicator_update.json
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
     --key-result-id 7652569715131075780
   ```

2. **检查指标配置**，确认：
   - `current_value_calculate_type` 为 0（手动更新）才能修改 `current_value`
   - `status_calculate_type` 为 0（手动更新）才能修改 `indicator_status`

3. **更新指标**
   ```bash
   lark-cli okr indicators patch \
     --indicator-id "ind-123" \
     --data '{"current_value":65.0,"current_value_calculate_type":0,"indicator_status":1,"status_calculate_type":0}'
   ```

4. **验证更新结果**
   ```bash
   lark-cli okr key_result.indicators list \
     --key-result-id 7652569715131075780
   ```

### 场景：修改关键结果指标的目标值和单位

```bash
# 1. 查询获取 indicator_id
lark-cli okr key_result.indicators list --key-result-id 7652569715131075780

# 2. 更新目标值和单位
lark-cli okr indicators patch \
  --indicator-id 7652569715131075781 \
  --data '{"target_value":500,"unit":{"unit_type":0,"unit_value":"YUAN"}}'
```

## 参考

- [lark-okr](../SKILL.md) -- 所有 OKR 命令
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
- [okr +indicator-update](lark-okr-indicator-update.md) -- 快捷更新指标当前值（推荐）
