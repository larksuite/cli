# Mail attachment input

`--attach` is a repeated literal path, not a comma-separated list. Quote each
argument and repeat the flag for each file. Commas and spaces within a filename
are preserved through validation, classification and MIME construction. This
also applies to mail template attachment flags. Previously undocumented
comma-list splitting is intentionally removed to avoid attaching unintended
files. Recipient comma-list parsing is unchanged.

## PDF bytes from stdin

`+draft-create`, `+send`, `+reply`, `+reply-all` and `+forward` additionally accept:

```
--attach-stdin-name 'Invoice 521,625.00.pdf' --attach-stdin-type application/pdf
```

These flags read one binary PDF from stdin during Execute, with a hard 10 MiB
limit (the reader consumes at most limit + 1 bytes). The explicit filename must
be a basename ending in `.pdf`, not a path, and must have no controls, separators
or colon. The explicit type must be exactly `application/pdf`. The bytes must
begin `%PDF-`; this is a format marker check, **not** structural PDF validation
or a malware scan. Bytes, including NUL and non-UTF-8 bytes, are preserved.

Only feed bytes you are authorized to disclose, from an authorized producer.
This interface does not open, copy, relocate or grant access to any original
file. All existing `--attach`, inline, body-file and other path validation,
including the built-in `/root` denylist, remains intact. Never move protected
files to evade that policy. The caller remains responsible for source access
and disclosure authorization; stdin cannot prove provenance.

The command shares a single-use stdin claim with framework text inputs. The
stdin PDF is held in memory, not written to a temporary file. Its size/count
contribute to existing attachment limits/classification. It remains a normal
attachment using `application/octet-stream` on the wire, matching the mail
backend's attachment-versus-inline behavior. If combined attachments require
large-attachment upload, existing policy still applies to the path attachments.

Dry-run validates metadata without reading stdin or claiming byte validation.
The existing default draft behavior and explicit send confirmation are unchanged.
This is CLI code only; no deployment or live send is required to test it.

## Offline verification

```
go test ./shortcuts/mail ./shortcuts/common -run 'TestStdinAttachment|TestComposeStdinAttachment|TestComposeAttachment|TestReadStdinBytes|TestAttachmentProtected' -count=1 -v
go build .
```

The mounted-command MIME tests use the repository HTTP mock transport, not a
real mailbox. Literal filename parsing tests use a test-only injected metadata
backend because the checkout and test scratch live inside protected `/root`.
Production path behavior is separately tested to remain denied.
