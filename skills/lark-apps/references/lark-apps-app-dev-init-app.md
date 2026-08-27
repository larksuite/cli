# apps +app-dev-init-app

在本地初始化一个产物托管形态的 Web 应用项目（代码留在本地，构建产物后续发布到妙搭）。运行时命令事实以 `lark-cli apps +app-dev-init-app --help` 为准。

## 何时用

用户要在本地开发一个 Web 应用（纯前端或全栈）并计划后续部署到妙搭时，用它初始化技术栈模板。它不创建妙搭应用、不打任何远端 API、不涉及 git/沙箱；只在本地 scaffold 项目。已有项目目录时不要用它（目标目录必须为空或不存在）。

## 命令骨架

- 必填：`--type`，取值 `frontend`（映射模板 react-standard-webapp）或 `full_stack`（映射 react-express-standard-fullstack）。
- 可选：`--dir`，相对路径，默认 `./<模板名>`；目录已存在且非空会被拒绝。
- 前置：本机需有 Node.js（提供 npx）。内部执行 `npx @lark-apaas/miaoda-cli app init --template <模板名> --skip-install` 完成 scaffold，默认不装依赖（秒级返回）。

## 示例

```bash
lark-cli apps +app-dev-init-app --type frontend --dir ./my-app
lark-cli apps +app-dev-init-app --type full_stack --dry-run
```

## 输出契约

返回 `data.dir`（项目目录）、`data.template`、`data.stack` 和 `data.next_steps`（后续步骤清单）。按 next_steps 引导用户：

1. `cd <dir> && npm install && npm run dev` 本地开发预览；
2. 需要发布时先 `lark-cli apps +create --name <name>` 创建妙搭应用，把返回的 `app_id` 写入项目根的 `.spark/meta.json`；
3. 在项目根运行 `lark-cli apps +app-dev-publish` 构建并发布（见 [lark-apps-app-dev-publish.md](lark-apps-app-dev-publish.md)）。

## 常见失败

- `npx executable not found on PATH`：本机没装 Node.js，转述 hint 让用户安装。
- `target directory ... already exists and is not empty`：换 `--dir` 或让用户清空目录；不要擅自删除已有内容。
- `npx app init failed`：多为网络或 registry 问题，转述 stderr 摘要；模板 registry 是 registry.npmmirror.com。
