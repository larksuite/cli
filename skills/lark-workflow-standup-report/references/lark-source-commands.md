# 飞书数据源与命令参考

执行前必须读取 `../lark-shared/SKILL.md` 了解 user/bot 身份和权限处理。本 skill 仅使用 `--as user`。

## 推荐脚本

日报优先运行。macOS/Linux 使用 `.sh`，Windows/PowerShell 使用 `.ps1`。

macOS/Linux 依赖 `bash`、`jq`、`python3`、`perl`、`lark-cli`。若 macOS 自带 Bash 版本过旧，可使用 Homebrew 安装新版 bash：`brew install bash jq python3`。

macOS/Linux:

```bash
scripts/collect_lark_daily.sh \
  --date "YYYY-MM-DD" \
  --start "YYYY-MM-DDT00:00:00+08:00" \
  --end "YYYY-MM-DDTHH:mm:ss+08:00" \
  --out-dir "work/daily_YYYY-MM-DD" \
  --im-chunk-hours 1 \
  --im-request-timeout-seconds 45 \
  --im-max-failures-per-search 4
```

Windows/PowerShell:

```powershell
scripts/collect_lark_daily.ps1 `
  -Date "YYYY-MM-DD" `
  -Start "YYYY-MM-DDT00:00:00+08:00" `
  -End "YYYY-MM-DDTHH:mm:ss+08:00" `
  -OutDir "work/daily_YYYY-MM-DD" `
  -ImChunkHours 1 `
  -ImRequestTimeoutSeconds 45 `
  -ImMaxFailuresPerSearch 4
```

脚本会采集日历、视频会议、会议详情、可用的会议纪要/妙记、当前用户、全量群聊、本人群聊、云文档，处理分页，脱敏原始 JSON，并生成 `source_manifest.json`。

群聊搜索慢时，脚本先采本人消息，再采全量群聊；全量群聊按时间片隔离失败。某个时间片超时只写入 `source_manifest.json.errors`，不得阻塞日历、会议、本人消息、云文档和本地 Agent 证据采集。

## 手工命令

仅在脚本不可用或需要调试时手工执行。

### 日历

```bash
lark-cli calendar +agenda --as user \
  --start "<START>" --end "<END>" --format json
```

### 视频会议

```bash
lark-cli vc +search --as user \
  --start "<START>" --end "<END>" --format json --page-size 30
```

`has_more=true` 时使用 `--page-token` 翻页。对高信号会议默认继续读取详情：

```bash
lark-cli vc meeting get --as user \
  --params '{"meeting_id":"<id>","with_participants":true}'
```

若会议存在纪要或妙记，再继续读取：

```bash
lark-cli vc +notes --as user --meeting-ids "<meeting_id>" --format json
```

若已拿到妙记链接或 minute token，再继续读取：

```bash
lark-cli vc +notes --as user --minute-tokens "<minute_token>" --format json
```

日报模式下，会议证据默认优先级是：

1. 逐字稿正文。
2. Agent 基于逐字稿形成的自主摘要，以及同主题本地 Codex / Claude Code 会话、knowledge、产物在会议事项归因场景中形成的补强证据。
3. 纪要/逐字稿/妙记链接。
4. 妙记 AI 总结、待办、章节，仅作候选线索。
5. `meeting get` 的会议详情。
6. `vc +search` 搜索元数据。

`vc +notes` 或妙记拉取失败时不阻塞，直接降级到 `meeting get` 结果，但必须在 `source_manifest.json.errors` 和 `数据来源说明` 里写明。

只要拿到了逐字稿，就必须阅读逐字稿并自行总结，不要直接把飞书内置 AI 的妙记总结、待办或章节原文抄进日报。若只有 AI 摘要没有逐字稿，必须再用本人消息、云文档或本地 Agent 产物交叉验证；没有补强证据时，不得把 AI 摘要写成确定结论。本地 Agent 产出只有在引用会议结论时是补强；若它本身就是当天工作产出，仍可作为本地工作包主证据。

### 群聊消息

```bash
lark-cli im +messages-search --as user \
  --sender "ou_<user_openid>" \
  --start "<START>" --end "<END>" \
  --page-size 50 --format json
```

```bash
lark-cli im +messages-search --as user \
  --start "<START>" --end "<END>" \
  --page-size 50 --format json
```

两条路径都必须翻页。重点提取本人发出的含链接、文档、结论、行动项的消息。忽略机器人消息、表情回复、纯社交闲聊。

如果搜索结果中只有 `message_ids`，并出现 `failed to fetch message details`，说明 user token 具备搜索权限但缺少消息详情读取权限。不要切换到 bot 身份；user 身份读取正文需要“基础读消息权限 + 按会话类型补充权限”。优先使用最小只读授权：

```bash
lark-cli auth login --scope "im:message:readonly im:message.group_msg:get_as_user im:message.p2p_msg:get_as_user"
```

授权后重新运行采集脚本。只有 ID 无正文时，日报不能把群聊内容写成确定结论。

### 云文档

```bash
lark-cli docs +search --as user --query "" \
  --filter '{"open_time":{"start":"<START>","end":"<END>"}}' \
  --page-size 20 --format json
```

筛选重点：

- 本人创建/更新的文档。
- 与本周或当天主线相关的文档标题。

## 文档创建

优先运行。macOS/Linux 使用 `.sh`，Windows/PowerShell 使用 `.ps1`。

macOS/Linux:

```bash
scripts/create_daily_doc.sh \
  --markdown-path "outputs/工作日报-YYYY-MM-DD.md" \
  --title "工作日报 YYYY-MM-DD 杜励承" \
  --wiki-node "<日报父 Wiki 节点 URL>" \
  --response-path "work/daily_YYYY-MM-DD/feishu_doc_create_daily_YYYY-MM-DD.json"
```

Windows/PowerShell:

```powershell
scripts/create_daily_doc.ps1 `
  -MarkdownPath "outputs/工作日报-YYYY-MM-DD.md" `
  -Title "工作日报 YYYY-MM-DD 杜励承" `
  -WikiNode "<日报父 Wiki 节点 URL>" `
  -ResponsePath "work/daily_YYYY-MM-DD/feishu_doc_create_daily_YYYY-MM-DD.json"
```

手工命令（v2）：

```bash
lark-cli docs +create --api-version v2 --as user \
  --parent-token "<日报父 Wiki 节点 token>" \
  --doc-format markdown \
  --content "@work/daily_YYYY-MM-DD/daily_doc_create_content.md" \
  --format json
```

正文不要重复一级标题。
