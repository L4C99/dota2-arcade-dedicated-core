# v0.1.0 Release Notes

正式版本：v0.1.0。发布资产及最终产物检查结果随 GitHub Release 提供；完整提交、版本与构建时间以随包 BUILD.json 和 `d2core version --json` 交叉核对。

首个正式交付范围为 Windows/Linux amd64 本地专服核心：创建、重启、停止回收、多实例端口分配、异步操作、创建幂等、状态/日志、管理器恢复与本地 API。协议版本 1、模板版本 1、磁盘格式 2。

相对已验收 RC.2，本轮仅整理首页、正式文档、开发工具标识和包文件白名单；核心生命周期、协议和安全行为未变。M0—M4 与独立复核资料保留在源码仓库，不进入运行包。自动测试保留，不分发测试源码。

Windows 10 x64、Ubuntu 24 x64 已完成 RC.2 n6 的创建进房、重启重连、停止回收。正式包从远端固定提交的干净 checkout 新构建，发布前执行产物级检查及既定 test/vet/race。核心行为冻结，本次不重跑完整 M0—M4 或真实 Dota 验收；RC.2 进房结果不冒充正式包真实进房结果。最终结果见随 Release 提供的 RELEASE-VALIDATION.md。

正式资产：`d2core-v0.1.0-windows-amd64.zip`、`d2core-v0.1.0-linux-amd64.zip` 及校验文件。程序与 BUILD.json 的 version 必须为 `0.1.0`，提交和构建时间一致，gitDirty=false。不得复用开发目录旧二进制或覆盖 RC 标签/产物。

升级前停止并回收实例、备份历史。格式 1 不自动迁移；不更新游戏/VPK。限制包括本机同用户调用、ASCII 路径、地图及运行库自行准备、ready 不代替进房确认、IPv6 通配双栈未验证、无容量承诺。完整限制见 README 与操作说明。
