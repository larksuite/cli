# 提测说明 — `feat/slide_update`

## 一、新功能：`+replace-slide` 快捷命令

**背景**：原来编辑已有幻灯片页面的唯一方法是整页替换（先 create 新页、再 delete 旧页），操作繁琐且容易出错。本次新增 `+replace-slide`，支持**元素级局部替换**，无需整页重建。

### 命令参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | ✅ | xml_presentation_id、slides URL 或 wiki URL（自动解析） |
| `--slide-id` | ✅ | 目标页面的 slide_id |
| `--parts` | ✅ | JSON 数组，每条为一个替换操作，最多 200 条 |
| `--comment` | ❌ | 可选操作备注 |
| `--revision-id` | ❌ | 乐观锁版本号，默认 -1（取最新） |
| `--tid` | ❌ | 并发编辑事务 id，通常为空 |

### 支持两种操作（`action`）

**`block_replace`** — 用新 XML 替换指定 block_id 的元素

```json
[{
  "action": "block_replace",
  "block_id": "bUn123",
  "replacement": "<shape type=\"text\"><content><p>新内容</p></content></shape>"
}]
```

**`block_insert`** — 在指定位置插入新元素

```json
[{
  "action": "block_insert",
  "insertion": "<shape type=\"rect\" topLeftX=\"80\" topLeftY=\"80\" width=\"200\" height=\"100\"/>",
  "insert_before_block_id": "bUn456"
}]
```

### CLI 自动补全（用户无感知）

| 行为 | 说明 |
|------|------|
| 自动注入 `id` | `block_replace` 时自动把 `block_id` 写入 replacement 根元素的 `id` 属性，已有 id 则覆盖为正确值 |
| 自动注入 `<content/>` | `<shape>` 元素缺少 `<content/>` 子节点时自动补上（SML 2.0 协议要求，否则后端报 3350001） |
| 3350001 错误提示增强 | 返回上下文相关的 hint（如 block_id 不存在、mixed action 等），便于排查 |

### 主动拒绝的操作

- `str_replace`：后端支持但 CLI 主动屏蔽，返回明确报错并提示改用 `block_replace`

---

## 二、`--presentation` 支持 wiki URL

`+replace-slide` 和已有快捷命令一样，`--presentation` 接受三种输入：

| 输入形式 | 示例 |
|---------|------|
| 裸 token | `pres_abc` |
| slides URL | `https://xxx.feishu.cn/slides/pres_abc` |
| wiki URL | `https://xxx.feishu.cn/wiki/wikcn_abc` |

wiki URL 会自动调 `wiki.v2.spaces.get_node` 解析为真实 `xml_presentation_id`，并校验 `obj_type` 必须是 `slides`，否则报错。

---

## 三、lark-slides Skill 文档重构

SKILL.md 做了大幅精简重组，把原来内嵌的 Workflow / 布局建议 / jq 模板等大段内容拆到独立 reference 文档，主文件只保留命令列表和 scope 映射表。

### 新增 / 更新的 reference 文档

| 文档 | 变更 | 说明 |
|------|------|------|
| `references/lark-slides-replace-slide.md` | 新增 | `+replace-slide` 完整使用文档 |
| `references/lark-slides-edit-workflows.md` | 新增 | 元素级编辑和整页替换的选型指南 |
| `references/examples.md` | 新增 | 覆盖创建 / 局部替换 / 插图等场景的完整示例 |
| `references/lark-slides-xml-presentation-slide-get.md` | 新增 | `slide.get` 原生 API 使用说明 |
| `references/lark-slides-xml-presentation-slide-replace.md` | 新增 | `slide.replace` 原生 API 使用说明 |
| `references/lark-slides-media-upload.md` | 更新 | 补充与 `+replace-slide` 配合使用的场景 |

---

## 四、测试重点

### 4.1 `+replace-slide` 基本流程

| # | 用例 | 预期 |
|---|------|------|
| 1 | `block_replace` 正常执行 | 请求成功，返回 `xml_presentation_id` / `slide_id` / `revision_id` |
| 2 | `block_insert` 正常执行（带 `insert_before_block_id`） | 请求成功，`insert_before_block_id` 正确传给后端 |
| 3 | `block_insert` 正常执行（不带 `insert_before_block_id`） | 请求成功，body 中无 `block_id` 字段 |
| 4 | 后端返回 `failed_part_index` / `failed_reason` | CLI 输出中透传这两个字段 |

### 4.2 自动 id 注入

| # | 用例 | 预期 |
|---|------|------|
| 5 | replacement XML 根元素无 `id` | 发出的请求体中根元素携带 `id="<block_id>"` |
| 6 | replacement XML 根元素已有正确 `id` | 保持不变，不重复注入 |
| 7 | replacement XML 根元素有错误 `id` | 覆盖为正确的 `block_id` |

### 4.3 自动 `<content/>` 注入

| # | 用例 | 预期 |
|---|------|------|
| 8 | replacement 为自闭合 `<shape/>` | 转为 `<shape id="..."><content/></shape>` |
| 9 | replacement 为无 content 的 open shape | 在根标签后注入 `<content/>` |
| 10 | replacement 已有 `<content/>` | 保持不变 |
| 11 | replacement 为 `<img>` / `<table>` 等非 shape 元素 | 不注入 `<content/>` |

### 4.4 wiki URL 解析

| # | 用例 | 预期 |
|---|------|------|
| 12 | `--presentation` 传 wiki URL，目标是 slides 类型 | 先调 get_node，用 obj_token 发替换请求 |
| 13 | `--presentation` 传 wiki URL，目标不是 slides 类型 | 报错，提示 obj_type 不符 |

### 4.5 错误与边界

| # | 用例 | 预期 |
|---|------|------|
| 14 | `action` 为 `str_replace` | 报错，提示改用 `block_replace` |
| 15 | `action` 为未知值 | 报错，提示 supported actions |
| 16 | `block_replace` 缺少 `block_id` | 报错，指明缺少字段 |
| 17 | `block_replace` 缺少 `replacement` | 报错，指明缺少字段 |
| 18 | `block_insert` 缺少 `insertion` | 报错，指明缺少字段 |
| 19 | `--parts` 为空数组 `[]` | 报错，提示至少 1 条 |
| 20 | `--parts` 超过 200 条 | 报错，提示超出上限 |
| 21 | `--parts` 非法 JSON | 报错，提示 invalid JSON |

### 4.6 3350001 错误提示增强

| # | 用例 | 预期 hint 关键词 |
|---|------|----------------|
| 22 | 单纯 `block_replace` 触发 3350001 | `common causes`（block_id 不存在 / 坐标越界等） |
| 23 | 单纯 `block_insert` 触发 3350001 | `common causes` |
| 24 | mixed `block_replace` + `block_insert` 触发 3350001 | `mixed block_replace+block_insert` |
| 25 | 其他错误码（非 3350001） | 不附加 slides 专属 hint |

### 4.7 dry-run

| # | 用例 | 预期 |
|---|------|------|
| 26 | `--dry-run` 正常输出 | 展示请求 URL（含 `slide_id` query param）和注入 id 后的完整 body，不发真实请求 |
| 27 | wiki URL + `--dry-run` | 展示两步编排（get_node → replace），presentationID 显示为占位符 |
