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

首轮实际运行：[35553437892](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35553437892)，CI 提交 `9f17644f93ea9185429eb31cbc6aeda3f060f25d`。

- Windows current：test/vet/build/race 全部通过。
- Windows v0.1.0：test 失败，vet/build/race 通过。`TestReliabilityStopQueuedRestartNoNewGeneration` 在 reliability_test.go:107 收到 `BUSY: instance operation is running`，当时 create operation 已为 succeeded。这不是安装/缓存失败。
- Linux 两项：准备失败，尚未执行测试。原 runner UID 1001 的只读诊断确认 systemd 与 (sd-pam) 的 exe 不可读；切换到 nobody 后，又因 runner 工作目录父路径不可遍历导致诊断脚本打不开。后者为本轮 CI 脚本路径缺陷，已在 `800b4c52e4eb85b86a61c8554b08df35b27a5c42` 改用独立 `/tmp` 目录并复制脚本，不放宽工作目录权限。

Windows 失败的源码依据：Manager.finish 先持久化 operation 完成；defer 调用 finishWorker 后才移除 workers 条目；change(restart) 在 workers 条目仍存在时返回 BUSY。waitOperation 只等待 operation 结束，因此可能遇到这一窗口。main 与 v0.1.0 的产品和测试源码相同，一份通过不能排除另一份暴露的时序问题。此问题需要另行确定产品完成语义/测试同步要求；本轮不修改产品或测试，不重试吞错、不通过延时或过滤用例隐藏失败。对 v0.1.0 的已知影响是操作完成后立即 restart 可能短暂收到 BUSY；本次未出现误杀、数据损坏或协议格式变化的证据，不据此推断其他场景全部安全。

目录修复后的完整运行：[35553772566](https://github.com/L4C99/dota2-arcade-dedicated-core/actions/runs/35553772566)。最终结果见该运行各 job 和保留日志；一次通过不消除上述已捕获问题。

原始失败日志保存在运行日志及各 job 的 ci-* artifact（14 天）；本地审计副本放在忽略的 local/ci-audit，不把一次性 runner 诊断文件提交版本库。历史红色运行保留，不重写正式 tag 或发布包。若需修复产品才能稳定通过，应另行授权处理；本轮暂停在报告，不改核心。
