param(
    [Parameter(Mandatory = $true)]
    [string] $Date,

    [Parameter(Mandatory = $true)]
    [string] $Start,

    [Parameter(Mandatory = $true)]
    [string] $End,

    [Parameter(Mandatory = $true)]
    [string] $OutDir,

    [int] $RequestTimeoutSeconds = 120,

    [int] $ImRequestTimeoutSeconds = 45,

    [int] $ImChunkHours = 1,

    [int] $ImMinChunkMinutes = 60,

    [int] $ImMaxFailuresPerSearch = 4,

    [switch] $DryRun
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'lark_common.ps1')

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$script:imFailureCounts = @{}

function Test-AuthError {
    param([AllowNull()][string] $Message)
    return ($Message -match 'not logged in|auth login|ok=false \(auth\)')
}

$manifest = [ordered]@{
    date = $Date
    start = $Start
    end = $End
    generated_at = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss zzz')
    dry_run = [bool]$DryRun
    files = [ordered]@{}
    counts = [ordered]@{}
    errors = @()
}

$script:meetingIdSet = New-Object 'System.Collections.Generic.HashSet[string]'
$script:minuteTokenSet = New-Object 'System.Collections.Generic.HashSet[string]'

function Add-ErrorRecord {
    param([string] $Source, [string] $Message)
    $script:manifest.errors += [pscustomobject]@{ source = $Source; message = $Message }
}

function Save-Manifest {
    $manifestPath = Join-Path $OutDir 'source_manifest.json'
    $manifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
    Write-Output $manifestPath
}

function Format-LarkDateTime {
    param([Parameter(Mandatory = $true)][DateTimeOffset] $Value)
    return $Value.ToString('yyyy-MM-ddTHH:mm:sszzz')
}

function Add-ImFailure {
    param(
        [Parameter(Mandatory = $true)][string] $Prefix,
        [Parameter(Mandatory = $true)][string] $Message
    )
    if (-not $script:imFailureCounts.ContainsKey($Prefix)) {
        $script:imFailureCounts[$Prefix] = 0
    }
    $script:imFailureCounts[$Prefix]++
    Add-ErrorRecord $Prefix $Message
}

function Test-ImFailureLimitReached {
    param([Parameter(Mandatory = $true)][string] $Prefix)
    return ($script:imFailureCounts.ContainsKey($Prefix) -and $script:imFailureCounts[$Prefix] -ge $ImMaxFailuresPerSearch)
}

function Get-MeetingIdFromVcItem {
    param($Item)
    if ($null -eq $Item) { return $null }
    if ($Item.PSObject.Properties['id'] -and $Item.id) { return [string]$Item.id }
    return $null
}

function Get-MinuteTokenFromUrl {
    param([AllowNull()][string] $Url)
    if (-not $Url) { return $null }
    $match = [regex]::Match($Url, '/minutes/([^/?#]+)')
    if ($match.Success) { return $match.Groups[1].Value }
    return $null
}

if ($DryRun) {
    $manifest.plan = @(
        'calendar +agenda',
        'vc +search with pagination',
        'contact +get-user',
        'im +messages-search all with pagination',
        'im +messages-search sender=current_user with pagination',
        'docs +search with pagination',
        'redact raw json files',
        'write source_manifest.json'
    )
    Save-Manifest
    exit 0
}

try {
    $file = Join-Path $OutDir "calendar_agenda_$Date.json"
    Invoke-LarkCliJson -Args @('calendar', '+agenda', '--as', 'user', '--start', $Start, '--end', $End, '--format', 'json') -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
    $manifest.files.calendar = $file
    $j = Read-JsonFileOrNull -Path $file
    $manifest.counts.calendar = Count-JsonArray $j.data
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'calendar' $_.Exception.Message
}
Save-Manifest | Out-Null

