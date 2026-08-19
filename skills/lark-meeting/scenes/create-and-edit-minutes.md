# 生成和修改妙记、管理妙记权限

生成妙记和修改妙记都是写操作，必须有用户明确意图。生成成功后保存 `minute_token`；修改前确认唯一 `minute_token` 和目标内容，提供 token 不等于授权修改。除 `minutes +apply-permission` 外，本场景的 Minutes 写命令仅支持用户身份；来源身份为 bot 时停止并说明限制，只有用户明确同意后，才以 `--as user` 重新开始修改流程。`minutes +apply-permission` 支持用户或应用身份，必须沿用触发权限错误时的身份。

## 从本地音视频生成妙记

标准链路是 Drive 上传 → Minutes 创建 → 按需读取产物。不要改用 ffmpeg、Whisper 或其他本地 ASR。

### 上传音视频到 Drive

确认本地媒体路径以及用户最终需要妙记链接、逐字稿、总结、待办还是章节。源文件须符合 [`minutes +upload`](../references/lark-minutes-upload.md) 的格式要求，时长不超过 6 小时，大小不超过 6 GB。

按 [`lark-drive`](../../lark-drive/SKILL.md) 的路径和写操作规则执行 `drive +upload`，取得 `file_token`。上传参数和文件限制见 [`lark-drive-upload`](../../lark-drive/references/lark-drive-upload.md)。

### 使用 file_token 创建妙记

```bash
lark-cli minutes +upload --file-token <file_token> --as user
```

从返回的 `minute_url` 路径最后一段提取 `minute_token`，去掉 query 参数。创建参数、支持格式和异步语义见 [`lark-minutes-upload`](../references/lark-minutes-upload.md)。`minutes +upload` 成功仅表示异步创建请求已提交；报告返回的 `minute_url` 和可解析的 `minute_token`。未执行 `minutes +detail` 并确认就绪前，不得声称妙记产物已生成或可用。用户只要求发起创建或返回链接时，到此停止。

### 等待并读取妙记产物

上传后立即读取产物时必须加 `--wait-ready`：

```bash
lark-cli minutes +detail --minute-tokens <minute_token> --wait-ready --transcript --as user
```

将 `--transcript` 替换或扩展为用户需要的 `--summary`、`--todo`、`--chapter` 或 `--keyword`。创建任务仍在处理中时，按返回状态和重试提示轮询；不要重复上传或重复创建妙记。

用户要求独立提炼或复盘时，读取 Transcript 并基于原始发言分析，不要复述现成 Summary。产物 flags 和等待行为见 [`lark-minutes-detail`](../references/lark-minutes-detail.md)。

### 从失败阶段恢复

- Drive 上传成功但 Minutes 创建失败：保留并报告 `file_token`，从创建妙记继续，不要重复上传。
- Minutes 创建成功但产物未就绪：保留 `minute_token` 并重试查询，不要重新创建妙记。
- 不得把 Drive 上传成功误报为妙记创建成功；明确报告失败发生在上传、创建还是产物生成阶段。

## 修改妙记标题

使用 `minutes +update --minute-token <token> ... --as user`。参数见 [`lark-minutes-update`](../references/lark-minutes-update.md)。

## 替换 AI 总结

使用 `minutes +summary --minute-token <token> ... --as user` 替换总结全文。内容格式与权限见 [`lark-minutes-summary`](../references/lark-minutes-summary.md)。

## 增删改 AI 待办

妙记 AI 待办不是飞书任务。上下文包含妙记 URL / `minute_token` 并要求修改妙记待办时，禁止改走 `lark-task`。

```bash
lark-cli minutes +todo --minute-token <token> --operation add|update|delete ... --as user
```

- 多条新增优先使用 `--todos` 批量提交。
- 更新或删除前，先执行 `minutes +detail --minute-tokens <token> --todo --as user`，按内容匹配取得精确 `todo_id`；不要用列表顺序代替 ID。
- 待办 ID、批量结构和部分成功语义见 [`lark-minutes-todo`](../references/lark-minutes-todo.md)。

## 批量替换逐字稿关键词

```bash
lark-cli minutes +word-replace --minute-token <token> --replace-words '[{"source_word":"<old>","target_word":"<new>"}]' --as user
```

