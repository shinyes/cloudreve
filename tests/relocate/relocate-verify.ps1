# Relocate verification: tree walk, zero residue, download integrity, encrypted blobs.
# ASCII only (Windows PowerShell 5.1 reads .ps1 as ANSI when there is no BOM).
param(
  [Parameter(Mandatory = $true)][string]$Root,
  [Parameter(Mandatory = $true)][int]$Port
)

$ErrorActionPreference = 'Stop'
$base = "http://127.0.0.1:$Port"
$pass = 0; $fail = 0

function Check($name, $cond, $detail) {
  if ($cond) { $script:pass++; "  PASS  $name" }
  else { $script:fail++; "  FAIL  $name  -> $detail" }
}

function Call($method, $path, $headers, $bodyObj) {
  $p = @{ Uri = "$base$path"; Method = $method; Headers = $headers; UseBasicParsing = $true; TimeoutSec = 60 }
  if ($null -ne $bodyObj) { $p.Body = ($bodyObj | ConvertTo-Json -Depth 12) }
  $r = Invoke-WebRequest @p
  return ($r.Content | ConvertFrom-Json)
}

function UploadFile($headers, $uri, $content, $policyID) {
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($content)
  $req = @{ uri=$uri; size=$bytes.Length; mime_type='text/plain' }
  if ($policyID) { $req.policy_id = $policyID }
  $sess = Call PUT '/api/v4/file/upload' $headers $req
  if (-not $sess.data.session_id) { throw "upload session failed for $uri : $($sess.msg)" }
  $url = "$base/api/v4/file/upload/$($sess.data.session_id)/0"
  Invoke-WebRequest -Uri $url -Method POST -Headers @{ Authorization = $headers.Authorization; 'Content-Type'='application/octet-stream' } -Body $bytes -UseBasicParsing -TimeoutSec 60 | Out-Null
  return $sess.data.storage_policy.name
}

function DownloadText($headers, $uri) {
  $u = Call POST '/api/v4/file/url' $headers @{ uris=@($uri) }
  if (-not $u.data.urls[0].url) { throw "no url for $uri : $($u.msg)" }
  # The API returns an absolute URL based on the configured site URL (which may be
  # localhost); keep only the path so the request goes to this test instance.
  $path = ([System.Uri]$u.data.urls[0].url).PathAndQuery
  return (Invoke-WebRequest "$base$path" -Headers $headers -UseBasicParsing -TimeoutSec 60).Content
}

function WaitTask($headers, $taskID) {
  $status = ''
  for ($i = 1; $i -le 60; $i++) {
    Start-Sleep -Seconds 2
    $list = Call GET '/api/v4/workflow?page_size=10&category=general' $headers $null
    $t = $list.data.tasks | Where-Object { $_.id -eq $taskID }
    if ($t) { $status = $t.status; if ($t.error) { $script:lastTaskError = $t.error } }
    if ($status -in @('completed', 'error', 'canceled')) { break }
  }
  return $status
}

