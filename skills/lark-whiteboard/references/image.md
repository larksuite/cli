# 图片准备 (Image Preparation)

> 本文件说明如何在画板 DSL 中使用图片节点。进入任何含图片的场景前，必须先完成图片准备流程。

## 概述

画板 DSL 支持 `type: 'image'` 节点，但图片不能直接使用 URL 或其他域的 token，**必须先上传到目标画板获取 `whiteboard` 域 media token**，然后在 DSL 中引用。

**核心规则**：不管图片从哪来（本地文件、URL、文档中的 `docx_image` token、其他域的 Drive token），都必须通过 `docs +media-upload --parent-type whiteboard --parent-node <目标画板token>` 上传，拿到画板专属的 media token 后才能在 DSL 中使用。直接使用非 `whiteboard` 域的 token 会导致画板 API 报 500（错误码 2891001）或图片在文档中消失。

## Step 0：图片准备流程

### 1. 获取图片到本地

根据图片来源选择对应方式：

| 图片来源 | 获取方式 |
|---------|---------|
| 本地文件 | 直接使用 |
| 网络 URL | `curl -L -o photo.jpg "<URL>"` |
| 文档中的图片 token | `lark-cli docs +media-download --token <token> --output ./photo.png` |
| 其他域的 Drive token | `lark-cli docs +media-download --token <token> --output ./photo.png` |

**图片源选择（需要搜索图片时）**：

| 图片源类型 | 说明 |
|-------|------|
| 免费版权图库 | 支持按关键词搜索，图片无版权风险（CC0 或类似协议），图库种类丰富（人物/动物/风景/美食/建筑等），关键词能精准匹配图片内容 |
| 直接 URL | 用户提供或已知的图片链接，最可靠 |

**选择图库的必要条件**：
- **版权合规**：图片必须无版权纠纷风险，避免使用需要付费授权或有使用限制的图库
- **关键词搜索**：支持按关键词搜索并返回相关图片，确保图片内容与主题匹配
- **内容丰富**：图库图片种类多、数量大，能覆盖常见主题（宠物、美食、景点、产品等）

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
| 画板 API 返回 500（2891001） | 使用了非 `whiteboard` 域 token（如 `docx_image`、Drive file token） | 下载图片后用 `docs +media-upload --parent-type whiteboard` 重新上传 |
| 画板 API 返回 500 | 图片上传到了其他画板 | 重新上传到目标画板 |
| 画板在文档中图片消失 | 图片 token 的资源域与画板不匹配 | 确保图片通过 `--parent-type whiteboard --parent-node <画板token>` 上传 |
| 图片裂开/无法显示 | token 无效或已过期 | 重新上传获取新 token |
| 图片内容与主题无关 | 使用了随机占位图服务 | 改用免费版权图库服务 |
