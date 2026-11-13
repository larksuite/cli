# Multiple PDF attachments from binary stdin

All five compose shortcuts (`mail +draft-create`, `+send`, `+reply`, `+reply-all`,
`+forward`) accept `--attach-stdin-manifest` as a **literal JSON argv value**:

```json
[{"filename":"Invoice 521,625.00.pdf","size":12345},{"filename":"Receipt.pdf","size":67890}]
```

Write the original binary PDF payloads to stdin, concatenated in the same order.
`size` is the exact number of bytes for that entry, not an encoded length.
Do not base64-encode, add separators, or append a newline. The manifest is not
an `@file` or stdin input and never opens attachment paths.

Rules:

- 1–250 entries; at most 10 MiB (10,485,760 bytes) **combined** payload.
- Each entry has exactly one `filename` and one integer `size` (at least 5).
  Unknown, duplicate, case-aliased keys and trailing JSON are rejected.
- Filenames must be valid UTF-8 PDF basenames (case-insensitive `.pdf`), with no
  slash, backslash, colon, control characters, or surrounding whitespace.
  Commas and non-ASCII names are preserved. Filenames need not be unique.
- Each declared segment must begin `%PDF-`. Truncated streams, trailing bytes,
  and non-PDF segments fail before the compose draft is created.
- Exclusive with `--attach-stdin-name` / `--attach-stdin-type` and any other
  stdin consumer. Existing single-file flags remain supported unchanged.
- PDF-only type is implicit in this manifest format; attachments are emitted as
  `application/octet-stream` with attachment disposition for backend compatibility.
- Existing attachment count and EML limits include every stdin PDF. Regular
  `--attach` paths still use the unchanged literal-path guard and denylist.
- `--dry-run` validates metadata only, not stdin bytes. PDF magic validation is
  not malware scanning or authorization. Only supply user-authorized bytes.
- Commands remain draft-first. This flag never implies `--confirm-send`.

Example: Python caller with four authorized original PDF byte buffers, no
intermediate copies or relocated files (no account call occurs until you run it):

```python
import json
import subprocess

# attachments = [(basename, original_pdf_bytes), ...]  # all four PDFs
manifest = json.dumps([
    {"filename": name, "size": len(data)} for name, data in attachments
], ensure_ascii=False)
subprocess.run([
    "/path/to/lark-cli-multi-stdin", "mail", "+draft-create",
    "--to", "recipient@example.com", "--subject", "Payment request",
    "--body", "Please review the attached payment documents.",
    "--attach-stdin-manifest", manifest,
], input=b"".join(data for _, data in attachments), check=True)
```

Do not add `--confirm-send`. Signature behavior is unchanged by this patch.
Mounted HTTP-mock tests decode the EML for all five compose paths and verify
four attachment filenames (including comma and Thai names), octet-stream MIME,
attachment disposition, exact binary bytes, and count. No account/network
requests or deployment are needed for these tests.
