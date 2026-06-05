# minutes +update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

修改飞书妙记的标题（topic）。

本 skill 对应 shortcut：`lark-cli minutes +update`。

## 典型触发表达

- "把这个妙记的标题改成 xxx"
- "重命名这条妙记"
- "修改妙记标题"

## 命令示例

```bash
lark-cli minutes +update --minute-token xxx --topic "周会纪要 2026-05-18"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记的唯一标识，可从妙记 URL 末尾路径提取 |
| `--topic <string>` | 是 | 新的妙记标题 |

## 认证与权限
- 所需 scope：`minutes:minutes:update`。

## 输出结果

| 字段 | 说明 |
|------|------|
| `minute_token` | 被修改的妙记 Token，与输入的 `--minute-token` 一致，可继续用于查询妙记信息、下载媒体或获取纪要产物 |
| `topic` | 修改后的妙记标题，与输入的 `--topic` 一致 |

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关功能说明
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
