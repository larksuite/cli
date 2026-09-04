param(
    [Parameter(Mandatory = $true)]
    [string] $Date,

    [Parameter(Mandatory = $true)]
    [string] $Start,

    [Parameter(Mandatory = $true)]
    [string] $End,

    [Parameter(Mandatory = $true)]
    [string] $OutDir,

    [string[]] $ProjectRoots = @('D:\AICODING', 'D:\除二\AI code'),

    [string[]] $ImportantProjects = @('AIEXCEL', 'Swimlane', 'Wardrobe'),

    [int] $MaxFilesPerProject = 40
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$startDto = [DateTimeOffset]::Parse($Start)
$endDto = [DateTimeOffset]::Parse($End)
$startLocal = $startDto.DateTime
$endLocal = $endDto.DateTime
$codexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $env:USERPROFILE '.codex' }
$claudeHome = Join-Path $env:USERPROFILE '.claude'

function Test-InRange {
    param([datetime] $Time)
    return ($Time -ge $startLocal -and $Time -le $endLocal)
}

function Read-JsonLine {
    param([string] $Line)
    try { return $Line | ConvertFrom-Json } catch { return $null }
}

function Limit-Text {
    param(
        [AllowNull()][string] $Text,
        [int] $Max = 240
    )
    if (-not $Text) { return $null }
    $clean = ($Text -replace '\s+', ' ').Trim()
    if ($clean.Length -le $Max) { return $clean }
    return $clean.Substring(0, $Max) + '...'
}

function Get-CodexPayloadText {
    param($Payload)
    if ($null -eq $Payload) { return $null }
    $messageProp = $Payload.PSObject.Properties['message']
    if ($messageProp -and $messageProp.Value) { return [string]$messageProp.Value }
    $contentProp = $Payload.PSObject.Properties['content']
    if ($contentProp -and $contentProp.Value) {
        $parts = @()
        foreach ($c in @($contentProp.Value)) {
            $textProp = $c.PSObject.Properties['text']
            $typeProp = $c.PSObject.Properties['type']
            if ($textProp -and $textProp.Value) { $parts += [string]$textProp.Value }
            elseif ($typeProp -and $typeProp.Value -eq 'output_text' -and $textProp -and $textProp.Value) { $parts += [string]$textProp.Value }
        }
        if ($parts.Count -gt 0) { return ($parts -join ' ') }
    }
    return $null
}

function Read-CodexSessionSummary {
    param([string] $Path)

    $meta = $null
    $userMessages = New-Object System.Collections.Generic.List[string]
    $assistantMessages = New-Object System.Collections.Generic.List[string]
    $commands = New-Object System.Collections.Generic.List[string]
    $mentionedPaths = New-Object System.Collections.Generic.List[string]

    if (-not (Test-Path -LiteralPath $Path)) {
        return [pscustomobject]@{
            path = $Path
            cwd = $null
            session_id = $null
            user_messages = @()
            assistant_messages = @()
            commands = @()
            mentioned_paths = @()
        }
    }

    Get-Content -LiteralPath $Path -ErrorAction SilentlyContinue | ForEach-Object {
        $j = Read-JsonLine $_
        if ($null -eq $j) { return }

        if ($j.type -eq 'session_meta') {
            $meta = $j.payload
            return
        }

        if ($j.type -ne 'response_item' -or $null -eq $j.payload) { return }
        $payload = $j.payload

        if ($payload.type -eq 'function_call') {
            $cmd = $payload.name
            if ($payload.arguments) {
                $argText = Limit-Text -Text ([string]$payload.arguments) -Max 180
                if ($argText) { $cmd = "$cmd $argText" }
            }
            if ($cmd -and $commands.Count -lt 20) { $commands.Add($cmd) }
            return
        }

        if ($payload.type -ne 'message') { return }
        $text = Get-CodexPayloadText -Payload $payload
        $short = Limit-Text -Text $text -Max 260
        if (-not $short) { return }

        foreach ($m in [regex]::Matches($short, '[A-Za-z]:\\[^`"''\)\]\s]+')) {
            if ($mentionedPaths.Count -lt 20) { $mentionedPaths.Add($m.Value) }
        }

        if ($payload.role -eq 'user') {
            if ($short -match 'AGENTS\.md instructions') { return }
            if ($userMessages.Count -lt 8) { $userMessages.Add($short) }
        } elseif ($payload.role -eq 'assistant') {
            if ($assistantMessages.Count -lt 16) { $assistantMessages.Add($short) }
        }
    }

    return [pscustomobject]@{
        path = $Path
        cwd = if ($meta -and $meta.cwd) { $meta.cwd } else { $null }
        session_id = if ($meta -and $meta.id) { $meta.id } else { $null }
        user_messages = @($userMessages)
        assistant_messages = @($assistantMessages | Select-Object -Last 6)
        commands = @($commands)
        mentioned_paths = @($mentionedPaths | Select-Object -Unique)
    }
}

