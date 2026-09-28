# Changelog

本文区分未发布候选与正式 Runtime Release。main 发布后的文档、授权澄清和维护提交不构成新版本，也不自动替代生产集成固定的 tag/commit。

## v0.1.2（正式发布准备，尚未发布）

- 固定 LF checkout，构建输入/ZIP 静态内容逐字节核对 Git blobs；机器生成并核验 Manifest，统一候选版本入口。
- Release artifact completeness：自包含文档、配置示例与构建身份校验。
- 纳入 MIT 授权、历史授权范围及实际依赖许可证/声明材料。
- 强化公网模板的精确 Steam readiness 必需规则及 Ready 边界说明。
- 增加打包回归、ZIP 内容/校验值/原生版本验证。
- 无 Runtime 新功能：生命周期、协议 1、模板 schema 1、磁盘格式 2 保持不变。
- 历史 v0.1.1 tag 与资产保持不变，当前 stable 仍为 v0.1.1。

## v0.1.1

- 修复 Windows 早退进程的身份错误分类，以及 operation terminal 后立即 restart 的时序问题。
- 保留双平台自动验证与托管模板示例，实例由显式 stop/reclaim 管理。
- protocolVersion=1、模板 schemaVersion=1、磁盘 formatVersion=2 不变。
- 固定源码：`988720ad85af1f0d97bfe98ec4da4fcbb070beea`。

详见 [固定版本 Release Notes](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/v0.1.1/RELEASE_NOTES.md) 和 [正式 Release](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1)。

## v0.1.0

首个 Windows/Linux amd64 正式本地专服核心：create/status/operation、restart/stop 回收、创建幂等、进程身份核验、持久化与恢复、多实例及端口分配、日志和同用户本地 API。

历史依据见 [固定版本 Release Notes](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/v0.1.0/RELEASE_NOTES.md) 和 [正式 Release](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.0)。
