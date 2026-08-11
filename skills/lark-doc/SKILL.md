---
name: lark-doc
description: "飞书云文档（Docx / Wiki）内容操作：读取、创建、编辑文档，插入或下载图片附件，以及操作思维笔记。用户提供文档 URL/token（包括 doubao.com 的 /docx/、/wiki/）时使用；按 URL 路径/token 而非域名路由。文档内嵌资源按读取参考中的统一规则分流。文档评论走 lark-drive；表格或 Base 内部数据操作不在本 skill。"
metadata:
  requires:
    bins: ["lark-cli"]
    skills: ["lark-shared"]
  cliHelp: "lark-cli docs --help;lark-cli mindnotes --help"
---

# docs

## 场景与 Shortcut 路由

**CRITICAL：先判断场景，再读取该场景的参考文件；不要在任务开始时一次性读取全部参考文件。每个文件只在首次进入对应阶段时读取一次。**

**身份：文档操作推荐显式指定 `--as user`。例外：如果 `doc_token` / `note_doc_token` 等是从 bot 链路（如 `vc +detail --as bot` → `note +detail --as bot`）取得的，应继续显式使用 `--as bot`，不要无条件切回 user——身份延续规则见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)。**

**所有表示本地文件的 `@path` 均使用 `@./xxx` 形式的相对路径，并以运行 `lark-cli` 时的当前工作目录（CWD）为基准。**

### 文档内容

- **读取 / 摘要 — [`+fetch`](references/lark-doc-fetch.md)**：先读参考再获取文档。
- **从零创作 — [`创建工作流`](references/lark-doc-create-workflow.md)**：先完整执行创建工作流，**简单任务不是跳过的理由**；
- **导入 / 空文档 — [`+create`](references/lark-doc-create.md)**：仅创建空文档或原样导入用户提供的完整内容时，跳过创建工作流。
- **编辑 / block 直达链接 — [`+update`](references/lark-doc-update.md)**：语义改写、润色、重组、补写或排版均按 update 参考完成。

### 辅助能力

- **草稿初始化、解析与统计 — [`+script`](references/lark-doc-script.md)**：支持解析文档 URL / token 与本地 XML，统计字数并返回字符诊断；不支持 Markdown 输入。
- **历史版本 — [`+history-list` / `+history-revert` / `+history-revert-status`](references/lark-doc-history.md)**：查询、回滚文档历史版本或检查回滚任务状态。

### 资源、画板与思维笔记

- **插入本地素材 — [`+media-insert`](references/lark-doc-media-insert.md)**：在文末插入本地图片或文件。
- **预览素材 — [`+media-preview`](references/lark-doc-media-preview.md)**：预览文档中的图片、附件或素材。
- **下载素材 — [`+media-download`](references/lark-doc-media-download.md)**：下载文档中的图片、附件、素材或画板缩略图。
- **Docx 封面 — [`+resource-download` / `+resource-update` / `+resource-delete`](references/lark-doc-resource-cover.md)**：下载、更新或删除 Docx 封面。
- **画板 — [`画板工作流`](references/lark-doc-whiteboard.md)**：创建或更新画板时先读取工作流；更新已有画板必须复用现有 token，禁止新建空白画板；使用 [`whiteboard +update`](../lark-whiteboard/references/lark-whiteboard-update.md) 写入。
- **思维笔记 — `mindnotes`**：已有思维笔记走 [`思维笔记链路`](references/lark-doc-mindnote.md)；新建思维笔记走 [`lark-doc-whiteboard`](references/lark-doc-whiteboard.md)。

### 认证与 Scope

执行 Shortcut 时，不预读 [`lark-shared`](../lark-shared/SKILL.md) 或预跑 `auth status --verify`；仅遇到未认证、token / 身份或 scope 错误时读取该 Skill，修复后重试。认证、身份或 scope 管理请求则直接使用该 Skill。

## 不在本 Skill 范围

- **Drive 文件级操作**：找文档、导入导出、云空间文件上传 / 下载 / 权限管理 → [`lark-drive`](../lark-drive/SKILL.md)。复制文档、创建副本或另存为副本时，按其指引使用 `lark-cli drive files copy`；不要用 `docs +fetch` + `docs +create` 重建正文。
- **文档评论**：添加、查看、回复评论或增删 reaction → [`lark-drive`](../lark-drive/SKILL.md)。
