param(
    [Parameter(Mandatory = $true)]
    [string] $MarkdownPath,

    [Parameter(Mandatory = $true)]
    [string] $Title,

    [Parameter(Mandatory = $true)]
    [string] $WikiNode,

    [Parameter(Mandatory = $true)]
    [string] $ResponsePath,

    [switch] $DryRun
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $MarkdownPath)) {
    throw "MarkdownPath not found: $MarkdownPath"
}

$parent = Split-Path -Parent $ResponsePath
if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }

if ($DryRun) {
    [ordered]@{
        dry_run = $true
        title = $Title
        wiki_node = $WikiNode
        markdown_path = $MarkdownPath
        response_path = $ResponsePath
    } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $ResponsePath -Encoding UTF8
    Write-Output $ResponsePath
    exit 0
}

$markdown = Get-Content -LiteralPath $MarkdownPath -Raw
$content = if ($markdown -match '^\s*#\s+') {
    $markdown
} else {
    "# $Title`r`n`r`n$markdown"
}

$tempContentPath = Join-Path $parent 'daily_doc_create_content.md'
Set-Content -LiteralPath $tempContentPath -Value $content -Encoding UTF8

try {
    $payload = & lark-cli docs +create --as user --parent-token $WikiNode --doc-format markdown --content "@$tempContentPath" 2>&1 | Out-String
    Set-Content -LiteralPath $ResponsePath -Value $payload -Encoding UTF8

    if ($LASTEXITCODE -ne 0) {
        throw "lark-cli create failed: $payload"
    }
} finally {
    if (Test-Path -LiteralPath $tempContentPath) {
        Remove-Item -LiteralPath $tempContentPath -Force
    }
}

Write-Output $ResponsePath