$codexSessions = @()
$sessionIndex = Join-Path $codexHome 'session_index.jsonl'
if (Test-Path -LiteralPath $sessionIndex) {
    Get-Content -LiteralPath $sessionIndex | ForEach-Object {
        $j = Read-JsonLine $_
        if ($null -ne $j -and $j.updated_at) {
            try {
                $updated = ([DateTimeOffset]::Parse($j.updated_at)).ToLocalTime().DateTime
                if (Test-InRange $updated) {
                    $codexSessions += [pscustomobject]@{
                        source = 'codex_index'
                        thread_name = $j.thread_name
                        id = $j.id
                        updated_at = $updated.ToString('yyyy-MM-dd HH:mm:ss')
                    }
                }
            } catch {}
        }
    }
}

$codexSessionFiles = @()
$dateParts = $Date -split '-'
$codexDayDir = Join-Path $codexHome ("sessions\{0}\{1}\{2}" -f $dateParts[0], $dateParts[1], $dateParts[2])
if (Test-Path -LiteralPath $codexDayDir) {
    $codexSessionFiles += @(Get-ChildItem -LiteralPath $codexDayDir -Filter '*.jsonl' -File)
}
$codexArchiveDir = Join-Path $codexHome 'archived_sessions'
if (Test-Path -LiteralPath $codexArchiveDir) {
    $codexSessionFiles += @(Get-ChildItem -LiteralPath $codexArchiveDir -Filter '*.jsonl' -File |
        Where-Object { Test-InRange $_.LastWriteTime })
}

foreach ($file in ($codexSessionFiles | Sort-Object FullName -Unique)) {
    $summary = Read-CodexSessionSummary -Path $file.FullName
    $codexSessions += [pscustomobject]@{
        source = if ($file.FullName -like '*\archived_sessions\*') { 'codex_archived_session_file' } else { 'codex_session_file' }
        thread_name = $null
        id = if ($summary.session_id) { $summary.session_id } else { $file.BaseName }
        path = $file.FullName
        cwd = $summary.cwd
        updated_at = $file.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss')
        user_messages = $summary.user_messages
        assistant_messages = $summary.assistant_messages
        commands = $summary.commands
        mentioned_paths = $summary.mentioned_paths
    }
}

$claudeSessions = @()
$claudeProjects = Join-Path $claudeHome 'projects'
if (Test-Path -LiteralPath $claudeProjects) {
    Get-ChildItem -LiteralPath $claudeProjects -Recurse -Filter '*.jsonl' -File -ErrorAction SilentlyContinue |
        Where-Object { Test-InRange $_.LastWriteTime } |
        Select-Object -First 200 |
        ForEach-Object {
            $sample = Get-Content -LiteralPath $_.FullName -TotalCount 12 -ErrorAction SilentlyContinue
            $cwd = $null
            $branch = $null
            foreach ($line in $sample) {
                $j = Read-JsonLine $line
                if ($null -ne $j) {
                    if (-not $cwd -and $j.cwd) { $cwd = $j.cwd }
                    if (-not $branch -and $j.gitBranch) { $branch = $j.gitBranch }
                }
            }
            $claudeSessions += [pscustomobject]@{
                source = 'claude_project'
                path = $_.FullName
                cwd = $cwd
                git_branch = $branch
                updated_at = $_.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss')
            }
        }
}

