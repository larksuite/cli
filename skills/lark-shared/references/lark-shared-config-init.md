# 首次配置 lark-cli

首次使用需运行 `lark-cli config init --new` 完成应用配置。

**注意：`config init` 是阻塞命令，没有 `--no-wait`，不要套用 `auth login` 的 split-flow。** 它会一直阻塞到用户在浏览器完成配置或过期。帮用户初始化时，用 background 方式执行命令，启动后读取输出，从中提取授权链接发给用户：

```bash
lark-cli config init --new
```

输出里的授权 URL 按正文准则处理（生成二维码、URL 原样不改写）。
