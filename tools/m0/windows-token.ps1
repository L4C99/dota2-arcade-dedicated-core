#requires -Version 7.0
# Read-only diagnostic. Does not inject code, modify tokens, or terminate processes.
[CmdletBinding()]
param([int[]]$ProcessIds)
$ErrorActionPreference = 'Stop'
if (-not ('D2CoreM0TokenProbe' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class D2CoreM0TokenProbe {
 [DllImport("kernel32.dll", SetLastError=true)] static extern IntPtr OpenProcess(uint access, bool inherit, uint id);
 [DllImport("kernel32.dll", SetLastError=true, CharSet=CharSet.Unicode)] static extern bool QueryFullProcessImageName(IntPtr process, uint flags, StringBuilder path, ref uint size);
 [DllImport("advapi32.dll", SetLastError=true)] static extern bool OpenProcessToken(IntPtr process, uint access, out IntPtr token);
 [DllImport("advapi32.dll", SetLastError=true)] static extern bool GetTokenInformation(IntPtr token, int cls, out int result, int size, out int needed);
 [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
 public static string Inspect(uint id) {
   // PROCESS_QUERY_LIMITED_INFORMATION and TOKEN_QUERY only.
   var p=OpenProcess(0x1000,false,id);
   if(p==IntPtr.Zero) return "process query error="+Marshal.GetLastWin32Error();
   try {
    var path=new StringBuilder(32768); uint n=32768;
    var ok=QueryFullProcessImageName(p,0,path,ref n);
    string s="path="+(ok?path.ToString():"unknown"); IntPtr token;
    if(!OpenProcessToken(p,8,out token)) return s+"; token query error="+Marshal.GetLastWin32Error();
    try {
     int elevated, needed;
     if(!GetTokenInformation(token,20,out elevated,4,out needed)) return s+"; elevation query error="+Marshal.GetLastWin32Error();
     return s+"; elevated="+elevated;
    } finally {CloseHandle(token);}
   } finally {CloseHandle(p);}
 }
}
'@
}
if (-not $ProcessIds) {
    $ProcessIds = @(Get-Process dota2,steam -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Id)
}
$results = @(
    foreach ($processIdValue in $ProcessIds) {
        if ($processIdValue -le 0) { throw 'Expected a positive process ID' }
        [pscustomobject]@{
            recordedAt = [DateTime]::UtcNow.ToString('o')
            pid = $processIdValue
            observation = [D2CoreM0TokenProbe]::Inspect($processIdValue)
        }
    }
)
ConvertTo-Json -InputObject $results