try {
    $page = 1
    $vcFiles = @()
    $pageToken = $null
    $hasMore = $true
    while ($hasMore -and $page -le 20) {
        $file = Join-Path $OutDir "vc_search_${Date}_page$page.json"
        $args = @('vc', '+search', '--as', 'user', '--start', $Start, '--end', $End, '--format', 'json', '--page-size', '30')
        if ($pageToken) { $args += @('--page-token', $pageToken) }
        Invoke-LarkCliJson -Args $args -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
        $vcFiles += $file
        $j = Read-JsonFileOrNull -Path $file
        $hasMore = [bool]($j.data.has_more)
        $pageToken = $j.data.page_token
        $page++
    }
    $manifest.files.vc = $vcFiles
    $manifest.counts.vc = ($vcFiles | ForEach-Object {
        $j = Read-JsonFileOrNull -Path $_
        foreach ($item in @($j.data.items)) {
            $meetingId = Get-MeetingIdFromVcItem -Item $item
            if ($meetingId) { [void]$script:meetingIdSet.Add($meetingId) }
            $displayInfo = if ($item.PSObject.Properties['display_info']) { [string]$item.display_info } else { '' }
            foreach ($u in [regex]::Matches($displayInfo, 'https?://\S+')) {
                $minuteToken = Get-MinuteTokenFromUrl -Url $u.Value
                if ($minuteToken) { [void]$script:minuteTokenSet.Add($minuteToken) }
            }
        }
        Count-JsonArray $j.data.items
    } | Measure-Object -Sum).Sum
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'vc' $_.Exception.Message
}
Save-Manifest | Out-Null

try {
    $detailFiles = @()
    $index = 1
    foreach ($meetingId in $script:meetingIdSet) {
        $file = Join-Path $OutDir "vc_meeting_${Date}_detail_${index}.json"
        Invoke-LarkCliJson -Args @('vc', 'meeting', 'get', '--as', 'user', '--params', "{""meeting_id"":""$meetingId"",""with_participants"":true}") -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
        $detailFiles += $file
        $j = Read-JsonFileOrNull -Path $file
        $noteDocToken = $null
        $verbatimDocToken = $null
        $minuteToken = $null
        if ($j -and $j.data) {
            if ($j.data.PSObject.Properties['note_doc_token']) { $noteDocToken = $j.data.note_doc_token }
            if ($j.data.PSObject.Properties['verbatim_doc_token']) { $verbatimDocToken = $j.data.verbatim_doc_token }
            if ($j.data.PSObject.Properties['minute_token']) { $minuteToken = $j.data.minute_token }
            if (-not $minuteToken -and $j.data.PSObject.Properties['url']) { $minuteToken = Get-MinuteTokenFromUrl -Url ([string]$j.data.url) }
            if (-not $minuteToken -and $j.data.PSObject.Properties['meeting_url']) { $minuteToken = Get-MinuteTokenFromUrl -Url ([string]$j.data.meeting_url) }
        }
        if ($minuteToken) { [void]$script:minuteTokenSet.Add([string]$minuteToken) }
        $index++
    }
    if ($detailFiles.Count -gt 0) {
        $manifest.files.vc_meeting_details = $detailFiles
        $manifest.counts.vc_meeting_details = $detailFiles.Count
    }
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'vc_meeting_details' $_.Exception.Message
}
Save-Manifest | Out-Null

try {
    $notesFiles = @()
    if ($script:meetingIdSet.Count -gt 0) {
        $chunks = @($script:meetingIdSet) | ForEach-Object -Begin { $bucket = @() } -Process {
            $bucket += $_
            if ($bucket.Count -ge 50) {
                ,$bucket
                $bucket = @()
            }
        } -End {
            if ($bucket.Count -gt 0) { ,$bucket }
        }

        $index = 1
        foreach ($chunk in $chunks) {
            $file = Join-Path $OutDir "vc_notes_${Date}_meeting_chunk${index}.json"
            Invoke-LarkCliJson -Args @('vc', '+notes', '--as', 'user', '--meeting-ids', ($chunk -join ','), '--format', 'json') -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
            $notesFiles += $file
            $index++
        }
    }
    if ($notesFiles.Count -gt 0) {
        $manifest.files.vc_notes = $notesFiles
        $manifest.counts.vc_notes = $notesFiles.Count
    }
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'vc_notes' $_.Exception.Message
}
Save-Manifest | Out-Null