$projectCandidates = @()
foreach ($root in $ProjectRoots) {
    if (-not (Test-Path -LiteralPath $root)) { continue }
    Get-ChildItem -LiteralPath $root -Directory -ErrorAction SilentlyContinue | ForEach-Object {
        $isImportant = $ImportantProjects -contains $_.Name
        $recentProject = Test-InRange $_.LastWriteTime
        $recentFiles = @()
        if ($recentProject -or $isImportant) {
            try {
                $recentFiles = @(Get-ChildItem -LiteralPath $_.FullName -Recurse -File -ErrorAction SilentlyContinue |
                    Where-Object {
                        (Test-InRange $_.LastWriteTime) -and
                        ($_.FullName -notmatch '\\node_modules\\|\\.git\\|\\dist\\cache\\|\\__pycache__\\')
                    } |
                    Sort-Object LastWriteTime -Descending |
                    Select-Object -First $MaxFilesPerProject |
                    ForEach-Object {
                        [pscustomobject]@{
                            path = $_.FullName
                            last_write_time = $_.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss')
                        }
                    })
            } catch {}
        }

        $status = if ($recentFiles.Count -gt 0) {
            'has_today_files'
        } elseif ($recentProject) {
            'project_timestamp_only'
        } elseif ($isImportant) {
            'important_project_no_today_evidence'
        } else {
            'not_recent'
        }

        if ($recentProject -or $isImportant -or $recentFiles.Count -gt 0) {
            $projectCandidates += [pscustomobject]@{
                name = $_.Name
                path = $_.FullName
                last_write_time = $_.LastWriteTime.ToString('yyyy-MM-dd HH:mm:ss')
                status = $status
                recent_files = $recentFiles
            }
        }
    }
}

$evidence = [ordered]@{
    date = $Date
    start = $Start
    end = $End
    generated_at = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss zzz')
    codex_home = $codexHome
    claude_home = $claudeHome
    codex_sessions = $codexSessions
    claude_sessions = $claudeSessions
    project_candidates = $projectCandidates
}

$jsonPath = Join-Path $OutDir "agent_evidence_$Date.json"
$mdPath = Join-Path $OutDir "agent_session_evidence_$Date.md"
$evidence | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $jsonPath -Encoding UTF8

