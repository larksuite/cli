# `vc +meeting-screenshot`

获取视频会议截图，并保存为 JPEG。

## 常用用法

使用当前用户身份截图，文件写入默认目录：

```bash
lark-cli vc +meeting-screenshot --as user --meeting-id <long_meeting_id>
```

指定输出路径：

```bash
lark-cli vc +meeting-screenshot --as user --meeting-id <long_meeting_id> --output ./meeting-screenshots/current.jpg
```

## 参数

| Flag | 含义与用法 |
| --- | --- |
| `--as <identity>` | `user`，即当前 Agent Employee；要求该主体已在会中并具备会中读取权限。与发现 `meeting_id` 时使用的是同一个主体。 |
| `--meeting-id <meeting_id>` | 必填。长数字会议 ID，不接受 9 位会议号；只有会议号时，先用同一身份调用 `vc +meeting-list-active` 获取。 |
| `--output <relative-path>` | 可选。指定 JPEG 文件名或包含子目录的相对路径；相对于执行命令时的当前工作目录。 |
| `--overwrite` | 可选。目标文件已存在时允许替换；不传时命令会失败并保留原文件。 |

## 常见错误

| 错误现象 | 根本原因 | 解决方案 |
| --- | --- | --- |
| `128006`: `meeting screenshot is currently in limited availability and is not available for this meeting.` | 会议截图能力仍在灰度，当前会议不在可用范围内 | 不要通过切换 `--as` 身份或新增 scope 绕过。确认目标会议归属方是否在截图能力范围内；如果应当可用，保留 `log_id` 交给服务端排查 |

## 文件路径与结果

- 未指定 `--output` 时，默认写入当前工作目录下的 `meeting-screenshots/<meeting_id>-<UTC timestamp>.jpg`。
- `--output` 可以只写文件名，也可以包含多级子目录；父目录会自动创建。
- 不接受绝对路径，也不接受解析后超出当前工作目录的 `..` 或符号链接路径。
- 成功结果包含绝对文件路径、字节数、JPEG content type、SHA-256 和服务端 `log_id`。
- 服务端决定截图内容并校验会议是否满足条件；调用方不能指定要截取的区域或共享内容。失败不会替换已有文件。
