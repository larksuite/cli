# apps +plugin-install

> **本地命令**：读当前目录的 `package.json`，在项目根目录下运行（和 npm 一样）。**不接受 `--app-id`**——它不是远端 API 命令。

安装插件包到项目。运行时命令事实以 `lark-cli apps +plugin-install --help` 为准。

## 何时用

用户要接入 AI 能力或飞书平台能力，需要先安装对应的插件包。安装后才能创建插件实例。具体有哪些可用插件、该选哪个，读取创建的应用仓库 Skill：`.agents/skills/plugin-guide/SKILL.md`。

**插件包 ≠ npm 包**：插件包写入 `actionPlugins`，npm 写入 `dependencies`，两套独立机制。禁止用 `npm install` 代替本命令。

## 命令骨架

- `--name <key>`：插件包 key（从仓库 Skill 的「AI 插件目录」获取）。不传则批量安装 `actionPlugins` 中声明的所有插件。
- `--version <ver>`：指定版本（如 `1.0.0`）。不传则安装最新版。

在项目根目录下运行（和 npm 一样，无需指定路径）。

## 示例

```bash
# 安装最新版
lark-cli apps +plugin-install --name <plugin-key>

# 安装指定版本
lark-cli apps +plugin-install --name <plugin-key> --version 1.0.0

# 批量安装已声明的所有插件
lark-cli apps +plugin-install
```

## 输出契约

- 已安装同版本会跳过（status=already_installed）。
- 失败时 hint 指示原因（网络/版本不存在/package.json 缺失）。
