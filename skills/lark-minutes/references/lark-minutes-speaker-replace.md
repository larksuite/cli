# minutes +speaker-replace

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

替换妙记逐字稿中的说话人身份：把妙记逐字稿里"原说话人"对应的所有发言段，重新归属到"新说话人"。常用于解决妙记自动识别错说话人，或需要把外部/非飞书说话人改绑到正确飞书用户的场景。

本 skill 对应 shortcut：`lark-cli minutes +speaker-replace`。

## 典型触发表达

- "把这条妙记里 A 的发言改成 B"
- "妙记说话人识别错了，帮我把张三的部分换成李四"
- "把妙记里外部说话人 / 非飞书说话人的发言改成某个飞书用户"
- "妙记说话人修改 / 替换 / 重新归属"

## 完整工作流

识别到「修改妙记说话人」需求后，**必须**按以下顺序执行；**禁止**把展示名直接传给 `--from-speaker-id`。

1. **确认 `minute_token`**
   - 从妙记 URL、搜索或 VC 链路取得 `minute_token`。

2. **查说话人列表（必须先做）**
   - 用 **`lark-cli api`** 直接调用内部 HTTP 接口：
     ```bash
     lark-cli api GET "/open-apis/minutes/v1/minutes/<minute_token>/transcript/speakerlist" --as user
     ```
   - 返回 `data.speakers[]`，每项含 `speaker_id`（不透明 id）与 `name`（逐字稿展示名）。示例：
     ```json
     {
       "data": {
         "speakers": [
           {"speaker_id": "ENCRYPTED_TOKEN_ABC", "name": "说话人1"},
           {"speaker_id": "ENCRYPTED_TOKEN_DEF", "name": "说话人2"}
         ]
       }
     }
     ```

3. **解析 `--from-speaker-id`**
   - 根据用户描述的原说话人（展示名，如「说话人1」「张三」），在 `speakers[]` 里按 `name` **精确匹配**，取对应的 **`speaker_id`** 作为 `--from-speaker-id` 的值。
   - **`--from-speaker-id` 只传 `speaker_id`，不传展示名。**
   - 若同名有多条（`name` 相同、`speaker_id` 不同）：**不要擅自挑选**。可结合 [`vc +notes --minute-tokens`](../../lark-vc/references/lark-vc-notes.md) 对照各人发言内容，请用户确认后再用精确的 `speaker_id`。
   - 若列表中无匹配展示名：告知用户并核对拼写，或请用户在妙记页面确认标签。

4. **解析 `--to-user-id`**
   - 新说话人必须是 `ou_` 开头的 open_id。用户只给姓名时，先用 [lark-contact](../../lark-contact/SKILL.md) 解析。

5. **执行替换**
   ```bash
   lark-cli minutes +speaker-replace \
     --minute-token obcnxxxxxxxxxxxxxxxxxxxx \
     --from-speaker-id ENCRYPTED_TOKEN_ABC \
     --to-user-id ou_new_speaker_open_id
   ```

## 命令示例

```bash
# 1. 先查列表（裸调 HTTP）
lark-cli api GET "/open-apis/minutes/v1/minutes/obcnxxxxxxxxxxxxxxxxxxxx/transcript/speakerlist" --as user

# 2. 再替换（from-speaker-id 来自上一步的 speaker_id）
lark-cli minutes +speaker-replace \
  --minute-token obcnxxxxxxxxxxxxxxxxxxxx \
  --from-speaker-id ENCRYPTED_TOKEN_ABC \
  --to-user-id ou_new_speaker_open_id
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记的唯一标识，可从妙记 URL 末尾路径提取 |
| `--from-speaker-id <id>` | 是 | 被替换的原说话人 **`speaker_id`**（来自 speakerlist API 的 `data.speakers[].speaker_id`） |
| `--to-user-id <ou_xxx>` | 是 | 新的说话人，**必须是 `ou_` 开头的 open_id**，不支持用户名 |

## 核心约束

### 1. 必须先查 speakerlist，再替换

Agent 必须先 `lark-cli api GET .../speakerlist`，再 `+speaker-replace`；`--from-speaker-id` 只接受 `speaker_id`。

### 2. 新说话人必须是 open_id

`--to-user-id` 仅支持 `ou_` 开头的 open_id，**不支持直接传姓名**；如果用户只给了姓名，请先用 [lark-contact](../../lark-contact/SKILL.md) 把姓名解析成 `open_id`。

### 3. 历史参数

存在一个隐藏的历史参数 `--from-user-id`（飞书说话人的 open_id），仅为向后兼容保留；新流程请一律使用 `--from-speaker-id` + `speaker_id`。

## 认证与权限

- 所需 scope：`minutes:minutes:readonly`（内部解析说话人）、`minutes:minutes:update`（执行替换）。

## 输出结果

| 字段 | 说明 |
|------|------|
| `minute_token` | 被修改的妙记 Token，与输入的 `--minute-token` 一致 |
| `from_speaker_id` | 实际用于替换的不透明说话人标识 |
| `to_user_id` | 替换后的新说话人 open_id，与输入的 `--to-user-id` 一致 |

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关功能说明
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
