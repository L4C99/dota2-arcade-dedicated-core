# M0 实验工具

这些工具用于验证运行前提，不是正式管理端。Go 源码位于 cmd/d2core 和 internal/m0；下列 PowerShell 探针要求 Windows 与 PowerShell 7。

## 只读采集

```sh
go run ./cmd/d2core m0-inspect
```

可显式传入 --executable、--working-directory、--cfg-directory、--vpk、--gameinfo、--version-file；值必须是本平台绝对路径。VPK 会完整读取以计算 SHA256。JSON 写 stdout，诊断写 stderr；缺少输入标记 missing_input，错误输入返回非零，engineValidation 始终为 not_run。

suppliedInputsValid 仅描述已提供项的静态检查，空输入也可能为 true；它不表示写权限、地图加载或客户端进房已通过。

## Windows 引擎探针

仅在明确隔离的测试安装与空闲端口使用。每次实验创建新的绝对运行目录，并保存 input.json，例如：

```json
{
  "runId": "d2core_m0_example01",
  "executable": "C:/Games/Dota2/game/bin/win64/dota2.exe",
  "workingDirectory": "C:/Games/Dota2/game",
  "cfgDirectory": "C:/Games/Dota2/game/dota/cfg",
  "vpk": "C:/Maps/sample_addon.vpk",
  "map": "example_map",
  "port": 27015
}
```

路径、地图与端口必须按环境替换；运行目录使用 ASCII 路径。探针会向 cfgDirectory 新增唯一 cfg，不可指向未经允许修改的安装。当前 Windows 启动探针采用 VPK 绝对路径，适合同机资源验证；跨机器 addon 资源定位须另行准备，不能将本例直接视为远端连接方案。

```powershell
./tools/m0/windows-engine.ps1 -Action launch -RunDirectory C:/D2CoreTests/run01
./tools/m0/windows-engine.ps1 -Action observe -RunDirectory C:/D2CoreTests/run01
pwsh -NoProfile -File ./tools/m0/windows-quit.ps1 -RunDirectory C:/D2CoreTests/run01
```

- launch 保存意图与身份；已有意图拒绝重试。-FaultAfterStartBeforeIdentity 专用于故障注入，会在启动引擎后直接退出并留下未记录身份的进程，普通复验不要启用。
- observe 重新持有进程句柄，核验路径和精确创建时间。
- windows-quit.ps1 必须在单独 PowerShell 进程执行，确认独占控制台后发 quit；等待 10 秒，超时保留记录，不自动强杀。
- 必要时用 windows-engine.ps1 -Action force-stop -RunDirectory ... 终止已核验的测试进程；它不是全进程树清理工具。必须另查子进程与端口，不能只看返回成功。
- windows-token.ps1 -ProcessIds ... 只读查询指定进程的路径与提权状态，不修改令牌或进程。

探针保留 cfg 和日志；确认退出后按文件归属及归档内容手动清理唯一 cfg，不能批量清理安装目录。不得仅凭端口或 PID 接管其他进程。完整验收方法与已知限制见 [M0 报告](../../docs/validation/m0.md)。
