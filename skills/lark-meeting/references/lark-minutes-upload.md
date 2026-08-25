# minutes +upload


上传音视频文件到飞书妙记并生成妙记（Minute）。

本 skill 对应 shortcut：`lark-cli minutes +upload`。

## 典型触发表达

- "把这个音视频文件转成妙记"
- "把这个音视频文件转成纪要"
- "把这个音视频文件转成逐字稿、文字稿或撰写文字"
- "把这个音视频文件转成总结、待办或章节"

## 命令示例

```bash
# 通过已上传到云空间（云盘/云存储）的 file_token 生成妙记
lark-cli minutes +upload --file-token boxcnxxxxxxxxxxxxxxxx

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

### 2. 异步生成

API 会立即返回 `minute_url`，但妙记可能仍在异步生成中。`minutes +upload` 不返回处理状态，命令成功只表示异步创建请求已提交；只有后续执行 `minutes +detail` 并确认就绪，才能声称妙记产物已生成或可用。上传与后续产物获取由 [`create-and-edit-minutes`](../scenes/create-and-edit-minutes.md) 编排；上传后立即查询产物时，`minutes +detail` 必须使用 `--wait-ready`。

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

## 相关场景
- [生成和修改妙记](../scenes/create-and-edit-minutes.md)
