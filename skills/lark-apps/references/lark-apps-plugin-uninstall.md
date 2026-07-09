# apps +plugin-uninstall

> **本地命令**：读当前目录的 `package.json`，在项目根目录下运行（和 npm 一样）。**不接受 `--app-id`**——它不是远端 API 命令。

卸载插件包。运行时命令事实以 `lark-cli apps +plugin-uninstall --help` 为准。

## 何时用

用户不再需要某个插件能力时，卸载对应的插件包。卸载前应先删除该插件的所有实例。

## 命令骨架

- `--name <key>`：要卸载的插件包 key。

在项目根目录下运行（和 npm 一样，无需指定路径）。

## 示例

```bash
lark-cli apps +plugin-uninstall --name <plugin-key>
```

## 输出契约

- 删除 `node_modules/{key}` + 移除 `actionPlugins` 条目。
