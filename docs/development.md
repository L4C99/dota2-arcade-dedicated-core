# 开发与交付维护（非生产操作指南）

源码仓库保留 cmd/internal/client 自动测试、.github CI，以及 docs/validation 中有长期复验价值的 M0—M4、RC 和复核结论。这些材料不是运行依赖，也不随 Release 包分发。

## 仓库布局

- cmd、internal：CLI 和核心实现、自动测试。
- client：对外 Go 本地协议客户端及测试。
- examples：部署模板及可读外部调用示例；示例程序不是生产常驻控制器。
- tools/m0：非生产环境探针；只在隔离测试环境使用。
- tools/package：维护者构建/归档工具，非生产运行组件。
- docs：正式文档、设计背景和阶段证据；运行包仅选取正式文档。
- local、dist、data、build：忽略的本地产物，不提交。根目录私人任务计划、VPK 同样忽略。

不要提交密钥、个人目录配置、一次性脚本、原始玩家日志、抓包、进程转储或本机诊断输出。长期结论需脱敏写成验证文档；原始证据保留在本地隔离目录。不要因文件处于忽略目录就将它作为公开交付内容。

## 开发验证

固定 Go 1.27.1：`go test ./...`、`go vet ./...`、`go build ./cmd/d2core`。源码默认版本 0.1.0-dev。CI 和假引擎测试不代替真实 Dota 验收。M0—M4 的历史环境、限制和复验方式见 validation；review-fixes 是保留的复核结论，不是原始私有 review 产物。

## 正式构建

先提交并推送验收内容，从远端新 clone，detach 到确认的完整 SHA，确认工作树干净；使用独立构建缓存。执行：

```text
go run ./tools/package --go ABS_GO --output ABS_NEW_OUTPUT --version 0.1.0 --build-time RFC3339_UTC
```

工具固定 amd64、CGO_ENABLED=0、trimpath 与文件白名单，拒绝脏工作树和覆盖已有输出。不指定版本生成开发包。相同提交/工具链/时间用于复现；不要将开发目录旧二进制带入归档。

核查两平台 ZIP 文件列表不含 CI、测试、M0、review 或 validation；检查随包 Markdown 链接、BUILD.json 与 version JSON。运行目标平台二进制和必要烟测，重新计算 SHA256。更新发布状态和最终验证记录时必须如实区分候选测试与正式资产测试。标签、提交、文件名、程序版本和 Release Notes 必须一致；发布是独立步骤，工具不会自动发布。
