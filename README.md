# Dota 2 游廊专服核心

面向联机启动器的本机专服管理核心，采用 Go，通过 CLI 与受访问限制的本地协议使用。已完成 M0 双平台运行基线；当前代码为只读环境采集与实验探针，尚无正式管理端或开房功能。

- [开工任务书](任务计划书.md)：独立的产品规则、Git/GitHub 管理约定、M0/M1 执行任务及完整交付要求。
- [Windows 模板示例](examples/template.windows.json)：配置格式基线，路径与地图需替换；Linux 示例由新项目实测后补齐。

项目直接位于 `F:/dota2-arcade-dedicated-core`。实例创建即启动，主动停止后自动回收，重启保留实例身份；历史诊断单独保留。

- [M0 环境与测试输入](docs/validation/environment.md)
- [M0 验证记录与执行步骤](docs/validation/m0.md)
- [M0 验收结论与运行限制](docs/validation/m0-results.md)
- [阶段决策与审视意见落实](docs/decisions.md)

当前工具：`go run ./cmd/d2core m0-inspect` 输出 JSON，缺失资源明确标为 `missing_input`，引擎验收始终为 `not_run`。可用参数见 M0 验证记录。它不是任务书中的正式 `check` 接口。

开发工具链为 Go 1.27.1。本地测试：`go test ./...`；静态检查：`go vet ./...`。测试只启动有时间上限的测试子进程，不启动 Dota。代码已推送至公开仓库 [L4C99/dota2-arcade-dedicated-core](https://github.com/L4C99/dota2-arcade-dedicated-core)，按用户要求暂不添加许可证，尚未发布产品版本。

[首次双平台 CI](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35335806449) 已通过 test/vet/build；真实 Dota 验收另见 M0 记录。凭据及推送方式见 [GitHub 约定](docs/github.md)。

游戏资源、VPK、实际运行配置、凭据和原始敏感日志不上传 GitHub。

M0 已确认两平台真实进房、停止、重开与中断恢复机制。引擎日志路径要求 ASCII 字符（可含空格），中文玩家名正常记录。跨机客户端需要能解析同一 addon 名称的匹配资源；绝对路径仅在对应机器有效。系统工具实验不等于正式 Go 管理端已实现；后续工作仍按 M1–M4 验收。