$lines = New-Object System.Collections.Generic.List[string]
$tick = [char]96
$lines.Add("## 本地 Agent 证据摘要（$Date）")
$lines.Add("")
$lines.Add("### 候选本地项目")
$lines.Add("")
$lines.Add("| 项目 | 路径 | 状态 | 当天文件数 | 说明 |")
$lines.Add("| --- | --- | --- | ---: | --- |")
foreach ($p in $projectCandidates) {
    $reason = switch ($p.status) {
        'has_today_files' { '有当天修改文件，必须由 Agent 判断是否纳入日报。' }
        'project_timestamp_only' { '目录时间有变化，但未定位到可用文件证据，默认待确认。' }
        'important_project_no_today_evidence' { '历史重点项目，本次未发现当天文件证据，默认不纳入。' }
        default { '无当天证据。' }
    }
    $pathCell = "$tick$($p.path)$tick"
    $lines.Add("| $($p.name) | $pathCell | $($p.status) | $($p.recent_files.Count) | $reason |")
}
if ($projectCandidates.Count -eq 0) {
    $lines.Add("| 无 | - | no_candidates | 0 | 未发现当天本地项目证据。 |")
}
$lines.Add("")
$lines.Add("### 纳入日报的本地工作包")
$lines.Add("")
$lines.Add("> 由 Agent 根据候选项目、会话摘要和实际产物判断。脚本只提供证据，不直接写最终结论。")
$lines.Add("")
$lines.Add("### 未纳入原因")
$lines.Add("")
$lines.Add("| 项目/会话 | 原因 |")
$lines.Add("| --- | --- |")
foreach ($p in $projectCandidates | Where-Object { $_.status -eq 'important_project_no_today_evidence' }) {
    $lines.Add("| $($p.name) | 历史重点项目，但本次未发现当天文件证据；除非会话或用户补充证明当天有产出，否则不纳入。 |")
}
$lines.Add("")
$lines.Add("### Codex 会话候选")
$lines.Add("")
$lines.Add("| 来源 | 线程/文件 | 更新时间 |")
$lines.Add("| --- | --- | --- |")
foreach ($s in $codexSessions | Select-Object -First 80) {
    $name = if ($s.thread_name) { $s.thread_name } elseif ($s.path) { $s.path } else { $s.id }
    $nameCell = "$tick$name$tick"
    $lines.Add("| $($s.source) | $nameCell | $($s.updated_at) |")
}
if ($codexSessions.Count -eq 0) { $lines.Add("| 无 | - | - |") }
$lines.Add("")
$lines.Add("### Codex 会话摘要")
$lines.Add("")
foreach ($s in $codexSessions | Where-Object { $_.PSObject.Properties['path'] -and $_.path } | Select-Object -First 40) {
    $threadName = if ($s.PSObject.Properties['thread_name']) { $s.thread_name } else { $null }
    $cwdValue = if ($s.PSObject.Properties['cwd']) { $s.cwd } else { $null }
    $title = if ($threadName) { $threadName } elseif ($cwdValue) { $cwdValue } else { $s.path }
    $lines.Add("#### $title")
    $lines.Add("")
    $lines.Add("- 会话文件：$tick$($s.path)$tick")
    if ($cwdValue) { $lines.Add("- 工作目录：$tick$cwdValue$tick") }
    if ($s.PSObject.Properties['user_messages'] -and $s.user_messages -and $s.user_messages.Count -gt 0) {
        $lines.Add("- 用户请求：$((@($s.user_messages) | Select-Object -First 3) -join ' / ')")
    }
    if ($s.PSObject.Properties['assistant_messages'] -and $s.assistant_messages -and $s.assistant_messages.Count -gt 0) {
        $lines.Add("- 处理摘要：$((@($s.assistant_messages) | Select-Object -Last 3) -join ' / ')")
    }
    if ($s.PSObject.Properties['mentioned_paths'] -and $s.mentioned_paths -and $s.mentioned_paths.Count -gt 0) {
        $lines.Add("- 提到路径：$((@($s.mentioned_paths) | Select-Object -First 8) -join '；')")
    }
    $lines.Add("")
}
$lines.Add("")
$lines.Add("### Claude Code 会话候选")
$lines.Add("")
$lines.Add("| 路径 | cwd | 分支 | 更新时间 |")
$lines.Add("| --- | --- | --- | --- |")
foreach ($s in $claudeSessions | Select-Object -First 80) {
    $pathCell = "$tick$($s.path)$tick"
    $cwdCell = "$tick$($s.cwd)$tick"
    $branchCell = "$tick$($s.git_branch)$tick"
    $lines.Add("| $pathCell | $cwdCell | $branchCell | $($s.updated_at) |")
}
if ($claudeSessions.Count -eq 0) { $lines.Add("| 无 | - | - | - |") }

Set-Content -LiteralPath $mdPath -Value ($lines -join "`r`n") -Encoding UTF8
Write-Output $jsonPath
Write-Output $mdPath
