# slides +replace-slide（块级替换 / 插入）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

对指定 slide 做块级替换或插入。编辑已有 PPT 的主路径——`slide_id` 不变、页序不动、只影响被指定的块。

相比直接调 `xml_presentation.slide.replace`，这个 shortcut 的四个额外价值：

1. `--presentation` 接受 `xml_presentation_id` / `/slides/` URL / `/wiki/` URL（wiki 自动解析）；
2. `block_replace` 的 `replacement` 根元素 `id="<block_id>"` 由 CLI 自动注入——底层 API 的硬约束（不注入返回 3350001）；直接调原生 API 需自己加，用 Shortcut 则自动注入；
3. `<shape>` 元素缺少 `<content/>` 子元素时由 CLI 自动注入——SML 2.0 schema 要求每个 `<shape>` 必须有 `<content/>` 子元素，缺失同样触发 3350001；自闭合的 `<shape .../>` 也会被自动展开为 `<shape ...><content/></shape>`；
4. 3350001 错误时提供上下文感知的 hint，帮助 AI agent 和用户快速定位原因。

## 命令

```bash
# block_insert：在页末追加一个新元素
lark-cli slides +replace-slide --as user \
  --presentation slidesXXXXXXXXXXXXXXXXXXXXXX \
  --slide-id pfG \
  --parts '[{"action":"block_insert","insertion":"<shape type=\"rect\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"100\"/>"}]'

# block_replace：已知某块 id，整块替换（replacement 根 id 自动注入为 bUn）
lark-cli slides +replace-slide --as user \
  --presentation slidesXXXXXXXXXXXXXXXXXXXXXX \
  --slide-id pfG \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'

# 大 --parts 走文件或 stdin（auto-gen 命令不支持 @file，但 shortcut 支持）
lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts @parts.json
cat parts.json | lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts -

# wiki URL 直接传（CLI 自动 get_node → 拿真实 xml_presentation_id）
lark-cli slides +replace-slide --as user \
  --presentation "https://xxx.feishu.cn/wiki/wikcnXXXXXX" --slide-id pfG \
  --parts '[{"action":"block_insert","insertion":"<shape type=\"rect\" width=\"100\" height=\"100\"/>"}]'

# 预览（不实际调用）
lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts "$PARTS" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL |
| `--slide-id` | 是 | 页面 ID（`xml_presentation.slide.get` / `xml_presentations.get` 都能拿到） |
| `--parts` | 是 | JSON 数组（`[{...}, ...]`），单次最多 200 条。支持 `@<file>` 和 `-`（stdin）读取 |
| `--comment` | 否 | 操作备注（出现在版本历史） |
| `--revision-id` | 否 | 乐观锁；默认 `-1` 表示基于最新版执行，传具体版本号则期间有变更会冲突 |
| `--tid` | 否 | 并发事务 ID；多人协作长事务才用，单次单人调用留空 |

## parts 元素结构

> **限制**：单次 `--parts` 中所有条目必须是同一种 action（全 `block_replace` 或全 `block_insert`），混合会触发 3350001，需拆为两次调用。最多 200 条。**其他 action（含 `str_replace`）CLI 会直接报错拒绝**。

每条 part 按 `action` 取不同字段：

### action = `block_replace`

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | `"block_replace"` |
| `block_id` | 是 | 目标块的 3 位 short element ID（从 `slide.get` 返回 XML 里读） |
| `replacement` | 是 | 新 XML 片段；**根元素 `id` 会被 CLI 自动注入为 `block_id`**，用户不用自己加（如果已经加了且不一致会被覆盖为正确值） |

### action = `block_insert`

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | `"block_insert"` |
| `insertion` | 是 | 要插入的 XML 片段 |
| `insert_before_block_id` | 否 | 插到这个块之前；省略（不提供此字段）则追加到页末 |

## 返回值

```json
{
  "xml_presentation_id": "slidesXXXXXXXXXXXXXXXXXXXXXX",
  "slide_id": "pfG",
  "parts_count": 1,
  "revision_id": 102
}
```

| 字段 | 说明 |
|------|------|
| `xml_presentation_id` | 解析后的真实 token（wiki URL 解析后会变化） |
| `slide_id` | 与入参一致 |
| `parts_count` | 本次提交的 parts 条数 |
| `revision_id` | 成功后的新版本号，下次做乐观锁时用 |
| `failed_part_index` | 有部分失败时存在，指向第几条 part 失败 |
| `failed_reason` | 失败原因文字描述 |

整批作为原子事务：任一 part 失败则整批不生效，服务端通过 `failed_part_index` / `failed_reason` 告诉你是哪条；按此定位修正后重发。

## 使用流程

### 给已有页加图（典型场景）

```bash
PID=xxx
SID=yyy

