# 生成和修改妙记

生成妙记和修改妙记都是写操作，必须有用户明确意图。生成成功后保存 `minute_token`；修改前确认唯一 `minute_token` 和目标内容，提供 token 不等于授权修改。Minutes 写操作默认使用用户身份。

## 从本地音视频生成妙记

标准链路是 Drive 上传 → Minutes 创建 → 按需读取产物。不要改用 ffmpeg、Whisper 或其他本地 ASR。

### 上传音视频到 Drive

确认本地媒体路径以及用户最终需要妙记链接、逐字稿、总结、待办还是章节。源文件须符合 [`minutes +upload`](../references/lark-minutes-upload.md) 的格式要求，时长不超过 6 小时，大小不超过 6 GB。

按 [`lark-drive`](../../lark-drive/SKILL.md) 的路径和写操作规则执行 `drive +upload`，取得 `file_token`。上传参数和文件限制见 [`lark-drive-upload`](../../lark-drive/references/lark-drive-upload.md)。

### 使用 file_token 创建妙记

```bash
lark-cli minutes +upload --file-token <file_token> --as user
```

从返回的 `minute_url` 路径最后一段提取 `minute_token`，去掉 query 参数。创建参数、支持格式和任务状态见 [`lark-minutes-upload`](../references/lark-minutes-upload.md)。用户只要妙记时，返回 `minute_url`、`minute_token` 和创建状态后停止。

### 等待并读取妙记产物

上传后立即读取产物时必须加 `--wait-ready`：

```bash
lark-cli minutes +detail --minute-tokens <minute_token> --wait-ready --transcript
```

将 `--transcript` 替换或扩展为用户需要的 `--summary`、`--todo`、`--chapter` 或 `--keyword`。创建任务仍在处理中时，按返回状态和重试提示轮询；不要重复上传或重复创建妙记。

用户要求独立提炼或复盘时，读取 Transcript 并基于原始发言分析，不要复述现成 Summary。产物 flags 和等待行为见 [`lark-minutes-detail`](../references/lark-minutes-detail.md)。

### 从失败阶段恢复

- Drive 上传成功但 Minutes 创建失败：保留并报告 `file_token`，从创建妙记继续，不要重复上传。
- Minutes 创建成功但产物未就绪：保留 `minute_token` 并重试查询，不要重新创建妙记。
- 不得把 Drive 上传成功误报为妙记创建成功；明确报告失败发生在上传、创建还是产物生成阶段。

## 修改妙记标题

使用 `minutes +update --minute-token <token> ...`。参数见 [`lark-minutes-update`](../references/lark-minutes-update.md)。

## 替换 AI 总结

使用 `minutes +summary --minute-token <token> ...` 替换总结全文。内容格式与权限见 [`lark-minutes-summary`](../references/lark-minutes-summary.md)。

## 增删改 AI 待办

妙记 AI 待办不是飞书任务。上下文包含妙记 URL / `minute_token` 并要求修改妙记待办时，禁止改走 `lark-task`。

```bash
lark-cli minutes +todo --minute-token <token> --operation add|update|delete ...
```

- 多条新增优先使用 `--todos` 批量提交。
- 更新或删除前，先执行 `minutes +detail --todo`，按内容匹配取得精确 `todo_id`；不要用列表顺序代替 ID。
- 待办 ID、批量结构和部分成功语义见 [`lark-minutes-todo`](../references/lark-minutes-todo.md)。

## 批量替换逐字稿关键词

```bash
lark-cli minutes +word-replace --minute-token <token> --replace-words '[{"source_word":"<old>","target_word":"<new>"}]'
```

多组替换放在同一个 JSON 数组中。具体参数运行 `lark-cli minutes +word-replace --help`。

返回 `not_found` 表示 `source_word` 没有命中，是参数问题而不是权限问题；先读取当前 Transcript，核对精确写法和大小写后再决定是否重试。

## 替换逐字稿说话人

1. 调用 `lark-cli api GET "/open-apis/minutes/v1/minutes/<token>/transcript/speakerlist"` 取得 `speaker_id`。
2. 按原说话人的显示名称精确匹配。存在同名候选时，结合 Transcript 展示候选并让用户确认，不要擅选。
3. 用户只提供目标姓名时，用 [`lark-contact`](../../lark-contact/SKILL.md) 解析为 `ou_` open_id。
4. 执行 `minutes +speaker-replace --from-speaker-id <speaker_id> --to-user-id <open_id>`；不要把展示名传给 `--from-speaker-id`。

完整流程和参数见 [`lark-minutes-speaker-replace`](../references/lark-minutes-speaker-replace.md)。

## 申请妙记权限

没有查看或编辑权限时，先说明权限事实。只有用户明确要求申请权限时才执行：

```bash
lark-cli minutes +apply-permission --minute-token <token> --perm view --as <source_identity>
```

根据用户目标选择 `view` 或 `edit`，并必须沿用触发无权错误时的身份。这只是发起申请，不代表已经获得权限。身份和权限语义见 [`lark-minutes-apply-permission`](../references/lark-minutes-apply-permission.md)。

`permission_denied` 表示对该妙记没有编辑权，不等于 OAuth scope 缺失；请所有者授权，不要误走 `auth login --scope`。

## 确认修改结果

修改前只读取目标相关字段，修改后用 `minutes +detail` 或对应读取接口回读。批量或多步修改逐项报告写前值、写后结果和失败原因；部分成功时不要回滚已成功项，除非命令明确承诺原子回滚。