try {
    $minuteFiles = @()
    if ($script:minuteTokenSet.Count -gt 0) {
        $chunks = @($script:minuteTokenSet) | ForEach-Object -Begin { $bucket = @() } -Process {
            $bucket += $_
            if ($bucket.Count -ge 50) {
                ,$bucket
                $bucket = @()
            }
        } -End {
            if ($bucket.Count -gt 0) { ,$bucket }
        }

        $index = 1
        foreach ($chunk in $chunks) {
            $file = Join-Path $OutDir "vc_notes_${Date}_minute_chunk${index}.json"
            Invoke-LarkCliJson -Args @('vc', '+notes', '--as', 'user', '--minute-tokens', ($chunk -join ','), '--format', 'json') -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
            $minuteFiles += $file
            $index++
        }
    }
    if ($minuteFiles.Count -gt 0) {
        $manifest.files.vc_minutes = $minuteFiles
        $manifest.counts.vc_minutes = $minuteFiles.Count
    }
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'vc_minutes' $_.Exception.Message
}
Save-Manifest | Out-Null

$currentUserOpenId = $null
try {
    $file = Join-Path $OutDir "current_user_$Date.json"
    Invoke-LarkCliJson -Args @('contact', '+get-user', '--as', 'user', '--format', 'json') -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
    $manifest.files.current_user = $file
    $j = Read-JsonFileOrNull -Path $file
    $currentUserOpenId = $j.data.user.open_id
    $manifest.current_user_name = $j.data.user.name
    $manifest.current_user_open_id = $currentUserOpenId
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'current_user' $_.Exception.Message
}
Save-Manifest | Out-Null

function Collect-ImRangePages {
    param(
        [Parameter(Mandatory = $true)][string] $Prefix,
        [string] $Sender,
        [Parameter(Mandatory = $true)][DateTimeOffset] $RangeStart,
        [Parameter(Mandatory = $true)][DateTimeOffset] $RangeEnd,
        [Parameter(Mandatory = $true)][string] $ChunkLabel
    )

    $page = 1
    $files = @()
    $pageToken = $null
    $hasMore = $true
    while ($hasMore -and $page -le 50) {
        if (Test-ImFailureLimitReached -Prefix $Prefix) { return $files }

        $file = Join-Path $OutDir "${Prefix}_${ChunkLabel}_page$page.json"
        $args = @(
            'im', '+messages-search',
            '--as', 'user',
            '--start', (Format-LarkDateTime $RangeStart),
            '--end', (Format-LarkDateTime $RangeEnd),
            '--page-size', '50',
            '--format', 'json'
        )
        if ($Sender) { $args += @('--sender', $Sender) }
        if ($pageToken) { $args += @('--page-token', $pageToken) }

        try {
            Invoke-LarkCliJson -Args $args -OutFile $file -TimeoutSeconds $ImRequestTimeoutSeconds | Out-Null
            $files += $file
            $j = Read-JsonFileOrNull -Path $file
            $hasMore = [bool]($j.data.has_more)
            $pageToken = $j.data.page_token
            $page++
        } catch {
            if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
            $minutes = ($RangeEnd - $RangeStart).TotalMinutes
            if ($page -eq 1 -and $minutes -gt $ImMinChunkMinutes) {
                $mid = $RangeStart.AddMinutes($minutes / 2)
                $leftLabel = "${ChunkLabel}a"
                $rightLabel = "${ChunkLabel}b"
                $left = Collect-ImRangePages -Prefix $Prefix -Sender $Sender -RangeStart $RangeStart -RangeEnd $mid -ChunkLabel $leftLabel
                $right = Collect-ImRangePages -Prefix $Prefix -Sender $Sender -RangeStart $mid -RangeEnd $RangeEnd -ChunkLabel $rightLabel
                return @($left) + @($right)
            }

            Add-ImFailure -Prefix $Prefix -Message "IM search failed for $ChunkLabel page $page ($((Format-LarkDateTime $RangeStart)) ~ $((Format-LarkDateTime $RangeEnd))): $($_.Exception.Message)"
            return $files
        }
    }
    return $files
}