$proc = Start-Process -FilePath (Join-Path $Root 'cloudreve.exe') -WorkingDirectory $Root `
  -RedirectStandardOutput (Join-Path $Root 'out.log') -RedirectStandardError (Join-Path $Root 'err.log') -PassThru

try {
  for ($i = 1; $i -le 40; $i++) {
    Start-Sleep -Seconds 3
    if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) { break }
  }
  Check 'server alive' (-not $proc.HasExited) 'process exited'

  Call POST '/api/v4/user' @{'Content-Type'='application/json'} @{ email='admin@example.com'; password='Passw0rd!23' } | Out-Null
  $login = Call POST '/api/v4/session/token' @{'Content-Type'='application/json'} @{ email='admin@example.com'; password='Passw0rd!23' }
  $AH = @{ Authorization = "Bearer $($login.data.token.access_token)"; 'Content-Type' = 'application/json' }

  # Policy B: plain. Policy C: encryption enabled, so blobs written there are ciphertext.
  Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Policy B'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400 } } } | Out-Null
  $c = Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Policy C'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400; encryption=$true } } }
  Check 'encrypted policy C created' (($c.code -eq 0) -and $c.data.settings.encryption) "code=$($c.code) enc=$($c.data.settings.encryption)"

  # Grant [1,2,3] without wiping the group's other settings.
  $group = (Call GET '/api/v4/admin/group/2' $AH $null).data
  $group.edges = @{ storage_policies = @( @{id=1}, @{id=2}, @{id=3} ) }
  Call PUT '/api/v4/admin/group/2' $AH @{ group=$group } | Out-Null
  $after = (Call GET '/api/v4/admin/group/2' $AH $null).data
  Check 'group granted 3 policies, settings intact' ($after.edges.storage_policies.Count -eq 3 -and $after.settings.max_walked_files -gt 0) "count=$($after.edges.storage_policies.Count) walk=$($after.settings.max_walked_files)"

  Call POST '/api/v4/user' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' } | Out-Null
  $ulogin = Call POST '/api/v4/session/token' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' }
  $UH = @{ Authorization = "Bearer $($ulogin.data.token.access_token)"; 'Content-Type' = 'application/json' }

  $up = Call GET '/api/v4/user/policies' $UH $null
  $policyB = ($up.data.policies | Where-Object { $_.name -eq 'Policy B' }).id
  $policyC = ($up.data.policies | Where-Object { $_.name -eq 'Policy C' }).id
  $policyDefault = ($up.data.policies | Where-Object { $_.name -eq 'Default storage policy' }).id
  Check 'user sees 3 policies' ($up.data.policies.Count -eq 3) "got $($up.data.policies.Count)"

  Call POST '/api/v4/file/create' $UH @{ uri='cloudreve://my/tree'; type='folder' } | Out-Null
  Call POST '/api/v4/file/create' $UH @{ uri='cloudreve://my/tree/sub'; type='folder' } | Out-Null

  # Plain files on the default policy.
  UploadFile $UH 'cloudreve://my/tree/a.txt' 'alpha' | Out-Null
  UploadFile $UH 'cloudreve://my/tree/sub/b.txt' 'bravo' | Out-Null
  UploadFile $UH 'cloudreve://my/tree/sub/c.txt' 'charlie' | Out-Null

  # A file explicitly placed on the encrypting policy.
  $encPolicy = UploadFile $UH 'cloudreve://my/tree/secret.txt' 'top-secret-payload' $policyC
  Check 'file uploaded onto encrypting policy' ($encPolicy -eq 'Policy C') "got '$encPolicy'"

  # A GENUINELY encrypted file, produced through the real client encryption path
  # (declare aes-256-ctr, encrypt with the returned key, upload ciphertext), so the
  # "encrypted -> non-encrypting policy" branch runs against real ciphertext.
  $encUpload = & node "$PSScriptRoot\upload-encrypted.mjs" $base $($ulogin.data.token.access_token) 'cloudreve://my/tree/enc.txt' 'encrypted-payload-xyz' $policyC 2>&1 | Out-String
  Check 'genuinely encrypted file uploaded' ($LASTEXITCODE -eq 0) "output: $encUpload"
  $encProps = & go run ./tools/relocate-check -db (Join-Path $Root 'cloudreve.db') -props 2>&1 | Out-String
  Check 'encrypted entity carries key material' ($encProps -match 'encrypt_metadata=present') "db: $encProps"
  $readableBefore = (Get-ChildItem (Join-Path $Root 'data\uploads') -Recurse -File | Where-Object { (Select-String -Path $_.FullName -Pattern 'encrypted-payload-xyz' -Quiet -ErrorAction SilentlyContinue) }).Count
  Check 'encrypted payload unreadable on disk before move' ($readableBefore -eq 0) "found $readableBefore"

  $db = Join-Path $Root 'cloudreve.db'
  $before = & go run ./tools/relocate-check -db $db 2>&1 | Out-String

  # --- relocate the whole tree to Policy B ---
  $task = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/tree'); dst_policy_id=$policyB }
  Check 'relocate task created' (($task.code -eq 0) -and $task.data.id) "code=$($task.code) msg=$($task.msg)"
  $status = WaitTask $UH $task.data.id
  Check 'relocate completed' ($status -eq 'completed') "status=$status err=$($script:lastTaskError)"

  $after = & go run ./tools/relocate-check -db $db -props 2>&1 | Out-String
  Check 'zero residue on other policies' ($after -match 'POLICY 2 => 5 entities' -and $after -notmatch 'POLICY 1 =>' -and $after -notmatch 'POLICY 3 =>') "db: $after"
  Check 'all entities dropped key material after decrypting move' (([regex]::Matches($after, 'encrypt_metadata=present')).Count -eq 0) "db: $after"

  # The file-level storage policy drives what the explorer and the admin panel report,
  # so it has to advance together with the entities. A stale value is what made a
  # relocated file still show its old policy in the admin file list.
  $filePolicies = & go run ./tools/relocate-check -db $db -file-policies 2>&1 | Out-String
  Check 'files report the target policy' ($filePolicies -notmatch 'policy=1') "file policies: $filePolicies"

  # --- content must survive the move, including the formerly encrypted file ---
  $a = DownloadText $UH 'cloudreve://my/tree/a.txt'
  Check 'plain file content intact' ($a -eq 'alpha') "got '$a'"
  $b = DownloadText $UH 'cloudreve://my/tree/sub/b.txt'
  Check 'nested file content intact' ($b -eq 'bravo') "got '$b'"
  $s = DownloadText $UH 'cloudreve://my/tree/secret.txt'
  Check 'encrypted blob still decryptable after move' ($s -eq 'top-secret-payload') "got '$s'"
  $e = DownloadText $UH 'cloudreve://my/tree/enc.txt'
  Check 'genuinely encrypted file readable after move' ($e -eq 'encrypted-payload-xyz') "got '$e'"
  $readableAfter = (Get-ChildItem (Join-Path $Root 'data\uploads') -Recurse -File | Where-Object { (Select-String -Path $_.FullName -Pattern 'encrypted-payload-xyz' -Quiet -ErrorAction SilentlyContinue) }).Count
  Check 'encrypted payload stored as plaintext after move' ($readableAfter -ge 1) "found $readableAfter"

  # --- relocating back must work too ---
  $task2 = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/tree/sub'); dst_policy_id=$policyDefault }
  $status2 = WaitTask $UH $task2.data.id
  Check 'reverse relocate completed' ($status2 -eq 'completed') "status=$status2 err=$($script:lastTaskError)"
  $b2 = DownloadText $UH 'cloudreve://my/tree/sub/b.txt'
  Check 'content intact after reverse relocate' ($b2 -eq 'bravo') "got '$b2'"

  # --- single-policy group must be rejected ---
  $group2 = (Call GET '/api/v4/admin/group/2' $AH $null).data
  $group2.edges = @{ storage_policies = @( @{id=1} ) }
  Call PUT '/api/v4/admin/group/2' $AH @{ group=$group2 } | Out-Null
  $rejected = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/tree/a.txt'); dst_policy_id=$policyB }
  Check 'single-policy group rejected' (($rejected.code -ne 0) -and ($rejected.msg -like '*single storage policy*')) "code=$($rejected.code) msg=$($rejected.msg)"

  "  (db after) $($after -replace "`r?`n", ' | ')"
}
finally {
  if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
}

""
"RESULT: $pass passed, $fail failed"
if ($fail -gt 0) { exit 1 }
