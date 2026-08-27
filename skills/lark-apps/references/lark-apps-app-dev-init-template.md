# apps +app-dev-init-template

在本地初始化一个产物托管形态的 Web 应用项目（代码留在本地，构建产物后续发布到妙搭）。运行时命令事实以 `lark-cli apps +app-dev-init-template --help` 为准。

## 何时用

用户要在本地开发一个 Web 应用（纯前端或全栈）并计划后续部署到妙搭时，用它初始化技术栈模板。它不创建妙搭应用、不打任何远端 API、不涉及 git/沙箱；只在本地 scaffold 项目。已有项目目录时不要用它（目标目录必须为空或不存在）。

## 命令骨架

- `--type` 与 `--template` 二选一：`--type frontend|full_stack` 用默认模板映射；`--template <短名>`（如 `vite-react`）直接指定模板包，优先于 `--type`——模板包名为 `@lark-apaas/coding-template-<短名>`。
- 可选：`--dir`，相对路径，默认 `./<模板名>`；目录已存在且非空会被拒绝。
- 前置：已完成 `lark-cli config init`（框架级要求，纯本地命令也需要）；本步骤**不需要 Node.js**。内部从 npm registry 只读下载模板包（主源 registry.npmmirror.com，失败自动降级 registry.npmjs.org 官方源） `@lark-apaas/coding-template-<模板名>` 并本地渲染，不执行任何远程脚本、不装依赖（秒级返回）。

## 示例

```bash
lark-cli apps +app-dev-init-template --type frontend --dir ./my-app
lark-cli apps +app-dev-init-template --template vite-react --dir ./demo
lark-cli apps +app-dev-init-template --type full_stack --dry-run
```

## 输出契约

返回 `data.dir`（项目目录）、`data.template`、`data.stack` 和 `data.next_steps`（后续步骤清单）。按 next_steps 引导用户：

1. `cd <dir> && npm install && npm run dev` 本地开发预览（dev 命令声明见项目根 `miaoda.json`）；
2. 需要发布时先 `lark-cli apps +create --name <name>` 创建妙搭应用；
3. 在项目根运行 `lark-cli apps +app-dev-publish --app-id <返回的 app_id>` 构建并发布（成功后 app id 写入 miaoda.json，后续免传；见 [lark-apps-app-dev-publish.md](lark-apps-app-dev-publish.md)）。

## 常见失败

- `target directory ... already exists and is not empty`：换 `--dir` 或让用户清空目录；不要擅自删除已有内容。
- `npm registry returned 404`：主源与官方源都取不到时报出，模板包可能未发布，转述 hint（联系产物侧或检查网络/registry 可达性）。
- registry 5xx / 网络失败：错误带 retryable，可稍后重试。
