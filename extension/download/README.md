# Download extension

`extension/download` is the public, transport-neutral download engine. It owns
range probing, multipart assembly, bounded retries, idle timeouts, response
validation, cancellation, and exact-length checks.

The caller supplies a `Transport`. Authentication, URL trust policy, and HTTP
error classification belong to that transport; storage belongs to the consumer
of `Stream.Body`.

Choose the representation contract deliberately:

- `download.MutableSource`: safe default. Multipart assembly requires a strong
  ETag; otherwise the engine restarts with one full response.
- `download.ImmutableSource`: permits multipart assembly without an ETag because
  the caller guarantees the source identifier pins the bytes.

External commands normally use the higher-level `command.Download`, which wires
an authenticated OpenAPI transport and invocation-scoped FileIO while retaining
the same engine options:

```go
target := command.FileTarget{Name: args.Output}
options := command.DownloadOptions{
    Representation: download.Immutable,
    Transfer: download.Options{
        PartSize: 8 << 20,
    },
}

// Dry-run reports the logical destination without opening a stream.
dryRun := command.NewDryRun(request).File(
    target.Intent("OpenAPI response body"),
)

// Execute streams and saves the file; use the host-resolved location.
artifact, err := command.Download(
    ctx,
    commandContext,
    request,
    target,
    options,
)
```

The zero `command.DownloadOptions` value selects `download.Mutable` and the
production transfer defaults. Existing targets fail by default; overwriting
requires `FileTarget{IfExists: command.IfExistsOverwrite}`.

For a pre-signed or CDN URL, use `command.DownloadURL` with the same target and
options. It accepts HTTPS only and routes through the host's external-request,
SSRF, DNS/IP pinning, and redirect policies. Dry-run should describe the file
intent without echoing a signed URL:

```go
dryRun := command.NewDryRun().
    Desc("Download an external HTTPS resource").
    File(target.Intent("external URL response body"))

artifact, err := command.DownloadURL(
    ctx,
    commandContext,
    args.URL,
    target,
    options,
)
```
