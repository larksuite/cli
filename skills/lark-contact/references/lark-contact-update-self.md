# 修改本人头像

`contact +update-self` 修改当前身份的头像。UAT 用户和使用 AAT 的 Agent Employee 均走 `--as user`，AAT 修改的是智能伙伴自己的头像。不接受他人的 ID 或管理员模式。完整参数见 `lark-cli contact +update-self --help`。

```bash
lark-cli contact +update-self --avatar-file ./avatar.png
lark-cli contact +update-self --avatar-key img_xxx --dry-run
```

## 图片与失败处理

- 本地图片文件与已上传的 image_key 二选一，不能同时传。头像 key 必须来自 `image_type=avatar` 上传，本命令上传文件时自动选择此类型。
- 单个文件必须为非空普通文件，最多 10 MiB；不能把图片 URL 当成本地路径。图片格式与像素限制由服务端校验。`--dry-run` 只预览计划，不读取图片或上传。
- 本地文件检查、读取和关闭完成后才开始上传；图片上传成功还不代表头像修改成功，以最终命令结果为准。更新失败后不要自动重试或报告已完成。
- 头像能否修改还受企业配置和当前应用授权限制。按错误响应处理，不切换管理员身份绕过限制。
