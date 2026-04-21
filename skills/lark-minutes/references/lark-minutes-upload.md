# minutes +upload

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

上传音视频文件到飞书妙记并生成妙记（Minute）。

本 skill 对应 shortcut：`lark-cli minutes +upload`。

## 典型触发表达

- "把这个音视频文件转成妙记"
- "帮我把电脑里的录音上传到妙记"
- "将这个 mp4/mp3 文件生成妙记"

## 完整工作流

当用户要求将音视频文件转换为妙记时，必须按照以下步骤执行：

1. **上传文件至云空间获取 file_token**
   - 使用 `lark-cli drive +upload` 命令上传本地文件到云空间（Drive）：
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

> **异步生成提示**：API 会立即返回 `minute_url`，但妙记可能仍在异步生成中，您可以直接通过该妙记链接查看当前的处理状态和转写结果。

## 命令示例

```bash
# 通过已上传到云空间的 file_token 生成妙记
lark-cli minutes +upload --file-token boxcnxxxxxxxxxxxxxxxx
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--file-token <token>` | 是 | 已经上传到飞书云空间的音视频文件的 file_token |

## 支持的格式与限制

待上传到妙记的原始音视频文件必须满足以下要求：

- 支持音频格式：`wav`、`mp3`、`m4a`、`aac`、`ogg`、`wma`、`amr`
- 支持视频格式：`avi`、`wmv`、`mov`、`mp4`、`m4v`、`mpeg`、`ogg`、`flv`
- 音视频时长不能超过 `6` 小时
- 文件大小不能超过 `6 GB`

> 说明：本 shortcut 只接收 `file_token`，不会直接读取本地文件内容，因此这些格式、时长和大小限制对应的是**原始上传文件**本身。若妙记生成失败，请先回查源文件是否满足上述要求。

## 核心约束

### 1. 必须提供 file_token

本接口不直接处理本地文件的上传，必须先使用 `drive +upload` 将文件上传到云空间获取 `file_token`，然后再调用本接口。

### 2. 先上传，再生成妙记

推荐流程如下：

1. 使用 `lark-cli drive +upload --file <path>` 上传本地音视频文件到云空间
2. 从返回结果中取出 `file_token`
3. 调用 `lark-cli minutes +upload --file-token <file_token>` 生成妙记

## 输出结果示例

```json
{
  "minute_url": "http(s)://<host>/minutes/<minute-token>"
}
```

| 字段 | 说明 |
|------|------|
| `minute_url` | 生成的妙记访问链接 |

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关功能说明
- [drive +upload](../../lark-drive/references/lark-drive-upload.md) -- 上传文件到云空间
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
