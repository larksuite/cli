# base +dashboard-block-get-data

> **前置条件：** 先阅读 [lark-base-dashboard.md](lark-base-dashboard.md) 了解 dashboard 整体工作流。

获取仪表盘图表组件（block）的**最终计算结果**，返回一份适合 AI 直接消费的图表协议 JSON。

这个命令适合以下场景：

1. 读取柱状图 / 条形图 / 折线图 / 饼图 / 环形图 / 面积图 / 组合图 / 散点图 / 漏斗图 / 雷达图 / 词云 / 指标卡的**实际计算结果**；
2. 把图表结果交给 AI 做后续总结、趋势解释、同比/环比说明、异常点提取；
3. 在**不读取原始记录**的前提下，直接消费图表层已经聚合好的结果；
4. 验证某个图表当前展示的数据是否符合预期。

> [!IMPORTANT]
> - 本命令返回的是**图表结果协议**，不是 block 元数据；
> - 如果你需要 `name`、`type`、`layout`、`data_config` 等配置，请先用 [`+dashboard-block-get`](lark-base-dashboard-block-get.md)；
> - 文本组件（`text`）不涉及计算，不适用本命令；

## 一句话理解

`+dashboard-block-get-data` = **拿图表“算出来的结果”**，而不是拿图表“怎么配置的”。

---

## 支持的图表类型

当前支持以下图表类型的数据计算与返回：

### 二维图表（10 种）

- 柱状图
- 条形图
- 折线图
- 饼图
- 环形图
- 面积图
- 组合图
- 散点图
- 漏斗图
- 雷达图

### 特殊类型（2 种）

- 词云
- 指标卡（statistics）

> [!CAUTION]
> 文本组件虽然也属于 dashboard block，但它不产生可计算数据，因此不会返回本协议。

---

## 推荐命令

```bash
lark-cli base +dashboard-block-get-data \
  --base-token bascn***************CtadY \
  --block-id chtxxxxxxxx
```

如果你还不知道目标 block 的 ID，典型顺序是：

```bash
# 先看仪表盘里有哪些组件
lark-cli base +dashboard-block-list \
  --base-token bascn***************CtadY \
  --dashboard-id blkxxxxxxxx

# 再读取某个组件的最终计算结果
lark-cli base +dashboard-block-get-data \
  --base-token bascn***************CtadY \
  --block-id chtxxxxxxxx
```

如果你需要先确认组件类型、名称或 `data_config`，请先执行：

```bash
lark-cli base +dashboard-block-get \
  --base-token bascn***************CtadY \
  --dashboard-id blkxxxxxxxx \
  --block-id chtxxxxxxxx
```

---

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token，标识目标多维表格 |
| `--block-id <id>` | 是 | 图表 Block ID，即目标组件的唯一标识 |
| `--format <fmt>` | 否 | 输出格式，遵循 CLI 全局输出格式规则 |
| `--dry-run` | 否 | 只预览 API 调用，不真正执行 |

> [!TIP]
> 这个命令**不需要** `--dashboard-id`。只要 `base_token + block_id` 即可定位并读取图表结果。

---

## 返回结构总览

