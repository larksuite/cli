---
name: lark-apps
description: "飞书妙搭应用（lark-cli apps）：把本地 HTML 文件或目录一键部署为可访问、可分享、可演示的妙搭应用（静态网站 / Web 页面），返回访问 URL；并提供应用创建、更新、列出、设置可用范围（specific 指定可见 / public 互联网公开 / tenant 企业全员）等管理能力。当用户说『用 HTML / 网页开发 PPT / 幻灯片 / 演示文稿 / 可演示的 demo』、『部署 / 发布 HTML / 静态网站 / 网页 / dist 目录』、『把 /xxx 中的 HTML 文件用 lark-cli 部署 / 发到妙搭』、『开发一个 xxx 并部署成可以分享的网站 / 可访问的链接 / 可分享 URL』、『生成一个可以演示 / 可以发给别人看的 PPT / 页面 / demo』，或提到 妙搭 / miaoda / apps / app_id / 可用范围 / open-to-tenant / open-to-public 等关键词时使用。**默认通过 `apps +html-publish` 自动完成部署并把访问 URL 直接返回给用户，不停在『HTML 写好了』那一步。**"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli apps --help; lark-cli apps +create --help; lark-cli apps +html-publish --help; lark-cli apps +access-scope-set --help"
---

# apps (v1)

```bash
# 常用示例
lark-cli apps +create           --name "客户调研问卷"
lark-cli apps +list             --page-size 50
lark-cli apps +html-publish     --app-id app_xxx --path ./dist
lark-cli apps +access-scope-set --app-id app_xxx --scope tenant
```

## 前置条件 — 执行操作前必读

**CRITICAL — 执行对应操作前，MUST 先用 Read 工具读取以下文件，缺一不可：**
1. [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、权限处理、全局参数（所有操作通用）
2. **创建应用（`apps +create`）** → 必读 [`lark-apps-create.md`](references/lark-apps-create.md)
3. **更新应用元信息（`apps +update`）** → 必读 [`lark-apps-update.md`](references/lark-apps-update.md)（部分更新，未传字段不变）
4. **列出应用 / 反查 app_id（`apps +list`）** → 必读 [`lark-apps-list.md`](references/lark-apps-list.md)（cursor 分页、`-q` 按 name 提取 app_id）
5. **发布 HTML / PPT / 静态网站（`apps +html-publish`）** → 必读 [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md)（`--path` 文件 vs 目录、tar.gz 打包不做过滤）
6. **设置可用范围（`apps +access-scope-set`）** → 必读 [`lark-apps-access-scope-set.md`](references/lark-apps-access-scope-set.md)（specific / public / tenant 三态互斥校验、targets JSON 结构）

**未读完以上文件就执行相应操作会导致参数选择错误、互斥违反或文件被错误打包。**

## 端到端流程（HTML / PPT / 静态网站发布）

只要用户说『用 HTML 写 PPT / 演示 / 静态网站 / demo』『部署 ./xxx 中的 HTML』『开发 xxx 并部署成可分享的网站』『生成一个可演示的 PPT』之类，**默认按下表走完整链路并把最终 URL 返回给用户，中间步骤不要停下追问**：

| 步骤 | 命令 | 触发条件 |
|------|------|---------|
| 1. 新建应用 | `apps +create --name "<根据内容主题起的应用名>"` → 从响应里拿 `app_id` | 默认都走新建，**不要先 `+list` 找复用**（除非用户明确说"用我现有的应用 X"） |
| 2. 发布 HTML | `apps +html-publish --app-id <id> --path <文件或目录>` | **必走**，无论用户有没有显式说"部署" |
| 3. 设置可用范围（可选） | `apps +access-scope-set --app-id <id> --scope tenant\|public\|specific ...` | 用户说"公开 / 全员可见 / 让 Alice 看 / 互联网可分享"等 |

报告给用户的话术：

> 应用「{name}」已发布，访问链接：`{url}`

若用户没指定可用范围且场景明显需要分享，主动追问一句"要设为企业全员 / 互联网公开吗？"，但不要为了问而问。

## 快速决策

- 用户说"用 HTML 写 PPT / 幻灯片 / 演示文稿 / 静态网站 / demo"、"开发一个可演示的页面"、"部署 / 发布 ./xxx 的 HTML"、"开发 xxx 并部署成可分享的网站" → 走上方「端到端流程」，**默认 `apps +html-publish` 自动部署并返回 URL**，不要只输出本地 HTML 文件就停下
- 用户说"把应用 X 开放给全员 / 全公司" → `--scope tenant`，不要再传别的 flag
- 用户说"公开 / 让任何人都能访问 / 互联网可见" → `--scope public --require-login=<bool>`，二选一
- 用户说"只让 Alice / 某部门 / 某群访问" → `--scope specific --targets <JSON>`；姓名先用 `contact +search-user` 换 `ou_id`，群名先用 `im +chat-search` 换 `chat_id`
- 用户没给 app_id → **默认 `apps +create --name "<根据内容主题起的名字>"` 新建一个**，不要去 `+list` 翻库找复用；仅当用户明确说"用我现有的应用 X / 部署到 app_xxx"时才走 `apps +list -q '.data.items[] | select(.name=="X") | .app_id'` 反查（第一页没命中且 `has_more=true` 用 `--page-token` 翻页继续找）
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
| [`+list`](references/lark-apps-list.md) | 列出当前用户的妙搭应用（cursor 分页，可用 `-q` 按 name 反查 app_id） |
| [`+access-scope-set`](references/lark-apps-access-scope-set.md) | 设置应用可用范围（specific / public / tenant，三态互斥校验） |
| [`+html-publish`](references/lark-apps-html-publish.md) | **把本地 HTML 文件 / 目录 / PPT / 静态网站一键部署为可分享的妙搭应用，返回访问 URL**（用 HTML 做 PPT、demo、演示页时的默认部署入口） |
