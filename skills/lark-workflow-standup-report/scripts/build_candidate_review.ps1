param(
    [Parameter(Mandatory = $true)]
    [string] $Date,

    [Parameter(Mandatory = $true)]
    [string] $SourceManifestPath,

    [Parameter(Mandatory = $true)]
    [string] $AgentEvidenceJsonPath,

    [Parameter(Mandatory = $true)]
    [string] $OutFile
)

Set-StrictMode -Version 3.0
$ErrorActionPreference = 'Stop'

. (Join-Path $PSScriptRoot 'lark_common.ps1')

function Get-ChatKey {
    param($Message)
    $chatName = $Message.PSObject.Properties['chat_name']
    if ($chatName -and $chatName.Value) { return $chatName.Value }
    $chatType = $Message.PSObject.Properties['chat_type']
    if ($chatType -and $chatType.Value) { return $chatType.Value }
    $chatId = $Message.PSObject.Properties['chat_id']
    if ($chatId -and $chatId.Value) { return $chatId.Value }
    return 'unknown_chat'
}

function Get-ChatLabel {
    param(
        $Message,
        [hashtable] $P2pPartnerNames
    )

    $chatName = $Message.PSObject.Properties['chat_name']
    if ($chatName -and $chatName.Value) { return [string]$chatName.Value }

    $chatType = if ($Message.PSObject.Properties['chat_type']) { [string]$Message.chat_type } else { $null }
    $chatId = if ($Message.PSObject.Properties['chat_id']) { [string]$Message.chat_id } else { $null }
    if ($chatType -eq 'p2p') {
        $partnerName = $null
        if ($chatId -and $P2pPartnerNames.ContainsKey($chatId)) { $partnerName = $P2pPartnerNames[$chatId] }
        if ($partnerName) { return "与${partnerName}私聊" }
        return '未命名私聊联系人'
    }
    if ($chatType) { return $chatType }
    if ($chatId) { return $chatId }
    return 'unknown_chat'
}

$manifest = Get-Content -LiteralPath $SourceManifestPath -Raw | ConvertFrom-Json
$agent = Get-Content -LiteralPath $AgentEvidenceJsonPath -Raw | ConvertFrom-Json
$sourceDir = Split-Path -Parent $SourceManifestPath
$rows = New-Object System.Collections.Generic.List[object]

function Get-NestedValue {
    param(
        $Object,
        [Parameter(Mandatory = $true)][string[]] $Path,
        $Default = $null
    )

    $current = $Object
    foreach ($part in $Path) {
        if ($null -eq $current) { return $Default }
        $prop = $current.PSObject.Properties[$part]
        if ($null -eq $prop) { return $Default }
        $current = $prop.Value
    }
    if ($null -eq $current) { return $Default }
    return $current
}

$selfFilesValue = Get-NestedValue -Object $manifest -Path @('files', 'im_self') -Default @()
if ($selfFilesValue -is [array]) { $selfFiles = $selfFilesValue } else { $selfFiles = @($selfFilesValue) }

$allFilesValue = Get-NestedValue -Object $manifest -Path @('files', 'im_all') -Default @()
if ($allFilesValue -is [array]) { $allFiles = $allFilesValue } else { $allFiles = @($allFilesValue) }

$allMessages = @()
foreach ($file in $allFiles) {
    $j = Read-JsonFileOrNull -Path $file
    if ($j -and $j.PSObject.Properties['data'] -and $j.data.PSObject.Properties['messages'] -and $j.data.messages) {
        $allMessages += $j.data.messages
    }
}

$currentUserName = Get-NestedValue -Object $manifest -Path @('current_user_name') -Default $null
$p2pPartnerNames = @{}
foreach ($message in $allMessages) {
    $chatType = if ($message.PSObject.Properties['chat_type']) { [string]$message.chat_type } else { $null }
    $chatId = if ($message.PSObject.Properties['chat_id']) { [string]$message.chat_id } else { $null }
    if ($chatType -ne 'p2p' -or -not $chatId) { continue }

    $senderName = $null
    if ($message.PSObject.Properties['sender'] -and $message.sender -and $message.sender.PSObject.Properties['name']) {
        $senderName = [string]$message.sender.name
    }
    if ($senderName -and $currentUserName -and $senderName -ne $currentUserName) {
        $p2pPartnerNames[$chatId] = $senderName
        continue
    }

    if ($message.PSObject.Properties['chat_partner'] -and $message.chat_partner -and $message.chat_partner.PSObject.Properties['name']) {
        $partnerName = [string]$message.chat_partner.name
        if ($partnerName) { $p2pPartnerNames[$chatId] = $partnerName }
    }
}

