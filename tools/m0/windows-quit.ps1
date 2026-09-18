#requires -Version 7.0
# Run in a separate PowerShell process: this helper changes its own console attachment.
# M0 experiment only. Sends the engine's quit command, never terminates by PID alone.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$RunDirectory)
$ErrorActionPreference = 'Stop'
if (-not [IO.Path]::IsPathFullyQualified($RunDirectory)) { throw 'Absolute run directory required' }
$identity = Get-Content -LiteralPath (Join-Path $RunDirectory 'process.json') -Raw | ConvertFrom-Json
$process = [Diagnostics.Process]::GetProcessById([int]$identity.pid)
try {
    $null = $process.Handle
    $expectedTime = ([DateTimeOffset]$identity.startTimeUtc).UtcDateTime
    if ($process.HasExited -or $process.StartTime.ToUniversalTime() -ne $expectedTime -or -not [string]::Equals($process.MainModule.FileName, $identity.executable, [StringComparison]::OrdinalIgnoreCase)) { throw 'Identity mismatch; no console command sent' }
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
public static class D2CoreM0ConsoleQuit {
 [StructLayout(LayoutKind.Explicit, Size=20)] public struct Input {
  [FieldOffset(0)] public ushort Type;
  [FieldOffset(4)] public int Down;
  [FieldOffset(8)] public ushort Repeat;
  [FieldOffset(10)] public ushort VirtualKey;
  [FieldOffset(12)] public ushort Scan;
  [FieldOffset(14)] public ushort Character;
  [FieldOffset(16)] public uint Control;
 }
 [DllImport("kernel32.dll", SetLastError=true)] static extern bool FreeConsole();
 [DllImport("kernel32.dll", SetLastError=true)] static extern bool AttachConsole(uint id);
 [DllImport("kernel32.dll", SetLastError=true)] static extern uint GetConsoleProcessList([Out] uint[] list, uint count);
 [DllImport("kernel32.dll", SetLastError=true, CharSet=CharSet.Unicode)] static extern IntPtr CreateFile(string name,uint access,uint share,IntPtr security,uint disposition,uint flags,IntPtr template);
 [DllImport("kernel32.dll", SetLastError=true, EntryPoint="WriteConsoleInputW")] static extern bool WriteConsoleInput(IntPtr handle,Input[] records,uint count,out uint written);
 [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
 public static void Send(uint target) {
  FreeConsole();
  if(!AttachConsole(target)) throw new Win32Exception(Marshal.GetLastWin32Error(),"AttachConsole failed");
  try {
   var ids=new uint[16]; uint count=GetConsoleProcessList(ids,16);
   if(count==0 || count>16) throw new Exception("Cannot verify console ownership");
   bool found=false; uint own=(uint)System.Diagnostics.Process.GetCurrentProcess().Id;
   for(int i=0;i<count;i++) { if(ids[i]==target) found=true; else if(ids[i]!=own) throw new Exception("Shared console; refusing command"); }
   if(!found) throw new Exception("Target not attached to console");
   var handle=CreateFile("CONIN$",0xC0000000,3,IntPtr.Zero,3,0,IntPtr.Zero);
   if(handle==new IntPtr(-1)) throw new Win32Exception(Marshal.GetLastWin32Error(),"Open console input failed");
   try {
    string command="quit\r"; var records=new Input[command.Length*2];
    for(int i=0;i<command.Length;i++) {
     var key=new Input { Type=1,Down=1,Repeat=1,Character=command[i],VirtualKey=(ushort)Char.ToUpperInvariant(command[i]) };
     records[i*2]=key; key.Down=0; records[i*2+1]=key;
    }
    uint written;
    if(!WriteConsoleInput(handle,records,(uint)records.Length,out written) || written!=records.Length) throw new Exception("Console write incomplete; observe process before retrying");
   } finally {CloseHandle(handle);}
  } finally {FreeConsole();}
 }
}
'@
    if ($process.HasExited) { throw 'Process exited before console request' }
    $started = [DateTime]::UtcNow
    [D2CoreM0ConsoleQuit]::Send([uint32]$identity.pid)
    $exited = $process.WaitForExit(10000)
    $result = @{ recordedAt = [DateTime]::UtcNow.ToString('o'); pid = $identity.pid; method = 'engine-quit-via-owned-console'; commandSent = $true; exited = $exited; elapsedMilliseconds = ([DateTime]::UtcNow - $started).TotalMilliseconds; exitCode = $(if ($exited) { $process.ExitCode } else { $null }); timeoutMilliseconds = 10000 }
    $result | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $RunDirectory 'quit-result.json') -Encoding utf8
    $result | ConvertTo-Json
    if (-not $exited) { throw 'Quit timed out; process and cfg retained; no automatic force kill' }
} finally { $process.Dispose() }
