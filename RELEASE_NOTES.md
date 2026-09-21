# v0.1.1 Release Notes

v0.1.1 是 Windows/Linux amd64 本地专服核心补丁版本。正式资产为 `d2core-v0.1.1-windows-amd64.zip`、`d2core-v0.1.1-linux-amd64.zip` 及 SHA256。完整提交和构建时间以随包 BUILD.json 与 `d2core version --json` 交叉核对。

## 修复

- Windows 早退进程错误分类：身份读取失败时，仅在确认原进程已退出后归类为 START_FAILED；尚未确认退出时保持 IDENTITY_UNVERIFIED，不放宽身份核验。
- operation 已对外 terminal 后，后续合法 restart 不再因为旧 worker 内部收尾窗口偶发 BUSY；保留内部收尾顺序，不依赖客户端 sleep/retry。
- Windows/Linux CI 覆盖完整 test、vet、build、race，以及时序回归重复检查。Windows race 使用已验证的 external-link 条件。

## 标准托管模板

两个标准 example 保留 `sv_hibernate_when_empty 0`，新增 `dota_quit_after_game 0`。空服不依赖自动 hibernate，一局结束后不依赖 Dota 自行退出，实例由外部管理端显式 stop → reclaim。这是可按场景调整的推荐部署策略，不是协议硬要求，核心不强制注入。

## 兼容与升级

protocolVersion=1、模板 schemaVersion=1、磁盘 formatVersion=2 均不变。v0.1.0 既有身份核验、PID 复用防护、停止/恢复、幂等和持久化安全约束保持。产品逻辑真实复验基线为 a174332d7a9b6008fcc820d05d38929a88c9dbc0；之后仅整理示例、说明与版本元数据。最终自动验证和 example 双平台真实烟测结果随 Release 验证记录提供。

升级前显式停止并回收实例，备份所需历史，再替换运行包。格式 1 不自动迁移，不由核心更新 Dota 或 VPK。历史 v0.1.0 和 RC 标签、产物不覆盖。

既有已知限制继续有效：仅本机同用户管理、ASCII 路径、运行库与地图自行准备、Ready 不代表真人可进房、IPv6 通配双栈未验证、无容量承诺。m0-inspect 仅为 M0 历史诊断、非生产用途，正式外部集成不依赖它。详见 README 与运维说明。
