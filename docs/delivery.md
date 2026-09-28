# v0.1.2 交付说明

本说明对应 v0.1.2 正式 Release，产物版本为 `0.1.2`。正式身份由 `v0.1.2` tag 解引用的发布提交、[Release 资产](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.2)、BUILD.json 与公布的 SHA256 共同确定。完整源码 SHA 必须相互一致；main 最新源码或 RC 包不能冒充固定正式资产。

核验正式身份时交叉检查 tag/commit、随包 BUILD.json、`d2core version --json` 和 Release SHA256 清单。main 后续文档、授权与白名单维护不追溯改变现有包内容。

## 运行包内容

v0.1.2 Windows/Linux amd64 ZIP 采用明确白名单，清单如下；已发布 v0.1.1 的实际内容仍以原 Release 为准：

- d2core（Windows 为 .exe）、launcher-example、BUILD.json。
- README.md、RELEASE_NOTES.md、CHANGELOG.md（区分候选与历史发布）。
- THIRD_PARTY_NOTICES.md（固定工具链/实际依赖的许可证和再分发声明）。
- LICENSE、LICENSING.md：纳入本版本构建白名单。历史 v0.1.1 不重新打包、不覆盖、不重发；授权范围由当前 LICENSE 与 LICENSING.md 澄清。
- docs/delivery.md、operations.md、local-api.md、a2s.md。
- examples/README.md、两平台模板、examples/launcher/README.md 与 main.go（外部调用示例源码）。

Go client 库通过源码模块使用；不随运行包复制整个仓库。launcher-example 是集成演示，默认 30 秒后回收其房间，不是生产控制器。运行二进制无需 Go/Python。

不包含 CI、测试源码、M0 脚本/实验资料、阶段验收、review、路线图、游戏/VPK、凭据、玩家日志或本地数据。已编译 CLI 保留既有 m0-inspect 非生产诊断入口，以避免本轮改变命令行为；运行及集成不依赖它。

## 安装与核验

1. Windows 用 `Get-FileHash <ZIP> -Algorithm SHA256`；Linux 用 `sha256sum -c <ZIP>.sha256`。
2. 解压至全新可写 ASCII 目录；Linux 必要时 `chmod +x d2core launcher-example`。
3. 执行 `d2core version --json`：本版本产物 version=0.1.2，与 BUILD.json.version 一致；gitCommit、buildTime 一致，gitDirty=false。
4. 按[模板说明](../examples/README.md)准备游戏资源和真实规则，以同一普通用户按[操作说明](operations.md)运行 check、serve、create。所有调用显式使用相同绝对 data-dir。
5. 查询 operation/status 后实际进房，再 restart 重连，stop 并确认 reclaimed/stopped/cleanup=complete。保留校验值、版本、实例/操作 ID 和真实客户端结果。

check、编译成功及 ready 均不能单独代表玩家可进入房间。失败时保留现场，按操作说明显式回收。管理器退出不停止游戏。

## 发布与升级门槛

已发布的 v0.1.2 及历史版本均不从后续 main 重建、改名或覆盖原资产。未来 Runtime 版本须独立授权并使用新的 version/tag/Release；从已验收的远端固定完整提交建立干净 checkout，固定工具链，按本次授权版本构建。不复用开发产物，不覆盖历史 tag 或包。提交与 SHA256 按实际新产物填写到该版本发布记录，不预写虚构值。

发布前检查包白名单、文档链接、version/BUILD.json、双平台运行及校验清单。原 RC 烟测证据只说明候选结果；正式包验证另记。详细维护流程在源码仓库 docs/development.md，开发工具不随运行包分发。

更新核心前停止回收实例并备份必要历史；游戏/VPK/SDK 停服更新由运维执行。当前格式 2，不自动迁移格式 1；不手动修改校验或以新键掩盖未知操作结果。

## 正式产物的构建与独立核验

维护者从最终固定提交的全新干净 checkout 使用 Go 1.27.1：

