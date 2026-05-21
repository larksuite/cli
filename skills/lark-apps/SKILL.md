---
name: lark-apps
description: "把本地 HTML 文件或目录部署到飞书妙搭（Miaoda），生成可分享访问的 Web 页面并返回 URL；管理应用的创建、更新、列表和访问范围。当用户要把 HTML、静态网站或 Web demo 发布成可分享链接，或提到妙搭 / Miaoda 时使用。不用于：上传普通文件到云空间（用 lark-drive）、编辑飞书云文档内容（用 lark-doc）、创建飞书原生幻灯片 / 演示文稿（用 lark-slides）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli apps --help"
---

# apps (v1)

```bash
# 常用示例
lark-cli apps +create           --name "客户调研问卷" --app-type HTML
lark-cli apps +html-publish     --app-id app_xxx --path ./dist
lark-cli apps +access-scope-set --app-id app_xxx --scope tenant
```

## 品牌可用性（先做）

跑 `lark-cli apps --help`；若提示暂未支持，告诉用户敬请期待并停止。

## 前置条件 — 执行操作前必读

**CRITICAL — 执行对应操作前，MUST 先用 Read 工具读取以下文件，缺一不可：**
1. [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、权限处理、全局参数（所有操作通用）
2. **创建应用（`apps +create`）** → 必读 [`lark-apps-create.md`](references/lark-apps-create.md)
3. **更新应用元信息（`apps +update`）** → 必读 [`lark-apps-update.md`](references/lark-apps-update.md)（部分更新，未传字段不变）
4. **发布 HTML / PPT / 静态网站（`apps +html-publish`）** → 必读 [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md)（`--path` 文件 vs 目录、tar.gz 打包不做过滤）
5. **设置可用范围（`apps +access-scope-set`）** → 必读 [`lark-apps-access-scope-set.md`](references/lark-apps-access-scope-set.md)（specific / public / tenant 三态互斥校验、targets JSON 结构）

**未读完以上文件就执行相应操作会导致参数选择错误、互斥违反或文件被错误打包。**

## 身份与一次性授权

妙搭应用是用户的个人资产，**统一使用 `--as user`**（CLI 默认 `--as auto` 会按 shortcut 声明自动落到 user）。

**首次操作前一次性把本域 scope 全拿到，避免每条命令首次跑都触发新一轮授权**：

```bash
lark-cli auth login --domain apps
```

## 端到端流程（HTML / PPT / 静态网站发布）

**第一步：判断用户意图是「明示部署」还是「仅演示」**：

| 用户表达 | 意图 | 处理 |
|---------|------|------|
| "部署 ./xxx 的 HTML"、"发布到妙搭"、"开发 xxx 并部署成可分享的网站 / 可访问的链接"、"生成可分享 URL" | **明示部署 / 分享** | 不停下追问，HTML 写完直接走下表 step 1→2 |
| "用 HTML 写一个 PPT / 幻灯片 / 演示文稿"、"做个可演示的 demo"、"写个介绍 xxx 的页面"（没提部署 / 分享 / URL） | **仅演示** | HTML 写完先输出本地文件路径 + 简要说明，**主动追问一句**："要部署到妙搭以便分享给别人吗？"用户同意再走 step 1→2；用户说不用就停 |

**第二步：用户同意部署 / 已明示部署后，按下表走完整链路并把最终 URL 返回给用户**：

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1. 新建应用 | `apps +create --name "<根据内容主题起的应用名>" --app-type HTML` → 从响应里拿 `app_id` | 默认都走新建（**不要尝试搜索 / 枚举已有应用**）。用户明确要复用现有应用时让他提供 **妙搭应用链接** 或 **app_id 字符串**（详见下方"快速决策"）；`--app-type` 必填，当前只支持 `HTML`（区分大小写），未来扩展 |
| 1.5 预检 | `apps +html-publish --app-id <id> --path <path> --dry-run` 看 `warnings` 字段 | 命中 `.git` / `.env*` / `*.pem` / `*.key` 等敏感文件时**停下来**，把 warnings 列给用户看，确认要继续才走 step 2；用户没确认前不要去掉 `--dry-run` 真发 |
| 2. 发布 HTML | `apps +html-publish --app-id <id> --path <文件或目录>` | 必走 |
| 3. 设置可用范围（可选） | `apps +access-scope-set --app-id <id> --scope tenant\|public\|specific ...` | 用户说"公开 / 全员可见 / 让 Alice 看 / 互联网可分享"等 |

报告给用户的话术：

> 应用「{name}」已发布，访问链接：`{url}`

若用户没指定可用范围且场景明显需要分享，主动追问一句"要设为企业全员 / 互联网公开吗？"，但不要为了问而问。

## 快速决策

- 用户**明示**"部署 / 发布 ./xxx 的 HTML"、"开发 xxx 并部署成可分享的网站 / 可访问的链接"、"发到妙搭" → 直接走「端到端流程」step 1→2，`apps +html-publish` 自动部署并返回 URL，不要追问
- 用户**只说**"用 HTML 写 PPT / 幻灯片 / 演示文稿 / demo"、"开发一个可演示的页面"（**没提**部署 / 分享 / URL） → HTML 写完先输出本地路径 + 简要说明，主动问一句"要部署到妙搭以便分享吗？"，用户同意才走 publish；不要擅自部署，但也不要忘了问
- 用户说"把应用 X 开放给全员 / 全公司" → `--scope tenant`，不要再传别的 flag
- 用户说"公开 / 让任何人都能访问 / 互联网可见" → `--scope public --require-login=<bool>`，二选一
- 用户说"只让 Alice / 某部门 / 某群访问" → `--scope specific --targets <JSON>`；姓名先用 `contact +search-user` 换 `ou_id`，群名先用 `im +chat-search` 换 `chat_id`
- 用户没给 app_id → **默认 `apps +create --name "<根据内容主题起的名字>" --app-type HTML` 新建一个**。**不要尝试搜索 / 枚举已有应用** —— 列举应用的命令对 Agent 不可见，强行调用也只会浪费一次 OAPI 请求。如果用户明确要复用现有应用，**让他提供下列任一种**：
  - **妙搭应用链接**：形如 `https://miaoda.feishu.cn/app/app_xxxxxxxxxxxxx`（或带尾斜杠 `/app/app_xxx/`）—— `app_id` 是 `/app/` 后面的 path segment（以 `app_` 开头）。从 URL 中提取的简单办法：`APP_ID=$(echo "$URL" | sed -E 's|.*/app/([^/?#]+).*|\1|')`
  - **app_id 字符串**：用户直接给的 `app_xxxxxxxxxxxxx`，不需要再做处理
- `--path` 既可传单个 HTML 文件也可传目录；目录会**递归打包成 tar.gz 不做过滤**，要提醒用户传干净的产物目录（如 `./dist`），避免把 `.git` / `node_modules` 一起打进去
- `apps +update` 只更新传入字段，未传字段保持不变；`--name` / `--description` 至少传一个，否则 Validate 阶段直接拦截
- `apps +access-scope-set` 三种 scope **互斥**：specific 必传 `--targets`、不允许 `--require-login`；public 必传 `--require-login`、不允许 `--targets` / `--apply-enabled` / `--approver`；tenant 不允许任何其他 flag
- 失败时**优先转述 `error.hint`**（CLI 给的可执行修复建议），hint 为空时退回 `error.message`；不要原样把 envelope JSON 复述给用户

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli apps +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-apps-create.md) | 创建妙搭应用（name / description / icon-url） |
| [`+update`](references/lark-apps-update.md) | 部分更新应用名 / 描述（只发传入字段） |
| [`+access-scope-set`](references/lark-apps-access-scope-set.md) | 设置应用可用范围（specific / public / tenant，三态互斥校验） |
| [`+html-publish`](references/lark-apps-html-publish.md) | **把本地 HTML 文件 / 目录 / PPT / 静态网站部署为可分享的妙搭应用，返回访问 URL**（用户明示部署 / 分享时直接调；仅说"可演示"时先问用户是否要部署再调） |
