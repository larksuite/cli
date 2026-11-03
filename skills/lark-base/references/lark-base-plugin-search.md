# Base 插件搜索

使用 `lark-cli base +plugin-search` 在当前调用身份可见范围内只读搜索 Base 插件。命令支持 `user` 和 `bot` 身份。

## 使用边界

- 适用于查找数据连接器插件、侧边栏插件、字段捷径插件、工作流插件、仪表盘插件和 BaseApp 应用。
- 搜索结果只表示候选，不表示插件已经安装、授权或可以执行。

## 准备输入

执行搜索前：

1. 从用户需求中提取与插件能力有关的部分。可以保留理解需求所需的非敏感业务概念、字段含义或脱敏示例，但不要直接带入原始记录值、批量数据、访问凭证、用户信息或其他敏感数据。
2. 根据用户意图选择一个或多个插件类型；无法判断时省略 `--plugin-types`，搜索全部支持类型。
3. 根据需要设置返回数量；`--limit` 表示单次搜索期望返回的最大候选数，范围为 1～20，默认 10。

搜索不绑定具体 Base，不需要 `base_token`；不要为了执行搜索而先调用 `+url-resolve`、`+title-resolve` 或猜测一个 Base。

| 用户意图 | `--plugin-types` |
| --- | --- |
| 外部系统导入、同步、周期拉取 | `data_connector` |
| 侧边栏插件 | `side_view` |
| 字段捷径插件、公式模板、AI 字段 | `field_shortcut` |
| 工作流或自动化节点插件 | `workflow` |
| 仪表盘插件 | `dashboard_widget` |
| BaseApp 应用 | `base_app` |

## 改写查询

将口语化或依赖对话上下文的表达改写为简短检索词。每条查询尽量只保留 1～2 个区分度最高的关键词；需求明确时使用一条查询，需要覆盖不同表达时可以生成少量同语义查询，不为凑数量做重复改写。

1. **消解指代：** 结合最近一轮与搜索有关的上下文，找出“这个”“刚才那个”“能直接写进字段吗”等表达实际指向的产品、对象或能力。
2. **保留关键词：** 优先保留产品名、业务对象或区分能力所必需的约束；删除礼貌用语、背景铺垫和“插件”“多维表格”等通用词。没有的信息不要猜。
3. **同语义改写：** 保持用户意图不变，只调整同义词、常见别称或表达方式。

数据同步场景只使用外部数据源的产品名作为查询词，例如 `MySQL`、`Jira`、`Salesforce`；不要追加“同步”“连接器”“多维表格”等通用词。

例如，上文讨论根据商品图片自动生成描述，用户接着问“有没有能直接写进字段的？”，可以改写为：

- AI 图片理解
- 商品图片描述

根据该需求，将 `--plugin-types` 设为 `field_shortcut`。

每条查询必须满足以下约束：

- 保持同一用户意图，不增加用户没有提出的能力。
- 尽量只保留 1～2 个区分度最高的关键词，不直接使用完整需求句。
- 只有在区分候选所必需时才保留业务对象或硬约束。
- 不虚构插件名称、开发者或产品能力。
- 不生成假想插件介绍（HyDE），避免把不存在的能力带入召回。

同一业务目标涉及多类能力时，可以为 `--plugin-types` 传入多个类型。彼此独立的需求分别搜索，不把无关需求拼成长查询。

## 执行搜索

对每条改写后的查询分别调用一次命令；各查询互不依赖时可以并行执行。

```bash
lark-cli base +plugin-search \
  --keyword "<改写后的检索词>" \
  --plugin-types data_connector \
  --limit 10 \
  --as user
```

- 只有一个类型时传单个值；多个类型使用英文逗号分隔或重复 `--plugin-types`。
- 搜索全部类型时省略 `--plugin-types`。
- `--keyword` 传改写后的单条查询，不传整段对话或原始长文本。
- `+plugin-search` 支持 `user` 和 `bot` 身份；默认按 Base Skill 的身份选择规则优先使用 `--as user`，用户明确要求应用身份时使用 `--as bot`。

## 理解和合并结果

命令成功时返回标准 JSON 响应。下面是一次 `partial=true` 的响应结构示例：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "partial": true,
    "items": [
      {
        "plugin_id": "anycross_3025843a23ecadd5",
        "plugin_type": "data_connector",
        "name": "MySQL 连接器",
        "description": "同步 mysql 数据",
        "developer": "飞书打卡",
        "introduction": "同步 mysql 数据",
        "is_official": false,
        "is_featured": false,
        "detail_url": "<plugin_detail_url>"
      }
    ]
  }
}
```

| 字段 | 含义 |
| --- | --- |
| `ok` | 命令是否执行成功。 |
| `identity` | 本次调用使用的身份，取值为 `user` 或 `bot`。 |
| `data` | 插件搜索结果。 |
| `data.partial` | 是否只有部分召回结果可用；`true` 表示至少一路召回失败，`false` 表示本次召回完整。 |
| `data.items` | 服务端已经融合并排序的候选插件数组；数组顺序就是服务端返回顺序。 |
| `data.items[].plugin_id` | 插件标识，用于识别插件以及跨查询结果去重。 |
| `data.items[].plugin_type` | 插件类型，对应请求中的 `--plugin-types` 枚举。 |
| `data.items[].name` | 插件名称。 |
| `data.items[].description` | 插件的简短描述。 |
| `data.items[].developer` | 插件开发者名称。 |
| `data.items[].introduction` | 插件的详细介绍。 |
| `data.items[].is_official` | 是否被标记为官方插件。 |
| `data.items[].is_featured` | 是否被标记为精选插件。 |
| `data.items[].detail_url` | 插件详情页链接，可能为空或缺失。 |

处理规则：

1. 保留每次调用中服务端给出的 `data.items` 顺序，不再排序。
2. 合并多次查询结果时，按 `plugin_id` 去重，并保留插件首次出现时的相对顺序。
3. 向用户推荐候选时，若 `detail_url` 非空，可以将插件名称渲染为该链接，或单独附上“查看详情”；字段缺失或为空时省略，不自行拼接或猜测链接。
4. `data.partial=true` 表示本次结果不完整，但已经返回的候选仍可使用；即使 `data.items` 为空，也不能据此断定没有匹配插件。
5. `data.items` 为空时，可以保留核心产品、对象和动作，去掉次要约束后放宽一次查询；仍为空则停止继续扩写。
6. 命令失败表示本次搜索失败，不要解释成没有匹配插件。
