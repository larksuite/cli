# Base 模板中心

模板中心是一个**公开的 Base 模板库**。当用户想“用一个现成的模板快速搭一个多维表格”时，这套命令帮助 AI 找到最合适的模板，最终通过 `+base-copy` 复制成用户自己的新 Base。

三个命令：

- `+template-categories`：列出所有模板分类，用于把用户意图对齐到某个类目。
- `+template-list`：列出某个分类下的模板（不传分类则返回“推荐”类目）。
- `+template-search`：按关键词搜索模板。

## 何时使用模板中心

满足以下特征时走模板中心：用户有**创建新 Base 的意图**，但**没有指向已有对象的锚点**（没有 Base URL、没有“我的/最近访问的表”、没有具体已存在的 Base 名）。

典型触发：

- “帮我建一个 CRM 多维表格”
- “有没有适合项目管理的模板”
- “找个 OKR 跟进的 Base 模板照着做”

**不要**走模板中心的情况（即使用户嘴上说“模板”）：

- 用户给了 Base/Wiki 链接或 token → 走 `+url-resolve`。
- 用户说“我之前那张表 / 我的模板 / 最近访问的” → 走 `+title-resolve` 或转 `lark-drive` 搜索。
- 用户要从零定义字段 schema，而不是套现成模板 → 走 `+base-create --table-name --fields`。

模板中心是独立的公开数据集，**不能**用 `drive +search` 找到，`drive +search` 只搜用户自己可访问的云空间对象。

## 推荐命令

```bash
# 列出所有模板分类
lark-cli base +template-categories --as user

# 列出某个分类下的模板（不传 --category-key 则返回“推荐”类目）
lark-cli base +template-list --category-key template_center_tab_ai --limit 10 --as user

# 按关键词搜索模板
lark-cli base +template-search --keyword "项目管理" --limit 10 --as user

# 翻页：把上一页返回的 offset 原样传给 --offset
lark-cli base +template-search --keyword "AI" --limit 10 --offset <上一页返回的 offset> --as user

# 选定模板后，用模板 token 复制成用户自己的新 Base
lark-cli base +base-copy --base-token <模板 token> --name "<新 Base 名>" --as user
```

## 工作流

模板中心有两条路径，按用户意图明确程度二选一，不要盲目全用。

### 路径 A：分类浏览（意图偏宽泛时首选）

用户只给了一个大方向（如“项目管理”“市场营销”），先按分类收敛，再在类目里挑模板。

1. `+template-categories` 列出全部分类，拿到 `categories[].key` 和 `name`。
2. AI 把用户意图匹配到最贴近的一个分类 `name`，取它的 `key`。
3. `+template-list --category-key <key>` 列出该类目下的模板。
4. 读每个模板的 `name` / `introduction` / `scenarios`，挑出最符合用户场景的那个，拿它的 `token`。
5. 用 `+base-copy --base-token <token>` 基于模板复制出新 Base（见下文“基于模板创建”）。

```bash
# 1. 看有哪些分类
lark-cli base +template-categories --as user

# 2~3. 匹配到“AI 应用”类目后，列出该类目模板
lark-cli base +template-list --category-key template_center_tab_ai --limit 10 --as user
```

匹配不到贴切分类，或用户意图本身就跨类目 / 很具体时，改走路径 B。

### 路径 B：关键词搜索（意图有具体词时首选）

用户给了明确、可检索的词（如“财务报销”“直播复盘”“AI 客服”），直接搜，不必先看分类。

1. `+template-search --keyword "<词>"` 搜模板。
2. 同样读 `name` / `introduction` / `scenarios` 选模板，拿 `token`。
3. `+base-copy` 复制。

```bash
lark-cli base +template-search --keyword "项目管理" --limit 10 --as user
```

关键词不能为空；空搜会被拒绝。用户只有“大方向”而没有具体检索词时，用路径 A 的分类浏览更稳。

### 分类 vs 搜索怎么选

| 用户意图 | 走哪条 |
|---|---|
| 只有大类方向（“市场营销类的”“办公用的”） | 路径 A，先 `+template-categories` 收敛 |
| 有具体、可检索的业务词（“报销”“OKR”“直播”） | 路径 B，直接 `+template-search` |
| 大方向下没挑到合适的 | A 之后再用 B 换关键词补搜 |

## 翻页

`+template-list` 和 `+template-search` 都是游标翻页：

- `--limit`：每页数量，默认 10，范围 1-100；`--page-size` 是等价别名。
- `--offset`：翻页游标，来自上一次响应的 `offset` 字段。**首次请求不要传**。
- 响应里 `has_more=true` 表示还有下一页，把响应的 `offset` 原样传给下一次 `--offset`。`has_more=false` 或 `offset` 为空字符串表示没有更多。

`--offset` 是服务端返回的不透明游标，不要解析它、不要自己拼造。

