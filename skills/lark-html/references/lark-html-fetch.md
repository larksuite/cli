# html +fetch

根据 HTML token 下载服务端提供的源码 ZIP，不解压、不还原文件树。妙搭应用的源码使用 apps +export。

## 用法

```bash
lark-cli html +fetch --token <html_token> --output ./downloads/src.zip
lark-cli html +fetch --token <html_token>
lark-cli html +fetch --token <html_token> --output ./src.zip --dry-run
```

`--token` 必填，只传 HTML token，不传整条页面 URL。`--output` 是完整的相对文件路径，省略时保存为当前目录的 `<token>.zip`。要放进 downloads 目录，明确写 `./downloads/src.zip`；传目录不会自动补文件名。

文件保存、覆盖规则和父目录创建遵循公共 FileIO。默认本地实现原子写入，成功下载后替换目标文件；下载错误或中断时保留原文件，不留下部分 ZIP。命令不展开 ZIP 内的路径。

## 权限与错误

支持用户及 Agent Employee profile，不支持普通应用 bot 身份；Scope 为 `ccm:html:download`，同时需要服务端的对象下载权限。公开页面的访问权限不代表源码下载权限。缺少 Scope 时使用公共授权流程，切勿传入 UserID 或 TenantID 绕过鉴权。

成功 JSON envelope 的 data 包含 `token`、`output`（保存位置）和 `size_bytes`。HTTP／OpenAPI 错误按公共错误契约返回，不会把 JSON 错误体保存成 ZIP。源码下载尚未启用或服务端依赖未就绪时，应先排查服务端状态。

沿用当前 profile 的 brand 选择国内或海外网关，不增加本命令专属区域参数。命令参数的最新事实以 `lark-cli html +fetch --help` 为准。
