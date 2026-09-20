# Dota 2 游廊专服核心

面向联机启动器的本机专服管理核心，采用 Go 开发，目标平台为 Windows 和 Linux。首版既定范围已实现并验收。

## 当前状态

M0—M4 既定专服核心已完成双平台验收，包括创建、重启、停止回收、幂等请求、管理端中断恢复、多实例隔离及真实整机重启后的旧记录处理。Windows 10 与 Ubuntu 24 均有真实客户端进房证据；A2S 配置和查询、外部调用示例、双平台构建包已交付。

核心提供本地 CLI 和受限本地协议，可独立运行。平台、节点控制器和未来控制台扩展不在本轮范围。验收仅覆盖已记录的地图和环境，不代表任意地图兼容、容量承诺或已部署生产。详见阶段记录及交付说明。

## 构建与检查

需要 Go 1.27.1。在仓库根目录执行：

```sh
go test ./...
go vet ./...
go build ./cmd/d2core
go run ./cmd/d2core m0-inspect
```

测试仅启动有时间上限的测试进程，不启动 Dota。[CI](https://github.com/L4C99/dota2-arcade-dedicated-core/actions) 在 Windows/Linux 上运行测试、静态检查和构建；真实引擎验收独立进行。

## 文档

RC.1 两平台本轮单房间产物烟测已由用户确认通过，详见 [烟测记录与包外勘误](docs/validation/rc1-smoke.md)。原 RC.1 包及 tag 保持不变，未发布正式生产 Release。

- [开发路线图](docs/roadmap.md)：阶段目标和验收标准。
- [技术决策](docs/decisions.md)：实现约束与后续验证责任。
- [M0 验证报告](docs/validation/m0.md)：环境、结果、限制和复验流程。
- [本地协议](docs/local-api.md)：M1 契约，接口可用性以阶段记录为准。
- [操作说明](docs/operations.md)：本地调用与权限要求。
- [M1 进度与验收](docs/validation/m1.md)：实现、自动验证和真实进房分开记录。
- [M2 进度与验收](docs/validation/m2.md)：恢复、持久化和整机重启验收状态。
- [M3 进度与验收](docs/validation/m3.md)：多实例、自动端口和存储策略。
- [M4交付进度](docs/validation/m4.md)、[构建包说明](docs/delivery.md)：交付包与验收证据。
- [A2S独立配置工具](docs/a2s.md)、[外部调用示例](examples/launcher/README.md)。
- [配置示例说明](examples/README.md)：模板 v1、双平台路径和就绪规则填写说明。
- [M0 实验工具](tools/m0/README.md)：只读采集及 Windows 进程探针。

## 当前运行限制

- 引擎日志路径使用 ASCII 字符，可含空格；中文玩家名等日志内容可正常记录。
- 跨机器连接需要两端能解析同一 addon 标识，并使用匹配资源；服务器绝对路径不等于客户端可用路径。
- 已验证的地图与平台组合不代表任意地图兼容。资源准备和更新由使用者或上层程序负责。

仓库不包含游戏安装、地图包、凭据或原始玩家日志。当前未提供许可证；源码构建默认为0.1.0-dev，候选打包会注入版本标识，尚未创建GitHub Release。

RC.2 帮助/版本修正及发布包命令验收范围见 [命令矩阵](docs/validation/rc2-commands.md)。