$selfMessages = @()
foreach ($file in $selfFiles) {
    $j = Read-JsonFileOrNull -Path $file
    if ($j -and $j.PSObject.Properties['data'] -and $j.data.PSObject.Properties['messages'] -and $j.data.messages) {
        $selfMessages += $j.data.messages
    }
}

$selfMessages |
    Where-Object {
        $msgType = if ($_.PSObject.Properties['msg_type']) { $_.msg_type } else { $null }
        $content = if ($_.PSObject.Properties['content']) { [string]$_.content } else { $null }
        $msgType -ne 'image' -and $content -and $content.Trim().Length -gt 0
    } |
    Group-Object { Get-ChatLabel -Message $_ -P2pPartnerNames $p2pPartnerNames } |
    Sort-Object Count -Descending |
    Select-Object -First 30 |
    ForEach-Object {
        $sample = ($_.Group | Select-Object -First 3 | ForEach-Object {
            $content = if ($_.PSObject.Properties['content']) { [string]$_.content } else { '' }
            $text = Protect-Text -Text $content
            if ($text.Length -gt 80) { $text.Substring(0, 80) + '...' } else { $text }
        }) -join ' / '
        $rows.Add([pscustomobject]@{
            item = $_.Name
            source = '飞书本人消息'
            evidence = "本人发言 $($_.Count) 条；样例：$sample"
            recommendation = '待纳入判断'
            reason = '满足本人相关的最低证据，但仍需判断是否为工作事项、是否有产出或后续责任。'
        })
    }

$vcDetailFilesValue = Get-NestedValue -Object $manifest -Path @('files', 'vc_meeting_details') -Default @()
if ($vcDetailFilesValue -is [array]) { $vcDetailFiles = $vcDetailFilesValue } else { $vcDetailFiles = @($vcDetailFilesValue) }
foreach ($file in $vcDetailFiles) {
    $j = Read-JsonFileOrNull -Path $file
    if (-not $j -or -not $j.data) { continue }
    $topic = $null
    foreach ($key in @('topic', 'title', 'meeting_topic', 'name')) {
        if ($j.data.PSObject.Properties[$key] -and $j.data.$key) {
            $topic = [string]$j.data.$key
            break
        }
    }
    $organizer = $null
    foreach ($key in @('organizer_name', 'owner_name', 'host_name')) {
        if ($j.data.PSObject.Properties[$key] -and $j.data.$key) {
            $organizer = [string]$j.data.$key
            break
        }
    }
    $timeText = $null
    foreach ($key in @('start_time', 'start_time_iso', 'meeting_start_time')) {
        if ($j.data.PSObject.Properties[$key] -and $j.data.$key) {
            $timeText = [string]$j.data.$key
            break
        }
    }
    $itemName = if ($topic) { $topic } else { '飞书会议' }
    $evidence = @()
    if ($organizer) { $evidence += "组织者：$organizer" }
    if ($timeText) { $evidence += "时间：$timeText" }
    if ($j.data.PSObject.Properties['meeting_id'] -and $j.data.meeting_id) { $evidence += "meeting_id 已采集" }
    $rows.Add([pscustomobject]@{
        item = $itemName
        source = '飞书会议'
        evidence = ($evidence -join '；')
        recommendation = '待纳入判断'
        reason = '会议是高价值证据源；应优先结合会议纪要、妙记和相关文档判断是否纳入。'
    })
}

$docFilesValue = Get-NestedValue -Object $manifest -Path @('files', 'docs') -Default @()
if ($docFilesValue -is [array]) { $docFiles = $docFilesValue } else { $docFiles = @($docFilesValue) }
foreach ($file in $docFiles) {
    $j = Read-JsonFileOrNull -Path $file
    foreach ($result in @($j.data.results) | Select-Object -First 20) {
        $meta = if ($result.PSObject.Properties['result_meta']) { $result.result_meta } else { $null }
        $title = if ($result.PSObject.Properties['title_highlighted'] -and $result.title_highlighted) { [string]$result.title_highlighted } else { $null }
        if (-not $title -and $meta -and $meta.PSObject.Properties['title']) { $title = [string]$meta.title }
        if (-not $title) { $title = '飞书文档' }
        $owner = if ($meta -and $meta.PSObject.Properties['owner_name']) { [string]$meta.owner_name } else { $null }
        $lastOpen = if ($meta -and $meta.PSObject.Properties['last_open_time_iso']) { [string]$meta.last_open_time_iso } else { $null }
        $docType = if ($result.PSObject.Properties['entity_type']) { [string]$result.entity_type } else { $null }
        $evidenceParts = @()
        if ($docType) { $evidenceParts += "类型：$docType" }
        if ($owner) { $evidenceParts += "所有者：$owner" }
        if ($lastOpen) { $evidenceParts += "最近打开：$lastOpen" }
        $rows.Add([pscustomobject]@{
            item = $title
            source = '飞书文档'
            evidence = ($evidenceParts -join '；')
            recommendation = '待纳入判断'
            reason = '文档打开或编辑本身不是结论，但若标题和时间与当天主线一致，应优先纳入候选审查。'
        })
    }
}