服务端响应外层仍然是标准 OpenAPI 包装：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "dimensions": [...],
    "measures": [...],
    "main_data": [...]
  }
}
```

其中 `data` 就是 CLI 图表协议本体。不同图表类型的 `data` 结构略有不同：

| 图表类型 | 一定有 | 可能有 |
|----------|--------|--------|
| 二维图表 | `dimensions` / `measures` / `main_data` | 无 |
| 词云 | `dimensions` / `measures` / `main_data` | 无 |
| 指标卡 | `dimensions` / `measures` / `main_data` | `comparison_data` / `trend_data` |

---

## 协议字段说明

### 1) `dimensions`

维度定义数组，告诉你主结果里每个 `dim_*` key 代表什么字段。

```json
[
  {
    "field_name": "文本",
    "alias": "dim_5bKp"
  }
]
```

字段含义：

| 字段 | 说明 |
|------|------|
| `field_name` | 维度字段显示名称 |
| `alias` | 维度别名，在 `main_data` / `trend_data` 中作为 key 使用 |

### 2) `measures`

指标定义数组，告诉你每个 `me_*` key 代表什么聚合指标。

```json
[
  {
    "field_name": "Count",
    "aggregation": "count_all",
    "alias": "me_Y291bnRfYWxsX0NvdW50"
  }
]
```

字段含义：

| 字段 | 说明 |
|------|------|
| `field_name` | 统计该指标时所使用的字段名称；当 `aggregation = count_all` 时固定为 `Count`，表示统计记录总数 |
| `aggregation` | 聚合方式，常见值：`count_all` / `count` / `sum` / `avg` / `min` / `max` |
| `alias` | 指标别名，在 `main_data` / `comparison_data` / `trend_data` 中作为 key 使用 |

例如：

- 如果统计“销售额”的求和，则 `field_name = 销售额`、`aggregation = sum`
- 如果统计记录总数，则 `field_name = Count`、`aggregation = count_all`

### 3) `main_data`

主结果集。每一行都是一个对象，key 不是字段名本身，而是 `dimensions` / `measures` 中声明过的 `alias`。

```json
[
  {
    "dim_5bKp": {"value": "A"},
    "me_Y291bnRfYWxsX0NvdW50": {"value": 3}
  }
]
```

### 4) `comparison_data`

仅指标卡可能返回。表示同/环比的两个值，顺序固定为：

1. 当前周期值
2. 对比周期值

> [!NOTE]
> 原始协议里通常**不直接展示周期名称**，只提供对应的值。因此解释“同比”还是“环比”、以及比较窗口具体是什么，通常要结合组件配置或 UI 上下文理解。

### 5) `trend_data`

仅指标卡可能返回。表示时间序列趋势，每一行通常包含一个时间维度和一个指标值。

---

## alias 规则与读取方式

你不应该把 alias 当成人类可读字段名，而应把它视为**结果表里的列 ID**。

常见生成规则：

- 维度 alias：`dim_` + `base64(field_name)`
- 指标 alias：`me_` + `base64(aggregation + "_" + field_name)`

> [!NOTE]
> 为了便于阅读，本文档中的部分示例会使用**简化后的 alias**（例如 `dim_xxx`、`me_xxx` 或较短的示例值），不保证和真实返回值逐字符一致。
> 在实际读取结果时，应始终以 `dimensions` / `measures` 中声明的 alias 为准，而不要假设所有示例都严格展开成完整编码值。

例如：

```json
{
  "dimensions": [
    {"field_name": "文本", "alias": "dim_5bKp"}
  ],
  "measures": [
    {"field_name": "Count", "aggregation": "count_all", "alias": "me_xxx"}
  ],
  "main_data": [
    {
      "dim_5bKp": {"value": "A"},
      "me_xxx": {"value": 3}
    }
  ]
}
```

应解读为：

- `dim_5bKp` 对应字段“文本”，取值是 `A`
- `me_xxx` 对应指标 `count_all(Count)`，取值是 `3`

> [!TIP]
> 读取结果时，**先看 `dimensions` / `measures`，再解 `main_data`**。不要仅凭 alias 名字猜含义。

---

## 各图表类型的协议细节

### 一、二维图表

适用于：柱状图、条形图、折线图、饼图、环形图、面积图、组合图、散点图、漏斗图、雷达图。

#### 结构特征

- `dimensions`：通常有 `1~2` 个维度
  - 不分组聚合时：通常 1 个维度
  - 开启分组聚合时：通常 2 个维度
- `measures`：指标定义数组
- `main_data`：按“维度组合”展开后的行数据

#### 这类数据代表什么

二维图表返回的本质上是一张**聚合结果表**：

- 每一行代表一个维度值，或一组维度组合；
- 每一个 measure 值代表该维度下算出来的指标结果；
- 如果图表开启了分组聚合，那么每一行表示“主维度 + 分组维度”的一个组合结果；
- 如果图表是折线图、面积图这类带时间轴的图，通常可以把第一维理解为横轴、把 measure 理解为纵轴数值；
- 如果图表是饼图、环形图这类占比图，通常可以把每一行理解为一个扇区对应的分类及其数值。

换句话说，AI 在读取这类结果时，可以把它当作“按某些维度聚合后的统计明细表”，适合进一步做排序、Top N、占比解释、分组对比和趋势总结。

#### 示例 1：普通二维图表（无分组聚合）

```json
{
  "dimensions": [
    {
      "field_name": "文本",
      "alias": "dim_5bKp"
    }
  ],
  "measures": [
    {
      "aggregation": "count_all",
      "field_name": "Count",
      "alias": "me_Y291bnRfYWxsX0NvdW50"
    }
  ],
  "main_data": [
    {
      "dim_5bKp": {"value": "A"},
      "me_Y291bnRfYWxsX0NvdW50": {"value": 3}
    },
    {
      "dim_5bKp": {"value": "B"},
      "me_Y291bnRfYWxsX0NvdW50": {"value": 2}
    },
    {
      "dim_5bKp": {"value": "C"},
      "me_Y291bnRfYWxsX0NvdW50": {"value": 2}
    }
  ]
}
```

可解读为：

- 维度字段是“文本”
- 指标是“按记录总数统计”
- 当“文本”字段为 `A` 时，对应的 `Count` 指标值是 `3`
- 当“文本”字段为 `B` 时，对应的 `Count` 指标值是 `2`
- 当“文本”字段为 `C` 时，对应的 `Count` 指标值是 `2`

#### 示例 2：二维图表（开启分组聚合）

```json
{
  "dimensions": [
    {
      "field_name": "文本",
      "alias": "dim_5bKp"
    },
    {
      "field_name": "单选",
      "alias": "dim_5aSl"
    }
  ],
  "measures": [
    {
      "aggregation": "count_all",
      "field_name": "Count",
      "alias": "me_YW91bnR"
    }
  ],
  "main_data": [
    {
      "dim_5bKp": {"value": "A"},
      "dim_5aSl": {"value": "a-1"},
      "me_YW91bnR": {"value": 2}
    },
    {
      "dim_5bKp": {"value": "A"},
      "dim_5aSl": {"value": "a-2"},
      "me_YW91bnR": {"value": 1}
    },
    {
      "dim_5bKp": {"value": "B"},
      "dim_5aSl": {"value": "b-1"},
      "me_YW91bnR": {"value": 1}
    },
    {
      "dim_5bKp": {"value": "C"},
      "dim_5aSl": {"value": "c-1"},
      "me_YW91bnR": {"value": 2}
    }
  ]
}
```

可解读为：

- 第一维是“文本”，第二维是“单选”，指标是“按记录总数统计”
- 当“文本”字段为 `A`、且“单选”字段为 `a-1` 时，对应的指标值是 `2`
- 当“文本”字段为 `A`、且“单选”字段为 `a-2` 时，对应的指标值是 `1`
- 当“文本”字段为 `B`、且“单选”字段为 `b-1` 时，对应的指标值是 `1`
- 当“文本”字段为 `C`、且“单选”字段为 `c-1` 时，对应的指标值是 `2`
- 如果按“文本”字段汇总，那么“文本”字段为 `A` 时总指标值是 `3`；为 `B` 时总指标值是 `1`；为 `C` 时总指标值是 `2`

---

### 二、词云

#### 结构特征

词云协议仍然沿用 `dimensions + measures + main_data` 的结构，但语义稍有不同：

- `dimensions` 对应被分词的字段；
- `main_data` 每一行代表一个词；
- `measure` 的 value 表示按该词分组后计算出来的统计值。

#### 这类数据代表什么

词云返回的不是“原文列表”，而是**按词分组后的聚合统计结果**：

- `dimensions` 定义的是被分词的来源字段；
- `measure` 对应的是该词在当前图表统计范围内对应的统计值，具体含义取决于聚合方式和指标字段；
- `main_data` 的每一行都可以理解成“某个词 + 该词对应的统计结果”，其中该维度的具体 value 就是拆分出来的词；
- 返回结果通常已经结合图表当前过滤条件、时间范围、数据权限等上下文计算完成。

因此，AI 读取词云数据时，更适合做“关键词排序”“热点词解释”“按词聚合结果分析”“主题归纳”，而不是把它当成逐条文本记录去理解。

#### 示例

```json
{
  "dimensions": [
    {
      "field_name": "文本",
      "alias": "dim_5bKp"
    }
  ],
  "measures": [
    {
      "aggregation": "count_all",
      "field_name": "Count",
      "alias": "me_YW91bnR"
    }
  ],
  "main_data": [
    {
      "dim_5bKp": {"value": "A"},
      "me_YW91bnR": {"value": 3}
    },
    {
      "dim_5bKp": {"value": "B"},
      "me_YW91bnR": {"value": 2}
    },
    {
      "dim_5bKp": {"value": "C"},
      "me_YW91bnR": {"value": 2}
    }
  ]
}
```

可解读为：

- 被统计的分词字段是“文本”
- 当前示例里的 measure 是 `count_all(Count)`，所以这里的统计值可以理解为“按词分组后的记录总数”
- 当分词结果为 `A` 时，对应的统计值是 `3`
- 当分词结果为 `B` 时，对应的统计值是 `2`
- 当分词结果为 `C` 时，对应的统计值是 `2`
- 按统计值排序，分词结果 `A` 对应的值最高
- 分词结果 `B` 和 `C` 的统计值相同，说明它们处于同一梯队

---

### 三、指标卡（statistics）

指标卡除了主值外，还可能包含同/环比与趋势结果，是本命令里结构最特殊的一类。

#### 结构特征

- `measures`：**有且仅有一个指标**
- `main_data`：通常只有一行，表示总指标值
- `comparison_data`：可选，表示当前周期值与对比周期值
- `trend_data`：可选，表示趋势序列
- `dimensions`：可能包含同/环比日期字段、趋势日期字段

#### 这类数据代表什么

指标卡返回的核心是一个**主指标摘要**，外加可选的比较信息和趋势信息：

- `main_data` 表示当前卡片最核心、最醒目的那个主值；它通常是某个表的记录总数，或某个字段的聚合值，本身**不带时间周期概念**；
- `comparison_data` 表示用于同/环比展示的两个数值，通常是“当前周期值”和“对比周期值”；它们表示某个时间周期下的记录总数，或某个字段的聚合值；
- `trend_data` 表示这个指标在一段时间内的变化轨迹，用来支持走势判断；
- `dimensions` 在指标卡里通常不是拿来做主分组展示，而是给 `trend_data` 或同/环比相关日期字段提供语义说明。

例如：

- `main_data = 7` 可以理解为当前卡片展示的主数据，比如某张表当前总记录数是 `7`；
- `comparison_data[0] = 6` 则表示某个比较周期下的当前值，比如“本月记录总数 = 6”；
- 因此，`main_data` 与 `comparison_data[0]` **不一定相等**，因为两者表达的口径并不完全相同。

因此，AI 在解读指标卡时，应该优先回答这几个问题：

1. 当前主值是多少；
2. 和对比周期相比是上升、下降还是持平；
3. 趋势整体是增长、波动还是下滑；
4. 是否存在明显的异常峰值或低谷。

> [!NOTE]
> 当指标卡**同时指定同/环比和趋势**时，`dimensions` 中日期维度的顺序是固定的：
> 1. 第一个元素是**趋势**对应的日期维度；
> 2. 第二个元素是**同/环比**对应的日期维度。
>
> 另外要注意：`comparison_data` 自身通常**不直接携带日期字段**，它只给出“当前周期值 / 对比周期值”。
> `dimensions` 中的第一个日期维度会直接出现在 `trend_data` 中，作为趋势序列的时间列；
> 第二个日期维度则主要用于补充“该卡片配置了哪类比较相关日期字段”的语义。

#### 示例

```json
{
  "dimensions": [
    {
      "field_name": "日期",
      "alias": "dim_ZGF0ZQ"
    },
    {
      "field_name": "日期2",
      "alias": "dim_ZGF0ZTI"
    }
  ],
  "measures": [
    {
      "aggregation": "count_all",
      "field_name": "Count",
      "alias": "me_YW91b"
    }
  ],
  "main_data": [
    {
      "me_YW91b": {"value": 7}
    }
  ],
  "comparison_data": [
    {
      "me_YW91b": {"value": 6}
    },
    {
      "me_YW91b": {"value": 0}
    }
  ],
  "trend_data": [
    {
      "dim_ZGF0ZQ": {"value": "2026-01-15"},
      "me_YW91b": {"value": 1}
    },
    {
      "dim_ZGF0ZQ": {"value": "2026-01-17"},
      "me_YW91b": {"value": 1}
    },
    {
      "dim_ZGF0ZQ": {"value": "2026-03-22"},
      "me_YW91b": {"value": 1}
    },
    {
      "dim_ZGF0ZQ": {"value": "2026-04-24"},
      "me_YW91b": {"value": 2}
    },
    {
      "dim_ZGF0ZQ": {"value": "2026-05-01"},
      "me_YW91b": {"value": 1}
    }
  ]
}
```

可解读为：

- 当前主指标值 = `7`
- 当前主指标值不带时间周期概念，可理解为当前卡片主数据
- comparison_data[0] = 当前周期值 `6`，例如某个时间周期（如本月）下的统计值
- comparison_data[1] = 对比周期值 `0`
- `dimensions[0]` 对应趋势日期维度，因此实际出现在 `trend_data` 里
- `dimensions[1]` 对应同/环比相关的日期维度，用来补充比较语义
- trend_data 展示该指标随时间的变化序列
- 从 comparison_data 看，当前周期相较对比周期是上升的，并且对比周期值为 0
- 从 trend_data 看，这个指标并不是每天都有值，而是在若干离散日期出现
- 趋势序列里的最高点出现在 `2026-04-24`，值为 `2`
- 其余出现的日期大多为 `1`，说明整体上有波动，但暂时没有持续快速增长的趋势

> [!NOTE]
> `comparison_data` 只告诉你“当前值 / 对比值”，**不额外标出日期区间文本**。如果用户需要完整说明“和上周比”还是“和上月比”，通常要结合组件配置或界面上下文进一步判断。

---

## 如何正确解读返回值

建议按下面顺序阅读：

1. **先看 `dimensions`**：确认每个 `dim_*` alias 对应哪个字段；
2. **再看 `measures`**：确认每个 `me_*` alias 是什么聚合方式；
3. **最后读 `main_data` / `comparison_data` / `trend_data`**：把 alias 还原成“字段名 + 指标名”再做解释。

### 推荐解释模板

如果要把结果转成自然语言，建议不要只“复述数值”，而应尽量覆盖下面几个层次：

1. **先解释指标含义**：说明 measure 代表“记录总数”“某字段求和”“平均值”等；
2. **再给出核心结果**：明确当前主值、主要分类、主要组合或主要词项；
3. **做排序或 Top N 提炼**：指出最高、最低、前几名、同一梯队；
4. **补充分组/对比关系**：如果有第二维或 comparison_data，就说明比较对象和差异；
5. **分析趋势或异常点**：如果有时间序列，指出上升、下降、波动、峰值、低谷；
6. **最后给一句结论**：总结最值得关注的信息。

可参考下面模板：

- 二维图表：
  - 基础模板：`按 <维度字段> 统计，当前指标 <指标含义>；其中 <维度值1>=<指标值1>，<维度值2>=<指标值2> ...`
  - 增强模板：`按 <维度字段> 统计，当前指标表示 <指标含义>。从结果看，<Top1维度值> 的值最高，为 <Top1值>；<Top2维度值> 和 <Top3维度值> 紧随其后。若按 Top N 看，前 <N> 项合计贡献了 ...；若看低值项，<低值维度值> 最低，为 <低值>。整体上，<一句总结>`

- 分组聚合图表：
  - 基础模板：`按 <维度1> 统计，并以 <维度2> 分组，得到 <组合1>=<值1>，<组合2>=<值2> ...`
  - 增强模板：`当前指标表示 <指标含义>。按 <维度1> 拆分后，不同 <维度2> 组之间存在明显差异：例如 <组合1> = <值1>，<组合2> = <值2>。如果按 <维度1> 汇总，<Top1维度1值> 总值最高，为 <汇总值>；如果看组内对比，<某组> 在 <某维度1值> 下表现最强 / 最弱。整体说明 <一句总结>`

- 词云：
  - 基础模板：`按分词结果统计，当前指标表示 <指标含义>；其中 <词1>=<统计值1>，<词2>=<统计值2> ...`
  - 增强模板：`当前词云反映的是“按词分组后的 <指标含义>”。从结果看，<Top1词> 的值最高，为 <值1>，说明它是当前最突出的关键词；<Top2词>、<Top3词> 处于第二梯队。如果按 Top N 看，主要关注词集中在 <主题A>、<主题B>；如果有多个词数值接近，可归为同一热点层级。整体上，这组词更适合用来总结 <主题/热点/关注点>`

- 指标卡：
  - 基础模板：`当前主指标值为 <main_data>；当前周期值为 <comparison_data[0]>；对比周期值为 <comparison_data[1]>；趋势上 ...`
  - 增强模板：`当前主指标表示 <指标含义>，主值为 <main_data>。若看周期比较，当前周期值为 <comparison_data[0]>，对比周期值为 <comparison_data[1]>，因此整体表现为 <上升/下降/持平>。若看趋势序列，最高点出现在 <日期>，值为 <峰值>；最低点出现在 <日期>，值为 <低值>；整体走势表现为 <持续增长/阶段波动/明显回落>。如果需要给出结论，可总结为：<一句总结>`

> [!TIP]
> 当用户明确要求“帮我分析”“帮我总结”“帮我找异常 / Top N / 趋势”时，优先采用增强模板，而不是只逐条复述原始数值。

---

## 常见工作流

### 场景 1：用户要“拿这个图表当前展示的数据”

```bash
# 如果已知 block_id，直接读结果
lark-cli base +dashboard-block-get-data \
  --base-token xxx \
  --block-id chtxxxxxxxx