function Collect-ImPages {
    param(
        [Parameter(Mandatory = $true)][string] $Prefix,
        [string] $Sender
    )

    $rangeStart = [DateTimeOffset]::Parse($Start)
    $rangeEnd = [DateTimeOffset]::Parse($End)
    $cursor = $rangeStart
    $chunkIndex = 1
    $files = @()
    $chunkHours = [Math]::Max(1, $ImChunkHours)

    while ($cursor -lt $rangeEnd) {
        if (Test-ImFailureLimitReached -Prefix $Prefix) {
            Add-ErrorRecord $Prefix "Stopped IM search after $ImMaxFailuresPerSearch failures; remaining time range was skipped."
            break
        }

        $chunkEnd = $cursor.AddHours($chunkHours)
        if ($chunkEnd -gt $rangeEnd) { $chunkEnd = $rangeEnd }
        $label = ('chunk{0:D2}' -f $chunkIndex)
        $files += @(Collect-ImRangePages -Prefix $Prefix -Sender $Sender -RangeStart $cursor -RangeEnd $chunkEnd -ChunkLabel $label)
        Save-Manifest | Out-Null
        $cursor = $chunkEnd
        $chunkIndex++
    }

    return $files
}

if ($currentUserOpenId) {
    try {
        $files = Collect-ImPages -Prefix "im_messages_self_$Date" -Sender $currentUserOpenId
        $manifest.files.im_self = $files
        $detailsMissing = $false
        $manifest.counts.im_self = ($files | ForEach-Object {
            $j = Read-JsonFileOrNull -Path $_
            if (Test-LarkImDetailsMissing $j) { $detailsMissing = $true }
            Count-JsonArray (Get-LarkImItems $j)
        } | Measure-Object -Sum).Sum
        if ($detailsMissing) {
            Add-ErrorRecord 'im_self_details' 'IM search returned message IDs only; message content enrichment is unavailable. Confirm user scopes include im:message:readonly (or im:message) plus im:message.group_msg:get_as_user and im:message.p2p_msg:get_as_user.'
        }
    } catch {
        if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
        Add-ErrorRecord 'im_self' $_.Exception.Message
    }
} else {
    Add-ErrorRecord 'im_self' 'Skipped because current user open_id was unavailable.'
}
Save-Manifest | Out-Null

try {
    $files = Collect-ImPages -Prefix "im_messages_all_$Date"
    $manifest.files.im_all = $files
    $detailsMissing = $false
    $manifest.counts.im_all = ($files | ForEach-Object {
        $j = Read-JsonFileOrNull -Path $_
        if (Test-LarkImDetailsMissing $j) { $detailsMissing = $true }
        Count-JsonArray (Get-LarkImItems $j)
    } | Measure-Object -Sum).Sum
    if ($detailsMissing) {
        Add-ErrorRecord 'im_all_details' 'IM search returned message IDs only; message content enrichment is unavailable. Confirm user scopes include im:message:readonly (or im:message) plus im:message.group_msg:get_as_user and im:message.p2p_msg:get_as_user.'
    }
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'im_all' $_.Exception.Message
}
Save-Manifest | Out-Null

try {
    $page = 1
    $docFiles = @()
    $pageToken = $null
    $hasMore = $true
    $filter = @{ open_time = @{ start = $Start; end = $End } } | ConvertTo-Json -Compress
    while ($hasMore -and $page -le 10) {
        $file = Join-Path $OutDir "docs_search_${Date}_page$page.json"
        $args = @('docs', '+search', '--as', 'user', '--query', '', '--filter', $filter, '--page-size', '20', '--format', 'json')
        if ($pageToken) { $args += @('--page-token', $pageToken) }
        Invoke-LarkCliJson -Args $args -OutFile $file -TimeoutSeconds $RequestTimeoutSeconds | Out-Null
        $docFiles += $file
        $j = Read-JsonFileOrNull -Path $file
        $hasMore = [bool]($j.data.has_more)
        $pageToken = $j.data.page_token
        $page++
    }
    $manifest.files.docs = $docFiles
    $manifest.counts.docs = ($docFiles | ForEach-Object {
        $j = Read-JsonFileOrNull -Path $_
        Count-JsonArray $j.data.results
    } | Measure-Object -Sum).Sum
} catch {
    if (Test-AuthError $_.Exception.Message) { Save-Manifest | Out-Null; throw }
    Add-ErrorRecord 'docs' $_.Exception.Message
}
Save-Manifest | Out-Null

Protect-JsonFiles -Directory $OutDir
Save-Manifest