# 1) 上传图片
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png --presentation "$PID" | jq -r '.data.file_token')

# 2) block_insert 到页末
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")}]')"
```

### 改标题（block_replace）

```bash
# 先拿原页 XML，从里面找到标题块的 3 位 short id（如 bUn）
lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}"

# block_replace 换掉整个标题块（id 自动注入）
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'
```

### 批量同类 action：一次替换多个块

同类 action 可以放进一个 `--parts` 批量执行（原子事务）。**不同 action 不能混用**——例如"换标题（`block_replace`）+ 加图（`block_insert`）"需要拆成两次调用。

```bash
# 同一批次只做 block_replace：一次替换标题块 + 正文块
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[
    {"action":"block_replace","block_id":"bab","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"},
    {"action":"block_replace","block_id":"bac","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"220\" width=\"800\" height=\"120\"><content textType=\"body\"><p>副标题</p></content></shape>"}
  ]'

# 若还需追加图片，再单独发一次 block_insert
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[{"action":"block_insert","insertion":"<img src=\"<file_token>\" topLeftX=\"700\" topLeftY=\"400\" width=\"180\" height=\"100\"/>"}]'
```

### 乐观锁

```bash
# 读时记录 revision_id
REV=$(lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}" \
  | jq -r '.revision_id')

# 写时传 --revision-id；期间有人改过就冲突（HTTP 400/409）
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" --revision-id "$REV" \
  --parts "$PARTS"
```

## 常见错误

| 现象 | 原因 | 对策 |
|------|------|------|
| `failed_part_index=i`, `block not found` | `parts[i].block_id` 在当前页不存在 | 重新 `slide.get` 拿最新 XML，按里面的 short ID 再填 |
| HTTP 400/409，信息含 revision | `--revision-id` 冲突 | 重读拿最新 `revision_id`；或用 `-1` 强制 |
| `--parts[i] action "str_replace" is not supported` | CLI 不暴露 `str_replace` | 把替换需求改写成 `block_replace` / `block_insert` |
| `--parts contains N items, exceeds maximum of 200` | 一次提交 parts 太多 | 拆多次调用 |
| `--parts[i] (block_replace) requires non-empty block_id` / `replacement` | 字段缺失 | 按 parts 元素结构补齐 |
| `<img>` 不显示 / 显示破图 | `src` 写了外链 URL | 换成通过 [`+media-upload`](lark-slides-media-upload.md) 拿到的 `file_token` |
| 3350001 | `replacement` 不是合法单根 XML 片段，或 `block_id` 不存在 | CLI 已自动注入 `id` 和 `<content/>`；如果仍报错，重新 `slide.get` 拿最新 XML 确认 `block_id` 存在；检查 XML 结构是否合法；注意混合 `block_replace`+`block_insert` 不支持，需拆分 |
| 403 | 权限不足 | 需要 `slides:presentation:update` 或 `slides:presentation:write_only`；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- [xml_presentation.slide get](lark-slides-xml-presentation-slide-get.md) — 读原页拿 `block_id` / `revision_id`
- [xml_presentation.slide replace](lark-slides-xml-presentation-slide-replace.md) — 底层 replace API 参考
- [+media-upload](lark-slides-media-upload.md) — 上传图片拿 `file_token`
- [lark-slides-edit-workflows.md](lark-slides-edit-workflows.md) — 读-改-写闭环 + 决策树
