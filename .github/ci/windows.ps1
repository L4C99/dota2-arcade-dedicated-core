param([Parameter(Mandatory=$true)][ValidateSet('prepare','test','vet','build','race')][string]$Mode)
# CI only. Hosted tools only; no game installation or system configuration changes.
$ErrorActionPreference='Stop'
$logs=Join-Path $env:RUNNER_TEMP 'd2core-ci-logs'
New-Item -ItemType Directory -Force $logs | Out-Null
if($Mode -eq 'prepare') {
    $go=(Get-Command go -ErrorAction Stop).Source
    $gcc=(Get-Command gcc -ErrorAction Stop).Source
    $expected=(Select-String -LiteralPath (Join-Path $env:GITHUB_WORKSPACE 'source/go.mod') -Pattern '^go (.+)$').Matches[0].Groups[1].Value
    if((& $go env GOVERSION) -ne "go$expected"){throw 'Go version mismatch'}
    $root=Join-Path $env:RUNNER_TEMP 'd2core-ci-work'
    New-Item -ItemType Directory -Force $root | Out-Null
    @("D2_GO=$go",'CC=gcc',"GOCACHE=$root/cache","GOMODCACHE=$root/mod") | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append
    $env:GOCACHE="$root/cache"; $env:GOMODCACHE="$root/mod"
    Set-Location (Join-Path $env:GITHUB_WORKSPACE 'source')
    $info=@(& $go version; & $gcc --version; git rev-parse HEAD; "CC=$gcc")
    $info | Tee-Object -FilePath "$logs/prepare.log"
    if(git status --porcelain){throw 'dirty checkout'}
    & $go mod download; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}
    & $go mod verify; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}
    if(git status --porcelain){throw 'dependency files changed'}
    exit 0
}
Set-Location (Join-Path $env:GITHUB_WORKSPACE 'source')
switch($Mode){
    test { $checkArgs=@('test','./...','-count=1') }
    vet { $checkArgs=@('vet','./...') }
    build { $checkArgs=@('build','-o',(Join-Path $env:RUNNER_TEMP 'd2core-ci-work/d2core.exe'),'./cmd/d2core') }
    race { $checkArgs=@('test','-race','-ldflags=-linkmode=external','./...','-count=1') }
}
& $env:D2_GO @checkArgs 2>&1 | Tee-Object -FilePath "$logs/$Mode.log"
exit $LASTEXITCODE