$projectCandidatesValue = Get-NestedValue -Object $agent -Path @('project_candidates') -Default @()
foreach ($p in @($projectCandidatesValue)) {
    $recommendation = if ($p.status -eq 'has_today_files') { '待纳入判断' } else { '默认不纳入' }
    $reason = switch ($p.status) {
        'has_today_files' { '有当天本地文件证据；需结合会话与产物判断是否为正式工作包。' }
        'project_timestamp_only' { '仅目录时间变化，缺少产物证据。' }
        'important_project_no_today_evidence' { '历史重点项目，但无当天证据。' }
        default { '缺少当天证据。' }
    }
    $rows.Add([pscustomobject]@{
        item = $p.name
        source = '本地项目'
        evidence = "$($p.path)；状态：$($p.status)；当天文件数：$($p.recent_files.Count)"
        recommendation = $recommendation
        reason = $reason
    })
}

$codexSessionsValue = Get-NestedValue -Object $agent -Path @('codex_sessions') -Default @()
foreach ($s in @($codexSessionsValue) | Select-Object -First 40) {
    $name = if ($s.thread_name) { $s.thread_name } elseif ($s.path) { $s.path } else { $s.id }
    $rows.Add([pscustomobject]@{
        item = $name
        source = 'Codex 会话'
        evidence = "更新时间：$($s.updated_at)"
        recommendation = '待纳入判断'
        reason = '需要读取会话摘要和产物；不能只因会话存在就写入日报。'
    })
}

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("## 日报候选事项审查（$Date）")
$lines.Add("")
$lines.Add("### 数据覆盖")
$lines.Add("")
$lines.Add("- 日历：$(Get-NestedValue -Object $manifest -Path @('counts', 'calendar') -Default 0)")
$lines.Add("- 视频会议：$(Get-NestedValue -Object $manifest -Path @('counts', 'vc') -Default 0)")
$lines.Add("- 群聊全量：$(Get-NestedValue -Object $manifest -Path @('counts', 'im_all') -Default 0)")
$lines.Add("- 本人发言：$(Get-NestedValue -Object $manifest -Path @('counts', 'im_self') -Default 0)")
$lines.Add("- 云文档：$(Get-NestedValue -Object $manifest -Path @('counts', 'docs') -Default 0)")
$errorsValue = Get-NestedValue -Object $manifest -Path @('errors') -Default @()
$errorCount = @($errorsValue).Count
if ($errorCount -gt 0) {
    $lines.Add("- 采集错误：$errorCount 项，见 source_manifest.json")
}
$lines.Add("")
$lines.Add("### 纳入门槛")
$lines.Add("")
$lines.Add("- 纳入日报必须有本人主导、本人明确推进、本人实际产出或本人后续责任。")
$lines.Add("- 全量群聊只作为上下文；无本人相关证据时默认不纳入。")
$lines.Add("- 日报自动化普通采集/创建文档不作为业务工作项。")
$lines.Add("")
$lines.Add("### 候选审查表")
$lines.Add("")
$lines.Add("| 候选事项 | 来源类型 | 本人相关证据 | 建议 | 未纳入/待确认原因 |")
$lines.Add("| --- | --- | --- | --- | --- |")
foreach ($r in $rows) {
    $item = ([string]$r.item).Replace('|', '/')
    $evidence = ([string]$r.evidence).Replace('|', '/').Replace("`r", ' ').Replace("`n", ' ')
    $reason = ([string]$r.reason).Replace('|', '/').Replace("`r", ' ').Replace("`n", ' ')
    $lines.Add("| $item | $($r.source) | $evidence | $($r.recommendation) | $reason |")
}
if ($rows.Count -eq 0) {
    $lines.Add("| 无 | - | - | 默认不纳入 | 未发现候选事项。 |")
}

Set-Content -LiteralPath $OutFile -Value ($lines -join "`r`n") -Encoding UTF8
Write-Output $OutFile
