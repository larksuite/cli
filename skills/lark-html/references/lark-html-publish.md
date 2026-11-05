# html +publish

发布本地 HTML 入口及其引用的本地 CSS、JS、图片等静态资源。此命令操作 HTML 单品，不是妙搭应用；已有 app_id 的应用使用 apps +deploy。

## 用法

```bash
lark-cli html +publish --file-path ./page.html --dry-run
lark-cli html +publish --file-path ./page.html
lark-cli html +publish --file-path ./page.html --token <html_token>
```

`--file-path` 必填，相对当前工作目录，指向非空 HTML 文件。命令递归收集可识别的本地资源引用；远程资源仍依赖其原 URL，不会下载打包。无法收集的依赖会给出提示。动态拼接或运行时加载的路径不能保证自动发现，发布前请检查页面引用。

入口部署为根 `index.html`，其他资源保持相对于入口目录的路径。上传最多 99 个非空文件，文件总计不超过 20,000,000 字节；服务端可配置更严格的限制。入口与另一个 index.html 冲突时需要先调整文件命名。

## 更新与幂等

`--token` 可选，只传 HTML token。当前用户有该对象的编辑权限时，将内容发布到该 token，不要求入口路径与首次发布相同，也不要求当前用户是创建人。没有编辑权限时，服务端走入口路径对应的现有幂等发布流程；始终使用返回的 `data.token` 和 `data.link_url`，它们可能与指定目标不同。权限服务故障等错误不会自动创建新文档。

不指定 `--token` 时，命令以原始入口的规范绝对路径生成 IdempotentKey，不以映射后的 index.html 命名。保持该路径可重复发布同一文档；移动或重命名入口会得到不同 Key。网络失败后可用同一命令重试。

HashTag 复用 apps +deploy：单文件为内容 SHA-256；多文件按部署路径与签名排序计算，非文本资源的签名只包含大小。CLI 每次仍发送完整文件，不凭 HashTag 在本地跳过请求；服务端保持现有的客户端 HashTag 比较规则，ContentHash 当前不参与发布判断。

## 权限与输出

支持用户及 Agent Employee profile，不支持普通应用 bot 身份；Scope 为 `ccm:html:publish`。用户必须具备对象发布权限；授权、Token 和国内／海外域名由公共客户端及当前 profile 处理。

成功 JSON envelope 的 data 包含 `token`、`link_url`；后续 fetch 使用该 token。`--dry-run` 只展示目标 token（如有）、部署路径、Key、摘要和文件数量，不发送请求，不展示文件内容。发布内容会用于公开页面，执行前应确认入口及其依赖符合用户的发布意图。

命令参数的最新事实以 `lark-cli html +publish --help` 为准。
