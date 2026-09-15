# Windows 平台子进程 stdin UTF-8 字节流传输规范

在 Windows 平台通过子进程管道（stdin）向 `lark-cli` 推送文档文本（如 `docs +update --content -`）时，须遵循以下编码规范：

## 核心机理与常见陷阱

- **陷阱**：Windows 控制台默认代码页通常为 GBK/CP936。若在 Python 等子进程中使用 `text=True` 或依赖系统默认编码推送 stdin，管道传输的中文会被 `lark-cli`（按 UTF-8 解码）解析为乱码（如出现替代字符 ）。
- **规范**：向 `lark-cli` stdin 传递数据时，必须显式以 UTF-8 字节流模式传输。

## 编程语言标准调用示例

### Python
```python
import subprocess

# 正确做法：显式编码为 UTF-8 字节流推入 stdin
proc = subprocess.Popen(
    ["lark-cli", "docs", "+update", "--doc", doc_id, "--command", "overwrite", "--content", "-"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    shell=True  # Windows 下调用 .cmd 脚本需要
)
stdout, stderr = proc.communicate(input=xml_content.encode("utf-8"))

# 错误做法（导致乱码）：
# subprocess.run(cmd, input=xml_content, text=True)
```

### Node.js
```javascript
const { spawn } = require('child_process');

const child = spawn('lark-cli', ['docs', '+update', '--doc', docId, '--command', 'overwrite', '--content', '-'], { shell: true });
child.stdin.write(Buffer.from(xmlContent, 'utf-8'));
child.stdin.end();
```
