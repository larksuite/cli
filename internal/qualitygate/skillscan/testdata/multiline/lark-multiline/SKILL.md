---
name: lark-multiline
description: Fixture for quoted arguments that span lines
---

```bash
# a quoted flag value carries the word across lines without a trailing backslash
lark-cli base +data-query \
  --base-token MAGObdemo \
  --dsl '{
    "datasource": {"type": "table", "table": {"tableId": "tbldemo"}},
    "shaper": {"format": "flat"}
  }'

# a double-quoted value does the same
lark-cli docs +fetch --doc A3Ijdemo --note "line one
line two"
```
