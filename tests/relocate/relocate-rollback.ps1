# Rollback verification for the relocation feature.
#
# Scenario A (single file): the blob is written, then the commit is forced to fail
# via the test sentinel. The entity must end up exactly as before, in particular it
# must NOT keep the encryption metadata generated for the failed encrypting write
# (that would make a plaintext blob be read as ciphertext = corrupted file).
#
# Scenario B (two files): the first file relocates successfully, the second fails, so
# the first must be rolled back to its original policy and metadata.
# ASCII only (Windows PowerShell 5.1 reads .ps1 as ANSI when there is no BOM).
param(
  [Parameter(Mandatory = $true)][string]$Root,
  [Parameter(Mandatory = $true)][int]$Port
)

$ErrorActionPreference = 'Stop'
$base = "http://127.0.0.1:$Port"
$pass = 0; $fail = 0
$db = Join-Path $Root 'cloudreve.db'
$sentinel = Join-Path $Root 'data\relocate-fail-sentinel'

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

function UploadPlain($headers, $uri, $content) {
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($content)
  $sess = Call PUT '/api/v4/file/upload' $headers @{ uri=$uri; size=$bytes.Length; mime_type='text/plain' }
  if (-not $sess.data.session_id) { throw "upload failed: $($sess.msg)" }
  Invoke-WebRequest -Uri "$base/api/v4/file/upload/$($sess.data.session_id)/0" -Method POST -Headers @{ Authorization = $headers.Authorization; 'Content-Type'='application/octet-stream' } -Body $bytes -UseBasicParsing -TimeoutSec 60 | Out-Null
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

  Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Plain B'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400 } } } | Out-Null
  Call PUT '/api/v4/admin/policy' $AH @{ policy=@{ name='Encrypted C'; type='local'; dir_name_rule='uploads/{uid}/{path}'; file_name_rule='{uid}_{randomkey8}_{originname}'; settings=@{ chunk_size=26214400; encryption=$true } } } | Out-Null

  $group = (Call GET '/api/v4/admin/group/2' $AH $null).data
  $group.edges = @{ storage_policies = @( @{id=1}, @{id=2}, @{id=3} ) }
  Call PUT '/api/v4/admin/group/2' $AH @{ group=$group } | Out-Null

  Call POST '/api/v4/user' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' } | Out-Null
  $ulogin = Call POST '/api/v4/session/token' @{'Content-Type'='application/json'} @{ email='user@example.com'; password='Passw0rd!23' }
  $UH = @{ Authorization = "Bearer $($ulogin.data.token.access_token)"; 'Content-Type' = 'application/json' }
  $up = Call GET '/api/v4/user/policies' $UH $null
  $plainId = ($up.data.policies | Where-Object { $_.name -eq 'Plain B' }).id
  $encId = ($up.data.policies | Where-Object { $_.name -eq 'Encrypted C' }).id

  # ---------- Scenario A: single plain file -> encrypting policy, commit forced to fail
  Call POST '/api/v4/file/create' $UH @{ uri='cloudreve://my/a'; type='folder' } | Out-Null
  UploadPlain $UH 'cloudreve://my/a/one.txt' 'plain-one-payload'
  $beforeA = & go run ./tools/relocate-check -db $db -props 2>&1 | Out-String
  Check 'A: starts plaintext on policy 1' (($beforeA -match 'policy=1') -and ($beforeA -match 'encrypt_metadata=none')) "db: $beforeA"

  New-Item -ItemType File -Force -Path $sentinel | Out-Null
  $taskA = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/a'); dst_policy_id=$encId }
  $statusA = WaitTask $UH $taskA.data.id
  Remove-Item $sentinel -Force

  Check 'A: task reports failure' ($statusA -eq 'error') "status=$statusA"
  $afterA = & go run ./tools/relocate-check -db $db -props 2>&1 | Out-String
  Check 'A: location restored to policy 1' ($afterA -match 'policy=1') "db: $afterA"
  Check 'A: NO leftover key material' ($afterA -match 'encrypt_metadata=none') "db: $afterA"
  $contentA = DownloadText $UH 'cloudreve://my/a/one.txt'
  Check 'A: file still readable (not corrupted)' ($contentA -eq 'plain-one-payload') "got '$contentA'"

  # ---------- Scenario B: two files, second one's blob removed so it fails
  Call POST '/api/v4/file/create' $UH @{ uri='cloudreve://my/b'; type='folder' } | Out-Null
  UploadPlain $UH 'cloudreve://my/b/first.txt' 'first-payload'
  UploadPlain $UH 'cloudreve://my/b/second.txt' 'second-payload'

  $secondBlob = Get-ChildItem (Join-Path $Root 'data\uploads') -Recurse -File | Where-Object { Select-String -Path $_.FullName -Pattern 'second-payload' -Quiet }
  Check 'B: second blob located' ([bool]$secondBlob) 'not found'
  if ($secondBlob) { Remove-Item $secondBlob.FullName -Force }

  $taskB = Call POST '/api/v4/workflow/relocate' $UH @{ src=@('cloudreve://my/b'); dst_policy_id=$plainId }
  $statusB = WaitTask $UH $taskB.data.id
  Check 'B: task reports failure' ($statusB -eq 'error') "status=$statusB err=$($script:lastError)"

  $afterB = & go run ./tools/relocate-check -db $db -props 2>&1 | Out-String
  $firstPolicy = ([regex]::Match($afterB, 'file=first\.txt\s+entity=\d+\s+policy=(\d)')).Groups[1].Value
  $secondPolicy = ([regex]::Match($afterB, 'file=second\.txt\s+entity=\d+\s+policy=(\d)')).Groups[1].Value
  # The first file completed its move and must stay there (per-file atomicity); the
  # second failed and must be untouched on the source policy.
  Check 'B: completed file stays on the target policy' ($firstPolicy -eq '2') "report: $afterB"
  Check 'B: failed file remains untouched on the source policy' ($secondPolicy -eq '1') "report: $afterB"
  $contentB = DownloadText $UH 'cloudreve://my/b/first.txt'
  Check 'B: first file still readable after the task failed' ($contentB -eq 'first-payload') "got '$contentB'"

  "  (A) status=$statusA | $($afterA -replace "`r?`n", ' | ')"
  "  (B) status=$statusB | $($afterB -replace "`r?`n", ' | ')"
}
finally {
  if (Test-Path $sentinel) { Remove-Item $sentinel -Force }
  if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
}

""
"RESULT: $pass passed, $fail failed"
if ($fail -gt 0) { exit 1 }
