# 图片资源处理

## 图片需求识别

**触发条件（严格）**：仅当用户**显式要求**使用图片时，才使用 image 节点。触发关键词：

> 图片、配图、插图、照片、真实图片、实拍、放一张图、加个图、嵌入图片

**不触发的情况**：即使主题涉及旅行、美食、产品、人物等视觉性内容，只要用户没有显式说要「图片/配图/插图」，就**一律不使用 image 节点**，用文字 + 形状 + icon 来呈现即可。

识别到图片需求后，先完成下方 Step 0，再回到 [DSL 路径 Workflow](../routes/dsl.md) 继续 Step 2（生成 DSL）。

**图片数量**：3-6 张为宜。

## Step 0：图片准备

1. 识别图片需求（见上方触发关键词表）
2. 确定需要几张图，为每张图准备不同的搜索关键词（英文）
3. 逐张下载 → 校验每张图不同（文件大小） → 逐张上传到飞书 Drive
4. 收集所有 file_token，在 Step 2 生成 DSL 时引用

## 上传步骤

**单张图片**：
```bash
curl -L -o palace.jpg "https://example.com/palace.jpg"
lark-cli drive +upload --file ./palace.jpg
# 响应: { "file_token": "<file_token>", ... }
```

**多张图片（每张必须是不同的图）**：
```bash
# 1. 每张图用不同的搜索词/URL 下载
curl -L -o forbidden-city.jpg "https://example.com/forbidden-city.jpg"
curl -L -o great-wall.jpg "https://example.com/great-wall.jpg"
curl -L -o temple.jpg "https://example.com/temple.jpg"

# 2. 校验每张图确实不同（比较文件大小，跨平台通用）
ls -l *.jpg   # 确认每张文件大小不同；若大小相同则内容可能重复，需重新下载

# 3. 逐张上传，收集 token
lark-cli drive +upload --file ./forbidden-city.jpg  # → <file_token_1>
lark-cli drive +upload --file ./great-wall.jpg      # → <file_token_2>
lark-cli drive +upload --file ./temple.jpg          # → <file_token_3>
```

> **多图常见错误**：用同一个 URL 参数下载多次，导致多张图片完全相同。每张图必须用不同的搜索关键词或不同的图片 ID。

## 图片来源策略

| 来源 | 方式 | 适用场景 |
|------|------|----------|
| 公开 URL | `curl -L -o file.jpg <URL>` 下载后上传 | 景点照片、开源图片 |
| AI 生成 | 调用图片生成工具，保存后上传 | 插画、图标、概念图 |
| 用户提供 | 用户给出本地路径或 URL | 产品截图、Logo |

> `image.src` 必须是飞书 Drive 的 `file_token`，不支持直接使用 URL。所有图片都需要先上传。
