---
name: lark-attendance
version: 2.0.0
description: "飞书考勤打卡：查询自己的打卡结果（迟到/早退/缺卡/补卡）。当用户问自己某天有没有打卡、某段时间的考勤/出勤情况时使用；仅限本人。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli attendance --help"
---

# attendance

仅本人（`--as user`）。各命令的参数细节见其 `--help`。

## 路由

| 想做什么 | 怎么调 |
|---|---|
| 查自己的打卡（迟到/早退/缺卡/出勤） | `+records --from <YYYY-MM-DD>`（区间加 `--to`）|
| 要打卡时刻 / 地点 / 考勤组等明细 | 加 `--detail`（默认只给结果枚举）|
| 相对时间（今天/本周/上月） | 先换算成具体 `YYYY-MM-DD`——命令只认具体日期 |
| 查他人 / 团队考勤 | 出本 skill 范围（需管理员）→ 见 `lark-openapi-explorer` |

## 示例

```bash
# 查一段时间的打卡结果
lark-cli attendance +records --as user --from 2026-06-01 --to 2026-06-08

# 要明细（几点几分、在哪打的）
lark-cli attendance +records --as user --from 2026-06-01 --detail
```

## 注意

- 空 `items` = 无记录（非报错）。
- 认证 / 权限报错见 `lark-shared`。
