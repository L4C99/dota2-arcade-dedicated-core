# Dota 2 游廊专服核心

面向联机启动器的本机专服管理核心，采用 Go，通过 CLI 与受访问限制的本地协议使用。已正式进入 M0，当前实现只读环境采集与隔离测试进程实验，尚无管理端或开房功能。

- [开工任务书](任务计划书.md)：独立的产品规则、Git/GitHub 管理约定、M0/M1 执行任务及完整交付要求。
- [Windows 模板示例](examples/template.windows.json)：配置格式基线，路径与地图需替换；Linux 示例由新项目实测后补齐。

项目直接位于 `F:/dota2-arcade-dedicated-core`。实例创建即启动，主动停止后自动回收，重启保留实例身份；历史诊断单独保留。

- [M0 环境与测试输入](docs/validation/environment.md)
- [M0 验证记录与执行步骤](docs/validation/m0.md)
- [阶段决策与审视意见落实](docs/decisions.md)

当前工具：`go run ./cmd/d2core m0-inspect` 输出 JSON，缺失资源明确标为 `missing_input`，引擎验收始终为 `not_run`。可用参数见 M0 验证记录。它不是任务书中的正式 `check` 接口。

本地测试：`go test ./...`；静态检查：`go vet ./...`。测试只启动有时间上限的测试子进程，不启动 Dota。远端、最终 module 路径及许可证待确定，尚未推送或发布。

游戏资源、VPK、实际运行配置、凭据和原始敏感日志不上传 GitHub。
