#requires -Version 7.0
# M0 experiment only; not a production manager. Configuration and evidence stay local.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('launch', 'observe', 'force-stop')][string]$Action,
    [Parameter(Mandatory)][string]$RunDirectory,
    [switch]$FaultAfterStartBeforeIdentity
)
$ErrorActionPreference = 'Stop'
if (-not [IO.Path]::IsPathFullyQualified($RunDirectory)) { throw 'Absolute run directory required' }
$runRoot = [IO.Path]::GetFullPath($RunDirectory)
$manifestPath = Join-Path $runRoot 'process.json'
$configPath = Join-Path $runRoot 'input.json'
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
foreach ($path in @($config.executable, $config.workingDirectory, $config.cfgDirectory, $config.vpk)) {
    if (-not [IO.Path]::IsPathFullyQualified($path) -or -not (Test-Path -LiteralPath $path)) { throw "Invalid input path: $path" }
}
if ($config.runId -notmatch '^d2core_m0_[a-z0-9_]+$') { throw 'Invalid experiment ID' }
if ($config.map -notmatch '^[a-zA-Z0-9_]+$') { throw 'Invalid map name' }
if ([int]$config.port -lt 1024 -or [int]$config.port -gt 65535) { throw 'Invalid test port' }
$cfgPath = Join-Path $config.cfgDirectory ($config.runId + '.cfg')
$logPath = Join-Path $runRoot 'engine.log'

if ($Action -eq 'launch') {
    # An interrupted launch requires investigation, never blindly run again.
    if (Test-Path -LiteralPath (Join-Path $runRoot 'intent.json')) { throw 'Existing launch intent: inspect possible process; use a new run only after resolving it' }
    if (Test-Path -LiteralPath $cfgPath) { throw 'Refusing to overwrite existing engine cfg' }
    if (Test-Path -LiteralPath $logPath) { throw 'Refusing to reuse engine log' }
    $vpkEnginePath = $config.vpk.Replace('\', '/')
    if ($vpkEnginePath -match '["\r\n]') { throw 'Unsupported cfg path characters' }
    $cfg = "hostname `"$($config.runId)`"`nmap $($config.map) gamemode=15 customgamemode=`"$vpkEnginePath`" nomapvalidation=1`nsv_hibernate_when_empty 0`n"
    $arguments = @('-dedicated', '-allow_no_lobby_connect', '-ip', '127.0.0.1', '-port', [string]$config.port, '-con_logfile', $logPath.Replace('\', '/'), '+exec', ($config.runId + '.cfg'))
    $intent = @{ recordedAt = [DateTime]::UtcNow.ToString('o'); executable = $config.executable; workingDirectory = $config.workingDirectory; arguments = $arguments; cfgPath = $cfgPath; cfg = $cfg }
    $intent | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $runRoot 'intent.json') -Encoding utf8
    [IO.File]::WriteAllText((Join-Path $runRoot 'startup.cfg'), $cfg, [Text.UTF8Encoding]::new($false))
    # CreateNew prevents clobbering another experiment's cfg even after a race.
    $cfgStream = [IO.File]::Open($cfgPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::Read)
    try { $bytes = [Text.Encoding]::UTF8.GetBytes($cfg); $cfgStream.Write($bytes, 0, $bytes.Length) } finally { $cfgStream.Dispose() }
    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = $config.executable
    $startInfo.WorkingDirectory = $config.workingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = $true
    foreach ($argument in $arguments) { $startInfo.ArgumentList.Add($argument) }
    $engineProcess = [Diagnostics.Process]::Start($startInfo)
    # Explicit M0 fault injection: leave intent without identity, bypassing cleanup.
    if ($FaultAfterStartBeforeIdentity) { [Environment]::Exit(73) }
    try {
        $identity = @{ pid = $engineProcess.Id; startTimeUtc = $engineProcess.StartTime.ToUniversalTime().ToString('o'); executable = $engineProcess.MainModule.FileName; runId = $config.runId; recordedAt = [DateTime]::UtcNow.ToString('o') }
        $identity | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding utf8
        $identity | ConvertTo-Json
    } finally { $engineProcess.Dispose() }
    exit 0
}

$identity = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$engineProcess = [Diagnostics.Process]::GetProcessById([int]$identity.pid)
try {
    # Pin an OS process handle before identity checks and use that same object to kill.
    $null = $engineProcess.Handle
    if ($engineProcess.HasExited) { throw 'Process already exited; no termination attempted' }
    $actualPath = $engineProcess.MainModule.FileName
    $actualStart = $engineProcess.StartTime.ToUniversalTime().ToString('o')
    # PowerShell versions differ in automatic JSON ISO-date conversion.
    $expectedStart = ([DateTimeOffset]$identity.startTimeUtc).UtcDateTime.ToString('o')
    if (-not [string]::Equals($actualPath, [string]$identity.executable, [StringComparison]::OrdinalIgnoreCase) -or $actualStart -ne $expectedStart) { throw 'Process identity mismatch: no termination attempted' }
    if (-not [string]::Equals([IO.Path]::GetFullPath($config.executable), [IO.Path]::GetFullPath($actualPath), [StringComparison]::OrdinalIgnoreCase)) { throw 'Configuration identity mismatch' }
    if ($Action -eq 'observe') {
        @{ recordedAt = [DateTime]::UtcNow.ToString('o'); pid = $engineProcess.Id; startTimeUtc = $actualStart; executable = $actualPath; logBytes = $(if (Test-Path -LiteralPath $logPath) { (Get-Item -LiteralPath $logPath).Length } else { 0 }) } | ConvertTo-Json
    } else {
        $engineProcess.Kill()
        if (-not $engineProcess.WaitForExit(10000)) { throw 'Exit unconfirmed; retain all records and cfg' }
        @{ recordedAt = [DateTime]::UtcNow.ToString('o'); pid = $identity.pid; method = 'forced-single-process'; exited = $true; exitCode = $engineProcess.ExitCode; childrenVerified = $false; portsVerified = $false } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runRoot 'stop.json') -Encoding utf8
        # Never automatically clean cfg here: retain evidence for explicit owned cleanup.
        Get-Content -LiteralPath (Join-Path $runRoot 'stop.json')
    }
} finally { $engineProcess.Dispose() }
