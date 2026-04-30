# 图片准备 (Image Preparation)

> 本文件说明如何在画板 DSL 中使用图片节点。进入任何含图片的场景前，必须先完成图片准备流程。

## 概述

画板 DSL 支持 `type: 'image'` 节点，但图片不能直接使用 URL，必须先上传到飞书获取 **media token**，然后在 DSL 中引用。

**关键约束**：
- 图片 token 必须通过 `docs +media-upload --parent-type whiteboard` 上传获取
- 图片必须上传到**目标画板**（`--parent-node` 设为目标画板 token），跨画板的 token 不可用
- `drive +upload` 获取的 Drive file token **不能**用于画板图片节点

## Step 0：图片准备流程

### 1. 下载图片

用 `curl` 下载图片到本地。**必须使用能根据关键词返回相关图片的图片源**。

**推荐图片源**：

| 图片源类型 | 说明 |
|-------|------|
| 免费版权图库 | 支持按关键词搜索，图片无版权风险（CC0 或类似协议），图库种类丰富（人物/动物/风景/美食/建筑等），关键词能精准匹配图片内容 |
| 直接 URL | 用户提供或已知的图片链接，最可靠 |

**选择图库的必要条件**：
- **版权合规**：图片必须无版权纠纷风险，避免使用需要付费授权或有使用限制的图库
- **关键词搜索**：支持按关键词搜索并返回相关图片，确保图片内容与主题匹配
- **内容丰富**：图库图片种类多、数量大，能覆盖常见主题（宠物、美食、景点、产品等）

```bash
curl -L -o photo1.jpg "<图片URL>"
curl -L -o photo2.jpg "<图片URL>"
```

**严禁使用随机占位图服务**：某些图库仅提供随机占位图，URL 中的关键词参数不会影响返回的图片内容，下载的图片与主题完全无关。

### 2. 校验图片

```bash
ls -l *.jpg   # 确认每张文件大小不同；若大小相同则内容可能重复，需重新下载
```

**图片内容审查（必须执行）**：
- 下载完成后，确认文件是真实图片而非 HTML 错误页：若某张图片大小 < 1KB，很可能是下载失败返回了 HTML 错误页，需重新下载
- **图片内容正确性只能在渲染后验证**：生成 DSL 并本地渲染 PNG 后，必须查看渲染结果，确认每张图片内容与主题相关（如宠物主题的图片确实是宠物，而非建筑/风景等不相关内容）
- 若发现图片内容与主题不符，必须用更精确的关键词重新下载并重新上传

### 3. 上传到目标画板

**必须**使用 `docs +media-upload --parent-type whiteboard` 上传：

```bash
lark-cli docs +media-upload --file ./photo1.jpg --parent-type whiteboard --parent-node <whiteboard_token>
# 响应: { "file_token": "<media_token>", ... }
```

逐张上传，收集每个 media token：

```bash
lark-cli docs +media-upload --file ./photo1.jpg --parent-type whiteboard --parent-node <whiteboard_token>  # → <media_token_1>
lark-cli docs +media-upload --file ./photo2.jpg --parent-type whiteboard --parent-node <whiteboard_token>  # → <media_token_2>
lark-cli docs +media-upload --file ./photo3.jpg --parent-type whiteboard --parent-node <whiteboard_token>  # → <media_token_3>
```

### 4. 在 DSL 中引用

```json
{ "type": "image", "id": "img-1", "width": 240, "height": 160, "image": { "src": "<media_token_1>" } }
```

## 常见错误

| 错误现象 | 原因 | 解决 |
|---------|------|------|
| 画板 API 返回 500（2891001） | 使用了 Drive file token 而非 media token | 改用 `docs +media-upload --parent-type whiteboard` |
| 画板 API 返回 500 | 图片上传到了其他画板 | 重新上传到目标画板 |
| 图片裂开/无法显示 | token 无效或已过期 | 重新上传获取新 token |
| 图片内容与主题无关 | 使用了随机占位图服务 | 改用免费版权图库服务 |
