---
name: lark-unterminated
description: Fixture for a genuinely unterminated quote inside a fence
---

```bash
lark-cli docs +fetch --doc A3Ijbroken --note 'oops
```

Prose between fences must not be absorbed into the broken example.

```bash
lark-cli im +chat-list --page-size 20
lark-cli docs +fetch --doc A3Ijlater
```
