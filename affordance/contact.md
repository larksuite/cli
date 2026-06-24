# contact
> skill: lark-contact

## user_profiles batch_query
Bulk-fetch personal status and signature for user ids you already have.

### Avoid when
- Need more than status/signature (name, dept, email), or don't have the open_id yet → use [[+search-user]]

### Tips
- Off by default — set include_personal_status / include_description to true under query_option
- ids in user_ids must match --user-id-type (default open_id)

### Examples

**Bulk-query status and signature**
```bash
lark-cli contact user_profiles batch_query --data '{"user_ids":["ou_3a8b****6a7b"],"query_option":{"include_personal_status":true,"include_description":true}}'
```
