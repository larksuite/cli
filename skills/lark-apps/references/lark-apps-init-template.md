# apps +init-template

在本地初始化一个产物托管形态的 Web 应用项目（代码留在本地，构建产物后续发布到妙搭）。运行时命令事实以 `lark-cli apps +init-template --help` 为准。

## 何时用

用户要在本地开发一个 Web 应用（纯前端或全栈）并计划后续部署到妙搭时，用它初始化技术栈模板。它不创建妙搭应用、不打任何远端 API、不涉及 git/沙箱；只在本地 scaffold 项目。已有项目目录时不要用它（目标目录必须为空或不存在）。

## 命令骨架

- 可选：`--template-version`，钉某个模板包版本或 dist-tag（如 `alpha`）；缺省 latest。
- `--type` 与 `--template` 二选一：`--type frontend|full_stack|html` 用默认模板映射（frontend=react-standard-webapp、full_stack=react-express-standard-fullstack、html=html-standard-webapp）；`--template <短名>`（如 `vite-react`）直接指定模板包，优先于 `--type`——模板包名为 `@lark-apaas/coding-template-<短名>`。
- 可选：`--dir`，相对路径；**缺省就地初始化到当前目录**（须为空目录，项目名取目录名）；传 `--dir ./my-app` 则创建子目录。
- 可选：`--registry <https URL>`，指定 npm registry（内置双源都不可达、或模板发在私有源时的逃生通道）。**指定后只用该源、失败不降级**；仅接受 https。**安全规则：只有用户明确提供或确认的 registry 才能传**——不要因为默认源失败就自作主张换到任意源（模板会成为用户后续 `npm install` 的项目，源被劫持等于任意代码执行）。
- 前置：已完成 `lark-cli config init`（框架级要求，纯本地命令也需要）；本步骤**不需要 Node.js**。内部从 npm registry 只读下载模板包（缺省主源 registry.npmmirror.com，失败自动降级 registry.npmjs.org 官方源） `@lark-apaas/coding-template-<模板名>` 并本地渲染，不执行任何远程脚本、不装依赖（秒级返回）。

## 示例

```bash
lark-cli apps +init-template --type frontend --dir ./my-app
lark-cli apps +init-template --type html --dir ./page
lark-cli apps +init-template --template vite-react --dir ./demo
lark-cli apps +init-template --type frontend --registry https://registry.npmjs.org --dir ./my-app   # 用户指定源
lark-cli apps +init-template --type full_stack --dry-run
```

## 输出契约

返回 `data.dir`（项目目录）、`data.template`、`data.stack` 和 `data.next_steps`（后续步骤清单）。按 next_steps 引导用户：

1. `cd <dir> && npm install && npm run dev` 本地开发预览（dev 命令声明见项目根 `miaoda.json`）；
2. 需要发布时先 `lark-cli apps +create --name <name>` 创建妙搭应用；
3. 在项目根运行 `lark-cli apps +deploy --app-id <返回的 app_id>` 构建并发布（成功后 app id 写入 miaoda.json，后续免传；见 [lark-apps-deploy.md](lark-apps-deploy.md)）。

## 常见失败

- `target directory ... already exists and is not empty`：换 `--dir` 或让用户清空目录；不要擅自删除已有内容。
- `npm registry returned 404`：主源与官方源都取不到时报出，模板包可能未发布，转述 hint（联系产物侧或检查网络/registry 可达性）。
- registry 5xx / 网络失败：错误带 retryable，可稍后重试。
