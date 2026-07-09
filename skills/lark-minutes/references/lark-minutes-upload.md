# minutes +upload


上传音视频文件到飞书妙记并生成妙记（Minute）。

本 skill 对应 shortcut：`lark-cli minutes +upload`。

## 典型触发表达

- "把这个音视频文件转成妙记"
- "把这个音视频文件转成纪要"
- "把这个音视频文件转成逐字稿、文字稿或撰写文字"
- "把这个音视频文件转成总结、待办或章节"

## 完整工作流

当用户要求将音视频文件转换为妙记，或进一步要纪要/逐字稿/文字稿/撰写文字时，必须按照以下步骤执行：

1. **上传文件至云空间（云盘/云存储）获取 file_token**
   - 使用 `lark-cli drive +upload` 命令上传本地文件到云空间/云盘/云存储（Drive）：
     ```bash
     lark-cli drive +upload --file <path/to/media/file>
     ```
   - 从命令的返回结果中提取生成的 `file_token`。

2. **将 file_token 转换为妙记链接（minute_url）**
   - 调用本 shortcut，将获取到的 `file_token` 转换为妙记：
     ```bash
     lark-cli minutes +upload --file-token <file_token>
     ```
   - 命令执行成功后，将返回生成的妙记链接 `minute_url`。

3. **如需纪要 / 逐字稿 / 文字稿 / 撰写文字，使用返回的 `minute_token` 调用 `minutes +detail`**
   - 如果用户要的是纪要、逐字稿、文字稿、撰写文字、总结、待办或章节，使用上一步返回的 `minute_token` 继续调用：
     ```bash
     lark-cli minutes +detail --minute-tokens <minute_token> --wait-ready --summary --todo --chapter --keyword --transcript
     ```
   - `--wait-ready` 参数表示等待妙记生成完毕后再获取产物，上传后立即读取详情时必须加上此参数。
   - `minutes +detail --minute-tokens` 会返回妙记产物（总结、待办、章节、关键词、逐字稿）；必要时还会把逐字稿落地到本地文件。

> **异步生成提示**：API 会立即返回 `minute_url`，但妙记可能仍在异步生成中，您可以直接通过该妙记链接查看当前的处理状态和转写结果。

## 命令示例

```bash
# 通过已上传到云空间（云盘/云存储）的 file_token 生成妙记
lark-cli minutes +upload --file-token boxcnxxxxxxxxxxxxxxxx

# 上传后立即获取妙记产物，需加 --wait-ready 等待生成完毕（--summary --todo --chapter --keyword --transcript 按需传入）
lark-cli minutes +detail --minute-tokens obcnxxxxxxxxxxxxxxxx --wait-ready --summary
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token <token>` | 是 | 已经上传到飞书云空间（云盘/云存储）的音视频文件的 file_token |

## 支持的格式与限制

待上传到妙记的原始音视频文件必须满足以下要求：

- 支持音频格式：`wav`、`mp3`、`m4a`、`aac`、`ogg`、`wma`、`amr`
- 支持视频格式：`avi`、`wmv`、`mov`、`mp4`、`m4v`、`mpeg`、`ogg`、`flv`
- 音视频时长不能超过 `6` 小时
- 文件大小不能超过 `6 GB`

> 说明：本 shortcut 只接收 `file_token`，不会直接读取本地文件内容，因此这些格式、时长和大小限制对应的是**原始上传文件**本身。若妙记生成失败，请先回查源文件是否满足上述要求。

## 核心约束

### 1. 必须提供 file_token

本接口不直接处理本地文件的上传，必须先使用 `drive +upload` 将文件上传到云空间（云盘/云存储）获取 `file_token`，然后再调用本接口。

### 2. 先上传，再生成妙记

推荐流程如下：

1. 使用 `lark-cli drive +upload --file <path>` 上传本地音视频文件到云空间（云盘/云存储）
2. 从返回结果中取出 `file_token`
3. 调用 `lark-cli minutes +upload --file-token <file_token>` 生成妙记
4. 如果目标是纪要、逐字稿、文字稿、撰写文字、总结、待办或章节，使用返回的 `minute_token`，继续调用 `lark-cli minutes +detail --minute-tokens <minute_token> --wait-ready`

> **边界说明**：`minutes +upload` 本身只负责把文件转成妙记并返回 `minute_url`。纪要内容、逐字稿、文字稿、撰写文字、总结、待办、章节属于后续产物获取，应由 [minutes +detail](lark-minutes-detail.md) 承接。

## 输出结果示例

```json
{
  "minute_url": "http(s)://<host>/minutes/<minute-token>",
  "minute_token": "<minute-token>"
}
```

| 字段 | 说明 |
|------|------|
| `minute_url` | 生成的妙记访问链接 |
| `minute_token` | 从 `minute_url` 提取出的妙记 Token，可直接传给 `minutes +detail --minute-tokens` |

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关功能说明
- [drive +upload](../../lark-drive/references/lark-drive-upload.md) -- 上传文件到云空间（云盘/云存储）