多组替换放在同一个 JSON 数组中。具体参数运行 `lark-cli minutes +word-replace --help`。

返回 `not_found` 表示 `source_word` 没有命中，是参数问题而不是权限问题；先读取当前 Transcript，核对精确写法和大小写后再决定是否重试。

## 替换逐字稿说话人

1. 调用 `lark-cli api GET "/open-apis/minutes/v1/minutes/<token>/transcript/speakerlist"` 取得 `speaker_id`。
2. 按原说话人的显示名称精确匹配。存在同名候选时，结合 Transcript 展示候选并让用户确认，不要擅选。
3. 用户只提供目标姓名时，用 [`lark-contact`](../../lark-contact/SKILL.md) 解析为 `ou_` open_id。
4. 执行 `minutes +speaker-replace --from-speaker-id <speaker_id> --to-user-id <open_id> --as user`；不要把展示名传给 `--from-speaker-id`。

完整流程和参数见 [`lark-minutes-speaker-replace`](../references/lark-minutes-speaker-replace.md)。

## 查看妙记授权列表

用户要查看妙记已授权给哪些成员，或查询某个成员当前的查看 / 编辑权限时，使用 Drive 协作者列表；这不是读取妙记内容，也不是为当前身份申请权限。先读取 [`lark-drive`](../../lark-drive/SKILL.md) 和 [`drive +member-list`](../../lark-drive/references/lark-drive-member-list.md)。

```bash
lark-cli drive +member-list --token "<minute_url>" --as <source_identity> --format json
```

完整妙记 URL 可自动推断资源类型为 `minutes`；裸 `minute_token` 必须显式传 `--type minutes`。需要核对指定成员时，按 `member_id` 精确匹配返回的 `items[]`，不要按姓名或列表顺序猜测。

## 分配妙记权限

用户要求“把妙记分享给某人”“让某人可以查看 / 编辑”或“给某人授予权限”时，修改的是目标妙记的协作者权限，使用 `drive +member-add`；禁止使用 `minutes +apply-permission`，后者只为当前调用身份向妙记所有者申请权限。

先读取 [`lark-drive`](../../lark-drive/SKILL.md) 和 [`drive +member-add`](../../lark-drive/references/lark-drive-member-add.md)。目标成员只有展示名时，按 [`lark-contact`](../../lark-contact/SKILL.md) 将其唯一解析为对应 ID；存在多个候选时请用户选择，不得猜测。

```bash
lark-cli drive +member-add \
  --token "<minute_url>" \
  --member-id "<open_id>" \
  --member-type openid \
  --perm view \
  --yes \
  --as <source_identity> \
  --format json
```

完整妙记 URL 可自动推断资源类型为 `minutes`；裸 `minute_token` 必须显式传 `--type minutes`。根据用户要求选择 `view` 或 `edit`；妙记不支持 `full_access`。只有妙记、目标成员和权限档位均已明确，且用户已明确要求执行授权时，才传 `--yes`。

写入后使用 `drive +member-list` 按 `member_id` 回读验证。只有返回的目标成员权限与用户要求一致时，才能声明分配完成；若目标成员已有不同权限，不要仅根据 `member-add` 回执声称权限已被覆盖或降级。

## 为当前身份申请妙记权限

没有查看或编辑权限时，先说明权限事实。只有用户明确要求申请权限时才执行：

```bash
lark-cli minutes +apply-permission --minute-token <token> --perm view --as <source_identity>
```

根据用户目标选择 `view` 或 `edit`，并必须沿用触发无权错误时的身份。这只是发起申请，不代表已经获得权限。身份和权限语义见 [`lark-minutes-apply-permission`](../references/lark-minutes-apply-permission.md)。

`permission_denied` 表示对该妙记没有编辑权，不等于 OAuth scope 缺失；请所有者授权，不要误走 `auth login --scope`。

## 确认修改结果

修改前只读取目标相关字段，修改后用 `minutes +detail` 或对应读取接口回读。批量或多步修改逐项报告写前值、写后结果和失败原因；部分成功时不要回滚已成功项，除非命令明确承诺原子回滚。
