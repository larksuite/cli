---
name: lark-backup
version: 0.1.0
description: "飞书备份工作流：用 lark-cli 备份飞书云文档、云空间文件夹、电子表格、多维表格到本地目录。当用户提到 backup、备份、归档、导出全部、下载整个文件夹、备份知识库、备份云空间、备份多维表格时使用。"
metadata:
  requires:
    bins: ["lark-cli", "jq", "bash"]
---

# lark-backup

**CRITICAL — 开始前 MUST 先读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)**，确认认证、身份和权限处理规则。

这个 skill 不替代 `lark-doc` / `lark-drive` / `lark-sheets` / `lark-base` / `lark-wiki`。它是在这些官方 skill 之上，加一层“备份工作流”和确定性的脚本入口。

## 适用场景

- “备份这个飞书文档 / wiki / 表格 / 多维表格”
- “把这个云空间文件夹递归下载到本地”
- “给我做一个可重复执行的飞书备份流程”
- “导出一个 Base，保留结构和记录”

## 默认原则

1. **默认用 `--as user`**
   - 备份用户自己的云空间、文档、知识库时，优先 `user` 身份。
   - 只有用户明确说要备份 bot 可见资源，或当前环境只有 bot 权限时，才改成 `--as bot`。
2. **优先用脚本，不手拼命令**
   - 先用 `scripts/lark-backup.sh`，因为它把 wiki 解析、目录递归、分页导出串起来了。
3. **wiki 不能直接当真实 token**
   - 遇到 `/wiki/...` 或 `wiki_token`，必须先解析真实 `obj_type` / `obj_token`。
4. **备份目录尽量独立**
   - 脚本会优先写到独立目录，避免覆盖旧备份。

## 前置权限

最省事的方式：

```bash
lark-cli auth login --recommend
```

如果要最小权限，常见会涉及：

```bash
lark-cli auth login --scope "wiki:node:read"
lark-cli auth login --scope "space:document:retrieve"
lark-cli auth login --scope "docs:document:readonly"
lark-cli auth login --scope "docs:document:export"
lark-cli auth login --scope "sheets:spreadsheet:read"
lark-cli auth login --scope "base:app:read"
```

## 推荐入口

### 1. 自动识别单个目标

支持文档 / wiki / sheet / bitable / file / folder URL 或 token。

```bash
bash scripts/lark-backup.sh auto \
  --target "https://xxx.feishu.cn/wiki/xxxx" \
  --output-dir ./backup/wiki-page
```

如果只是 token，且脚本无法推断类型，显式传 `--type`：

```bash
bash scripts/lark-backup.sh auto \
  --target "app_xxx" \
  --type bitable \
  --output-dir ./backup/base
```

### 2. 递归备份云空间文件夹

```bash
bash scripts/lark-backup.sh folder \
  --folder-token "fldxxxx" \
  --output-dir ./backup/drive-folder
```

### 3. 结构化备份多维表格

```bash
bash scripts/lark-backup.sh base \
  --base-token "app_xxx" \
  --output-dir ./backup/base
```

## 脚本会做什么

- `auto`
  - 识别 URL / token 类型
  - wiki 先解析真实对象
  - 再路由到文档、表格、Base、文件、文件夹备份逻辑
- `folder`
  - 调 `drive files list --page-all`
  - 递归子文件夹
  - 普通文件走 `drive +download`
  - 文档 / 表格 / Base 走导出逻辑
- `base`
  - 导出一个 `xlsx` 快照
  - 保存 `+base-get`
  - 保存 `+table-list`
  - 对每张表保存 `+table-get`
  - 对每张表按页保存 `+record-list`

更细的输出目录和边界说明，见 [references/backup-layout.md](references/backup-layout.md)。

## 遇到问题时的处理

- 权限报错：回到 `lark-shared`，补 `auth login --scope ...`
- wiki token 报错：先 `wiki spaces get_node`
- 大型 Base 或大文件夹：接受脚本运行较久；不要把 `+record-list` 并发化
- `slides` / `mindnote` / 其他不在脚本覆盖范围内的类型：保留元数据并提示用户改走单独导出策略

## 本地自检

```bash
bash scripts/lark-backup.sh --help
bash scripts/lark-backup-smoke-test.sh
```

## 资源

- [references/backup-layout.md](references/backup-layout.md) — 输出结构、对象覆盖范围、已知边界
- [scripts/lark-backup.sh](scripts/lark-backup.sh) — 备份脚本
- [scripts/lark-backup-smoke-test.sh](scripts/lark-backup-smoke-test.sh) — 编号宽度与排序 smoke test