```bash
lark-cli base +template-search --keyword "AI" --limit 10 --offset <上一页返回的 offset> --as user
```

## 数据结构

### TemplateCategory（分类对象）

`+template-categories` 返回 `categories[]`，每个元素：

| 字段 | 类型 | 含义 |
|---|---|---|
| `key` | string | 分类唯一标识，形如 `template_center_tab_ai`（`template_center_tab_` 前缀 + 类目名）。传给 `+template-list --category-key` 用的就是它 |
| `name` | string | 分类展示名，如 `AI 应用` / `办公通用`。AI 匹配用户意图时看这个 |

### Template（模板对象）

`+template-list` / `+template-search` 返回 `templates[]`，每个元素：

| 字段 | 类型 | 含义 |
|---|---|---|
| `token` | string | 模板的 Base token，是模板的唯一标识。基于模板创建时作为 `+base-copy --base-token` 的入参 |
| `name` | string | 模板名称，如 `工作汇报` |
| `introduction` | string | 模板介绍，说明模板用途、内容结构和适用方向。AI 判断模板是否契合用户需求主要看它 |
| `scenarios` | string[] | 适用场景列表，如 `["工作汇报","月报","项目进展"]`，用于快速判断场景匹配度 |
| `developer` | string | 模板开发者，如 `飞书` |
| `link` | string | 模板预览链接，可展示给用户，但复制模板用 `token` 而不是 `link` |
| `created_at` / `updated_at` | string | 创建 / 更新时间 |

列表 / 搜索响应还带分页字段：

| 字段 | 类型 | 含义 |
|---|---|---|
| `has_more` | boolean | 是否还有下一页 |
| `offset` | string | 下一页游标；无更多时为空字符串 |

**约定**：模板的唯一标识就叫 `token`（模板 Base token），不要在输出或转述里改名成 `id` 或 `key`；`key` 是分类的标识（`category_key`）。

### 模板列表/模版搜索-响应示例

```json
{
  "code": 0,
  "data": {
    "has_more": true,
    "offset": "1",
    "templates": [
      {
        "created_at": "2025-12-03T02:53:34Z",
        "developer": "Base Team",
        "introduction": "📊 品牌调研问卷  \n高效收集用户反馈，助力品牌优化决策  \n\n核心功能点  \n1 预设多维度调研问题模板  \n2 支持自定义问题类型与逻辑跳转  \n3 实时数据统计与可视化分析  \n\n适合场景  \n1 新品上市前市场需求调研  \n2 品牌形象与用户满意度评估  \n3 竞品对比与消费者偏好分析",
        "link": "https://example.com/base/<template_token>",
        "name": "品牌调研问卷",
        "scenarios": ["运营管理", "市场营销"],
        "token": "<template_token>",
        "updated_at": "2026-06-22T08:18:58Z"
      }
    ]
  },
  "msg": ""
}
```

读取模板列表时重点看：

- `templates[].name`：模板名称；基于模板创建 Base 且用户没有指定新名称时，直接作为 `+base-copy --name`。
- `templates[].token`：模板 Base token；复制时传给 `+base-copy --base-token`。
- `templates[].link`：模板预览链接；可以展示给用户帮助确认，但复制时不要用 link 代替 token。
- `templates[].introduction` / `templates[].scenarios`：用于判断模板是否匹配用户业务场景。
- `data.offset`：下一页游标；只有 `has_more=true` 时才继续传给 `--offset`。

## 基于模板创建 Base

模板中心只负责“找到模板”，它本身不创建 Base。选定模板后，用模板的 `token` 复制出用户自己的新 Base：

```bash
lark-cli base +base-copy --base-token <模板 token> --name "<新 Base 名>" --as user
```

- `--name` 用用户想要的新 Base 名；不传则沿用模板名。
- 只有用户明确说“只要结构 / 不要内容”时，才加 `--without-content`。
- `+base-copy` 的返回和权限说明见 SKILL.md 中 `+base-copy` 的相关规则。

## 注意事项

- 三个命令都是只读，默认 `--as user`，所需权限 `base:template:read`。
- 模板中心是公开数据集，不能用 `drive +search` 找到；用户要“我的/最近访问/已有 Base”不要走这里。
- 分类先于列表：`+template-list` 的 `--category-key` 必须来自 `+template-categories` 的返回，不要凭空猜类目 key。
- `+template-search` 不支持空关键词，会被拒绝；用户只有大方向、无具体检索词时改走分类浏览。
- 模板的唯一标识是 `token`（模板 Base token），不要改名成 `id` 或 `key`。
- `--offset` 是服务端返回的不透明游标，翻页时原样回传，不要解析或自行构造。
- 模板中心只查模板、不创建 Base；创建一律走 `+base-copy --base-token <token>`，不要用模板 token 去调 `+base-get` 之类的当前用户 Base 命令。
