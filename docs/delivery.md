# v0.1.1 交付说明

本文对应 v0.1.1；正式资产与最终检查记录以 GitHub Releases 为准。普通用户使用 Release 二进制。不要将 RC.2 包改名作为 v0.1.1。

## 运行包内容

Windows/Linux amd64 ZIP 采用明确白名单，仅包含：

- d2core（Windows 为 .exe）、launcher-example、BUILD.json。
- README.md、RELEASE_NOTES.md。
- docs/delivery.md、operations.md、local-api.md、a2s.md。
- examples/README.md、两平台模板、examples/launcher/README.md 与 main.go（外部调用示例源码）。

Go client 库通过源码模块使用；不随运行包复制整个仓库。launcher-example 是集成演示，默认 30 秒后回收其房间，不是生产控制器。运行二进制无需 Go/Python。

不包含 CI、测试源码、M0 脚本/实验资料、阶段验收、review、路线图、游戏/VPK、凭据、玩家日志或本地数据。已编译 CLI 保留既有 m0-inspect 非生产诊断入口，以避免本轮改变命令行为；运行及集成不依赖它。

## 安装与核验

1. Windows 用 `Get-FileHash <ZIP> -Algorithm SHA256`；Linux 用 `sha256sum -c <ZIP>.sha256`。
2. 解压至全新可写 ASCII 目录；Linux 必要时 `chmod +x d2core launcher-example`。
3. 执行 `d2core version --json`：正式包 version=0.1.1，与 BUILD.json.version 一致；gitCommit、buildTime 一致，gitDirty=false。
4. 按[模板说明](../examples/README.md)准备游戏资源和真实规则，以同一普通用户按[操作说明](operations.md)运行 check、serve、create。所有调用显式使用相同绝对 data-dir。
5. 查询 operation/status 后实际进房，再 restart 重连，stop 并确认 reclaimed/stopped/cleanup=complete。保留校验值、版本、实例/操作 ID 和真实客户端结果。

check、编译成功及 ready 均不能单独代表玩家可进入房间。失败时保留现场，按操作说明显式回收。管理器退出不停止游戏。

## 发布与升级门槛

维护者从远端固定完整提交的新干净 checkout 构建，固定 Go 1.27.1，执行打包工具 --version 0.1.1；版本标签为 v0.1.1。不复用开发产物、不覆盖旧 tag 或包。提交与 SHA256 在最终发布时填写到发布记录，不能预写虚构值。

发布前检查包白名单、文档链接、version/BUILD.json、双平台运行及校验清单。原 RC 烟测证据只说明候选结果；正式包验证另记。详细维护流程在源码仓库 docs/development.md，开发工具不随运行包分发。

更新核心前停止回收实例并备份必要历史；游戏/VPK/SDK 停服更新由运维执行。当前格式 2，不自动迁移格式 1；不手动修改校验或以新键掩盖未知操作结果。
