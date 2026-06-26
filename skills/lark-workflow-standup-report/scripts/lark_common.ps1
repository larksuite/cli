Set-StrictMode -Version 3.0

function Resolve-LarkCliInvocation {
    $bundledNode = 'C:\Users\Leo\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
    $runJs = 'C:\Users\Leo\AppData\Roaming\npm\node_modules\@larksuite\cli\scripts\run.js'

    if ($env:LARK_CLI_NODE -and $env:LARK_CLI_RUNJS -and
        (Test-Path -LiteralPath $env:LARK_CLI_NODE) -and
        (Test-Path -LiteralPath $env:LARK_CLI_RUNJS)) {
        return @{
            Mode = 'node'
            Exe = $env:LARK_CLI_NODE
            PrefixArgs = @($env:LARK_CLI_RUNJS)
        }
    }

    if ((Test-Path -LiteralPath $bundledNode) -and (Test-Path -LiteralPath $runJs)) {
        return @{
            Mode = 'node'
            Exe = $bundledNode
            PrefixArgs = @($runJs)
        }
    }

    $cmd = Get-Command lark-cli -ErrorAction SilentlyContinue
    if ($cmd) {
        return @{
            Mode = 'native'
            Exe = $cmd.Source
            PrefixArgs = @()
        }
    }

    throw 'Cannot locate lark-cli. Set LARK_CLI_NODE and LARK_CLI_RUNJS, or install lark-cli in PATH.'
}

function Invoke-LarkCliJson {
    param(
        [Parameter(Mandatory = $true)]
        [AllowEmptyString()]
        [string[]] $Args,

        [Parameter(Mandatory = $true)]
        [string] $OutFile,

        [int] $TimeoutSeconds = 120
    )

    $invocation = Resolve-LarkCliInvocation
    $allArgs = @($invocation.PrefixArgs) + $Args

    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $invocation.Exe
    foreach ($arg in $allArgs) {
        [void]$psi.ArgumentList.Add($arg)
    }
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true

    $proc = [System.Diagnostics.Process]::Start($psi)
    $stdoutTask = $proc.StandardOutput.ReadToEndAsync()
    $stderrTask = $proc.StandardError.ReadToEndAsync()
    if (-not $proc.WaitForExit($TimeoutSeconds * 1000)) {
        try { $proc.Kill() } catch {}
        try { $proc.WaitForExit() } catch {}
        throw "lark-cli timed out: $($Args -join ' ')"
    }

    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
    $payload = if ($stdout.Trim()) { $stdout } else { $stderr }

    Set-Content -LiteralPath $OutFile -Value $payload -Encoding UTF8

    try {
        $json = $payload | ConvertFrom-Json
        if ($json -and $json.PSObject.Properties['ok'] -and $json.ok -eq $false) {
            $errorType = if ($json.PSObject.Properties['error'] -and $json.error.PSObject.Properties['type']) { $json.error.type } else { 'unknown' }
            $errorMessage = if ($json.PSObject.Properties['error'] -and $json.error.PSObject.Properties['message']) { $json.error.message } else { 'lark-cli returned ok=false' }
            throw "lark-cli returned ok=false ($errorType): $errorMessage"
        }
    } catch {
        if ($_.Exception.Message -like 'lark-cli returned ok=false*') { throw }
    }

    return [pscustomobject]@{
        ExitCode = $proc.ExitCode
        Stdout = $stdout
        Stderr = $stderr
        OutFile = $OutFile
    }
}

function Read-JsonFileOrNull {
    param([Parameter(Mandatory = $true)][string] $Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $null }
    $raw = Get-Content -LiteralPath $Path -Raw
    if (-not $raw.Trim()) { return $null }
    try { return $raw | ConvertFrom-Json } catch { return $null }
}

function Protect-Text {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string] $Text)

    $safe = $Text
    $safe = $safe -replace '(?i)(Key:\s*)[A-Za-z0-9+/=_-]{16,}', '$1[REDACTED]'
    $safe = $safe -replace '(?i)(api[_ -]?key["''\s:=]+)[A-Za-z0-9+/=_-]{12,}', '$1[REDACTED]'
    $safe = $safe -replace '(?i)((?:access|refresh|tenant|user|app|authorization)[_-]?token["''\s:=]+)[A-Za-z0-9._+/=_-]{16,}', '$1[REDACTED]'
    $safe = $safe -replace '(?i)(secret["''\s:=]+)[A-Za-z0-9._+/=_-]{12,}', '$1[REDACTED]'
    $safe = $safe -replace '(?i)(password["''\s:=]+)[^\s,''"}]{6,}', '$1[REDACTED]'
    $safe = $safe -replace '(?i)(private[-_ ]?key["''\s:=]+)[A-Za-z0-9._+/=_-]{16,}', '$1[REDACTED]'
    return $safe
}

function Protect-JsonFiles {
    param([Parameter(Mandatory = $true)][string] $Directory)

    Get-ChildItem -LiteralPath $Directory -Filter '*.json' -File -ErrorAction SilentlyContinue | ForEach-Object {
        $raw = Get-Content -LiteralPath $_.FullName -Raw
        $safe = Protect-Text -Text $raw
        if ($safe -ne $raw) {
            Set-Content -LiteralPath $_.FullName -Value $safe -Encoding UTF8
        }
    }
}

function Count-JsonArray {
    param($Value)
    if ($null -eq $Value) { return 0 }
    if ($Value -is [array]) { return $Value.Count }
    return 1
}

function Get-LarkImItems {
    param($Json)

    if ($null -eq $Json -or -not $Json.PSObject.Properties['data']) { return @() }
    if ($Json.data.PSObject.Properties['messages'] -and $Json.data.messages) { return @($Json.data.messages) }
    if ($Json.data.PSObject.Properties['message_ids'] -and $Json.data.message_ids) { return @($Json.data.message_ids) }
    if ($Json.data.PSObject.Properties['items'] -and $Json.data.items) { return @($Json.data.items) }
    return @()
}

function Test-LarkImDetailsMissing {
    param($Json)

    if ($null -eq $Json -or -not $Json.PSObject.Properties['data']) { return $false }
    $hasMessageIds = $Json.data.PSObject.Properties['message_ids'] -and $Json.data.message_ids
    $hasMessages = $Json.data.PSObject.Properties['messages'] -and $Json.data.messages
    $note = if ($Json.data.PSObject.Properties['note']) { [string]$Json.data.note } else { '' }
    return ($hasMessageIds -and -not $hasMessages) -or ($note -match 'failed to fetch message details')
}