```text
go run ./tools/package --go ABS_GO --version 0.1.2 --build-time RFC3339_UTC --output ABS_OUTPUT
python tools/package/verify.py ABS_OUTPUT --commit FULL_FINAL_SHA --version 0.1.2 --native --write-manifest
```

上述验证脚本只在源码仓库提供，运行用户无需 Python。正式工具为两个平台生成 ZIP 和各自 .sha256；验证器核对完整白名单、ZIP 解压/CRC、SHA256、BUILD 身份和模板规则，并只执行本机平台的 version --json。另一平台原生执行由对应 CI runner 验证，不以交叉编译代替。

BUILD.json 包含 version、gitCommit、gitDirty=false、构建/源码时间、Go 版本、os/arch 与兼容版本。最终源码 SHA 以实际构建记录为准，不预写 tag。公网模板必须同时满足地图/脚本与精确 Steam 规则，具体见[模板说明](../examples/README.md)。

打包门槛失败时不发布。发布必须使用通过验收的固定源码与对应产物；版本号本身不代表验收通过。不得重建后覆盖 v0.1.2 或更早版本的历史资产。

## 字节来源与可重复构建

发布版本的唯一机器配置入口为源码仓库 `tools/package/VERSION`（当前 `0.1.2`）；builder 默认读取它，显式 --version 不一致则拒绝。双平台 CI 从同一文件读取，验证器也从指定 commit 的 Git blob 核对版本。当前稳定版本为 v0.1.2；正式身份仍须与固定 tag 和公布摘要交叉核验。

`.gitattributes` 为受控文本固定 LF checkout。打包前核对**整个 tracked tree** 的原始文件字节与 HEAD blobs；这覆盖两个 Go 编译目标、构建元数据及全部静态分发输入。即使 git status clean，CRLF 转写、过滤器改写或缺失文件仍会失败。工具不会悄悄规范化；请从固定提交重新建立独立 clean checkout，避免与编辑器/其他构建并发修改。旧 clone 可能保留历史 CRLF 字节，即使 Git 显示 clean 也会被正确拒绝；此时优先在新目录 fresh clone 并 checkout 固定完整 SHA，保留旧目录中的工作，不绕过 gate 或静默 normalize。打包后再次核对 tracked 输入，并检查所有静态 ZIP entry 与固定 commit blob 完全相等。

单独指定相同 buildTime 不构成跨任意环境可重复性的保证。需要相同完整 SHA、精确 Git blob 输入、Go 1.27.1 工具链及模块依赖、目标 OS/amd64、构建选项和时间。工具固定 CGO_ENABLED=0、GOAMD64=v1，关闭额外 GOFLAGS/GOEXPERIMENT/Go workspace 设置，并使用 -mod=readonly。仅在受控环境下比较独立 clean checkout 构建的两个 ZIP SHA256；不要用行尾转换后的近似相等替代字节相等。

验证器需要包含候选 Git objects 的本地源码仓库；可用 `--repo ABS_REPOSITORY` 指定，不要求该仓库的 worktree 字节用于核验。`--write-manifest` 仅在检查通过后以独占创建方式生成 RELEASE-MANIFEST.json 与 SHA256SUMS，字段来自实际 ZIP。后续省略该选项会重新检查已有 Manifest 的 version/commit/buildTime、文件名/长度/摘要/entry list 和 BUILD 身份；不允许手工漂移。

每个 .zip.sha256 sidecar 是同次产物的 **自身一致性** 检查，不是独立真实性证明。外部身份锚点是 Owner 或可信 GitHub Release 渠道公布的 SHA256；独立审查时可重复传入 `--expected-sha256 文件名=64位小写摘要`，须提供两个 ZIP 的已知摘要。不要从待检 ZIP 重新计算“预期值”再声称验证了外部身份。无需新增签名设施。

本版本随交付记录保留实际 buildTime、完整 SHA、两次独立构建比较结果和机器生成的 Manifest。源码来源检查不等于第三方模块或编译器供应链的密码学证明，也不声称本轮重新进行了真实 Dota 验收。
