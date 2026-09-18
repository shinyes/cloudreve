# Extra check: relocating a PLAIN file onto an ENCRYPTING policy.
# Covers the path that failed with "failed to set cryptor source: metadata not loaded".
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
  if (-not $sess.data.session_id) { throw "upload failed for $uri : $($sess.msg)" }
  Invoke-WebRequest -Uri "$base/api/v4/file/upload/$($sess.data.session_id)/0" -Method POST -Headers @{ Authorization = $headers.Authorization; 'Content-Type'='application/octet-stream' } -Body $bytes -UseBasicParsing -TimeoutSec 60 | Out-Null
  return $sess.data.storage_policy.name
}

function DownloadText($headers, $uri) {
  $u = Call POST '/api/v4/file/url' $headers @{ uris=@($uri) }
  $path = ([System.Uri]$u.data.urls[0].url).PathAndQuery
  return (Invoke-WebRequest "$base$path" -Headers $headers -UseBasicParsing -TimeoutSec 60).Content
}

function WaitTask($headers, $taskID) {
  $status = ''
  for ($i = 1; $i -le 60; $i++) {
    Start-Sleep -Seconds 2
    $list = Call GET '/api/v4/workflow?page_size=10&category=general' $headers $null
    $t = $list.data.tasks | Where-Object { $_.id -eq $taskID }
    if ($t) { $status = $t.status; if ($t.error) { $script:lastError = $t.error } }
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

  # Policy 2: plain. Policy 3: ENCRYPTING (the target of the failing case).
  Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Plain B'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400 } } } | Out-Null
  $c = Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Encrypted C'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400; encryption=$true } } }
  Check 'encrypting policy created' (($c.code -eq 0) -and $c.data.settings.encryption) "code=$($c.code)"

  $group = (Call GET '/api/v4/admin/group/2' $AH $null).data
  $group.edges = @{ storage_policies = @( @{id=1}, @{id=2}, @{id=3} ) }
  Call PUT '/api/v4/admin/group/2' $AH @{ group=$group } | Out-Null

  Call POST '/api/v4/user' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' } | Out-Null
  $ulogin = Call POST '/api/v4/session/token' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' }
  $UH = @{ Authorization = "Bearer $($ulogin.data.token.access_token)"; 'Content-Type' = 'application/json' }

  $up = Call GET '/api/v4/user/policies' $UH $null
  $encId = ($up.data.policies | Where-Object { $_.name -eq 'Encrypted C' }).id
  Check 'user sees encrypting policy' ([bool]$encId) "got '$encId'"

  Call POST '/api/v4/file/create' $UH @{ uri='cloudreve://my/plain'; type='folder' } | Out-Null
  $where = UploadFile $UH 'cloudreve://my/plain/note.txt' 'plain-to-encrypted-payload'
  Check 'file starts on a non-encrypting policy' ($where -ne 'Encrypted C') "got '$where'"

  # THE case that used to fail: plain blob -> encrypting policy.
  $task = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/plain'); dst_policy_id=$encId }
  Check 'relocate task created' (($task.code -eq 0) -and $task.data.id) "code=$($task.code) msg=$($task.msg)"
  $status = WaitTask $UH $task.data.id
  Check 'plain -> encrypting relocate completed' ($status -eq 'completed') "status=$status err=$($script:lastError)"

  # The file must still read back correctly (proves it was encrypted on write and
  # that the recorded key material matches the stored ciphertext).
  $content = DownloadText $UH 'cloudreve://my/plain/note.txt'
  Check 'content intact after encrypting relocation' ($content -eq 'plain-to-encrypted-payload') "got '$content'"

  $db = Join-Path $Root 'cloudreve.db'
  $report = & go run ./tools/relocate-check -db $db 2>&1 | Out-String
  Check 'entity now on the encrypting policy' ($report -match 'policy=3') "db: $report"

  "  (db) $($report -replace "`r?`n", ' | ')"
}
finally {
  if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
}

""
"RESULT: $pass passed, $fail failed"
if ($fail -gt 0) { exit 1 }