```

### 场景 2：用户说“帮我分析这个图表”，但你还不知道它是什么组件

```bash
# 先看组件配置，确认它是不是支持计算的图表类型
lark-cli base +dashboard-block-get \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id chtxxxxxxxx

# 再读最终计算结果
lark-cli base +dashboard-block-get-data \
  --base-token xxx \
  --block-id chtxxxxxxxx
```

### 场景 3：用户要找“仪表盘里哪个图的结果异常”

```bash
# 先列组件
lark-cli base +dashboard-block-list \
  --base-token xxx \
  --dashboard-id blk_xxx

# 再针对可疑 block 逐个取结果
lark-cli base +dashboard-block-get-data \
  --base-token xxx \
  --block-id chtxxxxxxxx
```

---

## 何时优先用这个命令

- 用户说“帮我拿这个图表算出来的数据 / 结果 / 指标”
- 用户已经知道 `block_id`，目标是**读取结果**而不是看配置
- 用户后续还要让 AI 对图表结果做解释、归纳、比较、总结
- 你只关心图表层的聚合产出，不需要回到底表逐条读记录

## 何时不要误用

- 想看 block 的 `data_config`、名称、类型、布局 → 用 `+dashboard-block-get`
- 想列出仪表盘里有哪些组件 → 用 `+dashboard-block-list`
- 想修改或新建组件 → 用 `+dashboard-block-update` / `+dashboard-block-create`
- 想看原始记录明细，而不是图表聚合结果 → 回到 `record-*`
- 目标是文本组件 → 本命令不适用

---

## 常见误区

### 误区 1：把这个命令当成“获取 block 详情”

不是。这个命令不返回：

- block 名称
- block 类型
- layout
- `data_config`
- 所属 dashboard 信息

这些都应该通过 [`+dashboard-block-get`](lark-base-dashboard-block-get.md) 获取。

### 误区 2：以为它返回的是原始记录

不是。它返回的是**图表聚合后的最终结果**。如果图表本身做了过滤、分组、聚合、时间窗口限制，返回值反映的是图表视角，不是原始表全量明细。

### 误区 3：直接把 alias 当真实字段名读

不应该。alias 只是协议里的键，必须结合 `dimensions` / `measures` 还原语义。

### 误区 4：看到指标卡的 `comparison_data` 就以为已经知道“同比/环比周期文本”

不一定。它只给出比较值，不一定给出周期标签。若要精确解释比较窗口，通常还需要组件配置或 UI 上下文。

---

## dry-run 用途

可用来确认最终会调用的接口路径：

```bash
lark-cli base +dashboard-block-get-data \
  --base-token bascn_example_token \
  --block-id chtxxxxxxxx \
  --dry-run \
  --format pretty
```

你应能看到类似：

```text
GET /open-apis/base/v3/bases/bascn_example_token/dashboards/blocks/chtxxxxxxxx/data
```

适合在以下场景使用：

- 校验 `base_token` / `block_id` 是否传对；
- 调试 agent 生成的命令；
- 编写自动化测试时确认请求结构。

---

## 参考

- [lark-base-dashboard.md](lark-base-dashboard.md) — dashboard 模块总指引
- [lark-base-dashboard-block-get.md](lark-base-dashboard-block-get.md) — 获取 block 元数据
- [dashboard-block-data-config.md](dashboard-block-data-config.md) — data_config 结构和组件类型说明
