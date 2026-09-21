# GitHub hosted CI 健康检查（开发验证）

本次仅修改 CI 与开发文档；v0.1.0 tag、发布资产及生命周期/协议/安全代码不变。CI 不运行 Dota、不访问 SSH 主机、不使用生产/Steam 凭据，不修改系统账户或内核安全配置。

## 历史审计

2026-09-21 审阅当时全部 21 个运行：6 次成功，15 次失败。没有重跑历史缺陷提交，保留原状态。

- 从 0ebfd72 的 [首次失败](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35444472625) 开始，15 次失败均包含 Linux Discover 读取同 UID 进程 `/proc/PID/exe` 的 permission denied，连带恢复测试失败。Go 安装成功，失败发生在 test；不是下载失败，也不是 Node 弃用警告造成。无法凭旧日志确定该 PID 是哪个进程，新增只读诊断用于辨别。
- 早期 [7bcbdb7](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35501034591)、[8b05f54](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35496143585) 还包含 TestLinuxLifecycleAndIdentity 启动身份读取失败；[0ee3ef8](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35496338744) 包含 TestLinuxManagerExitLeavesLogs 启动身份读取失败。该类既有竞态已有独立复验与后续 1a0b79c 修复记录，参见 review-fixes；本次不重写历史或重新修改产品。旧 Actions 日志不足以独立证明每一次启动错误的底层原因，不能将其全部归咎 runner。
- 正式 [v0.1.0](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35530126407) 与 main 的原运行仍红：Linux test 失败（含恢复、Discover、孤立 FIFO 回收），Windows 被矩阵 fail-fast 取消；vet/build 被跳过。不能把被取消的 Windows 当作通过。
- 旧工作流 setup-go 默认开启缓存，日志存在 cached 测试结果；未发现其直接导致上述错误的证据。旧工作流没有 race，平台用 latest 浮动标签，Action 主版本引用可漂移，默认矩阵联动取消遮蔽另一平台结果。

## 本轮 CI 策略

- windows-2025、ubuntu-24.04，各自检查当前事件 SHA 和正式 v0.1.0 原始完整 SHA `cf986dc49762621a681735a6a13f1941f577f43d`。新 CI driver 在当前提交运行，冻结版本只作为待测源码；不改原 tag，不声称旧红色 run 变绿。
- test（-count=1）、vet、build、race 全套；Windows race 使用已验证的 `-ldflags=-linkmode=external`，不跳过测试、不使用 continue-on-error。准备成功后各检查独立执行，失败仍导致 job 失败。
- Go 从待测源码 go.mod 精确固定 1.27.1，GOTOOLCHAIN=local 并断言版本；依赖 download/verify，禁止悄悄改 go.mod/go.sum。关闭跨运行缓存，每 job 独立构建/模块缓存；普通测试禁用结果缓存。
- Linux 先记录原 runner UID 下不可读的进程 exe（仅 PID/comm/错误，不输出环境或完整命令行），再使用镜像已有 nobody 非特权账户与独立临时目录运行完整测试。sudo 仅切换身份和调整该临时目录所属，不新建用户、不放宽 /proc/ptrace/AppArmor 等系统安全配置，也不以 root 运行测试。这验证独立服务账户部署条件，不承诺共享 runner UID 下同样可用。
- Windows 使用 runner 自带 gcc，记录版本和路径；缺少编译器或 race 失败会直接红，不降级跳过。
- checkout/setup-go 使用 Node24 版本，所有 Action 固定完整 SHA；token 仅 contents:read，checkout 不持久化凭据。日志上传保留 14 天，不上传游戏/状态目录。25 分钟超时，fail-fast=false。
- push 覆盖所有分支与 v* tag，PR 与 workflow_dispatch 可运行。tag/分支分别触发是正常行为；不自动发布 Release，也不移动旧 tag 来触发新 workflow。

## 结果边界

实际 hosted 验证结果将在运行结束后补充。若当前产品测试仍失败，保留日志并停止产品改动；不能通过忽略身份验证错误、跳过用例或修改测试期望取得绿色。
