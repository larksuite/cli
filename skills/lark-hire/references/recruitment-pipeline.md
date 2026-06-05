# 招聘管线（只读）工作流

把"职位 → 投递 → Offer"串起来查询。全部只读，无需确认。

## 1. 选定职位
```bash
lark-cli hire job list --page-all -q '.data.items[] | select(.title | test("工程师")) | {id, title}'
```

## 2. 该职位的投递管线
```bash
lark-cli hire application list --params '{"job_id":"<job_id>"}' --page-all \
  -q '.data.items[]'                       # 投递列表（部分接口返回 application_id）
lark-cli hire application get --params '{"application_id":"<id>"}'   # 投递详情 .data.application
```

## 3. 候选人维度
```bash
# 手机/邮箱 -> talent_id
lark-cli hire talent batch_get_id --data '{"mobile_code":"86","mobile_number_list":["138..."]}'
lark-cli hire talent get --params '{"talent_id":"<id>"}'            # .data.talent
lark-cli hire application list --params '{"talent_id":"<id>"}'      # 其投递
lark-cli hire offer list --params '{"talent_id":"<id>"}'           # 其 Offer（talent_id 必填）
```

## 提示
- 参数（路径+查询）统一放 `--params` 的 JSON；请求体放 `--data`。
- 字段以 `lark-cli schema hire.<resource>.<method>` 与官方文档为准。
- 大量数据用 `--format csv > out.csv` 导出后分析。
