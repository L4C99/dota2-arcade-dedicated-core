# Dota 2 游廊专服核心

面向启动器和节点平台的本机 Dota 2 专服管理核心。它管理游戏进程、实例记录和本地调用，提供 Windows/Linux CLI 与同用户本地 API。

它不是游廊平台、匹配服务、远程控制面板或游戏资源下载器；不会分发地图、配置 NAT、防火墙或自动升级游戏。

当前稳定版本：[v0.1.1](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1)。当前仓库源码及文档采用 [MIT License](LICENSE)，公开供社区研究、使用、fork 和二次开发。

## 维护状态与项目关系

本项目主要源于作者实际 Dota 2 Arcade Dedicated Server 部署需求，并公开作为社区研究和参考资料。维护以作者实际需求和空闲时间为准，不承诺持续开发、功能请求实现或技术支持；Issues 按 best-effort 处理。

这是独立社区项目，与 Valve Corporation 无隶属或官方认可关系。Dota 2、Steam 及相关商标归各自权利人所有。仓库及 Release 不包含 Dota 2、Steam、Workshop VPK 或其他游戏资产。

## v0.1.1 能做什么

- 创建、查询、重启、停止并显式回收实例；保留操作结果和历史日志。
- 自动分配指定范围内的游戏端口，也可显式指定端口；支持多个实例。
- 创建请求幂等、异步操作查询、管理器恢复与进程身份校验。
- 配置历史保留期限和最低可用空间；通过本地 API 接入外部程序。
- 提供独立的 `a2s enable` 配置工具；它会修改游戏配置，使用前阅读说明。

## 支持与已验证环境

目标平台为 Windows/Linux amd64；真实测试环境为 Windows 10 x64、Ubuntu 24 x64。v0.1.1 发布前已使用 n6 地图（工坊 ID `3564393242`）完成 Windows 10 x64 与 Ubuntu 24 x64 的创建进房、重启重连、停止回收真实烟测。服务端使用经验证的联机修改版 VPK，客户端订阅资源须与其兼容。这不代表所有地图、Linux 发行版或容量规模均已验证。

核心二进制不要求安装 Go/Python；Dota 服务端、运行库、Steam SDK 和地图需自行准备。管理器与 CLI 使用同一普通系统用户。Linux 环境准备见[操作说明](docs/operations.md)。

## 下载与快速开始

普通用户优先从 [GitHub Releases](https://github.com/L4C99/dota2-arcade-dedicated-core/releases) 下载对应平台二进制。v0.1.1 正式资产以该页面及校验清单为准；不要把 RC 包改名当作正式版。v0.1.1 文件名、版本核验和发布门槛见[交付说明](docs/delivery.md)与[Release Notes](RELEASE_NOTES.md)。

1. 校验 ZIP 的 SHA256，解压到全新可写 ASCII 路径；Linux 确认 `d2core` 和 `launcher-example` 有执行权限。
2. 运行 `d2core version --json`，核对版本、提交、构建时间与 `BUILD.json`，`gitDirty` 应为 false。
3. 复制 `examples/template.windows.json` 或 `examples/template.linux.json`，填入实际绝对路径和实测就绪规则。模板里的占位规则不能直接用于开服。
4. 使用下面的 CLI 流程启动管理器并创建房间。完整可复制的双平台命令见[操作说明](docs/operations.md)。

## 正式 CLI 基本用法

下面 `ABS_*`、`KEY`、`INSTANCE` 和 `OPERATION` 都需替换；Windows 当前目录执行时使用 `.\d2core.exe`，Linux 使用 `./d2core`。

```text
d2core check --template ABS_TEMPLATE --json
d2core serve --data-dir ABS_DATA --port-min 27015 --port-max 27064 --json
```

保持 serve 终端运行，在另一终端使用相同用户、相同 data-dir：

```text
d2core create --template ABS_TEMPLATE --idempotency-key KEY --data-dir ABS_DATA --json
d2core operation OPERATION --data-dir ABS_DATA --json
d2core status INSTANCE --data-dir ABS_DATA --json
d2core list --data-dir ABS_DATA --json
d2core logs INSTANCE --tail 100 --data-dir ABS_DATA --json
d2core restart INSTANCE --data-dir ABS_DATA --json
d2core stop INSTANCE --data-dir ABS_DATA --json
```

`KEY` 标识一次创建意图：同一请求重试沿用原键与参数，新房间使用新键。ID 从响应读取。create/restart/stop 返回受理不等于完成，必须轮询各自的 operation；ready 还需实际客户端进房验证。stop 成功应确认 `reclaimed / stopped / cleanup=complete`。failed 实例也需显式 stop。

自动端口默认 27015–27064；create 可加 `--port 27016`，restart 沿用原端口。管理器退出不会停止游戏。全部参数、帮助、A2S 和错误处置见[操作说明](docs/operations.md)；运行 `d2core help` 或 `d2core help create` 查看帮助。

## 配置模板

模板 v1 定义可执行文件、工作目录、参数、独立 cfg、日志就绪规则和超时。默认推荐数字工坊 ID：服务端布局为 `game/dota_addons/数字ID/pak01_dir.vpk`，地图命令使用 `customgamemode="数字ID"`。核心不下载 VPK，不保证任意服务端修改版与客户端资源兼容。

check 只做静态检查。创建时保存配置快照，编辑原模板不会改变已创建实例；需要新配置时回收旧实例，以新键创建。详见[模板说明](examples/README.md)。

## 本地 API 集成

协议 v1 使用 Windows 命名管道或 Linux Unix socket，只允许同用户本地访问，没有 TCP 管理 API。平台应在节点上运行本地调用程序，保留幂等键、instanceId 与 operationId，处理结果未知和异步完成。

参见[协议契约](docs/local-api.md)和[外部调用示例](examples/launcher/README.md)。源码仓库的 `client` 是 Go 客户端库。随包 `launcher-example` 演示完整流程，默认 **30 秒后停止自己创建的房间**，不是常驻生产控制器。

## 运维与升级

升级核心前停止并回收实例，备份需要的历史，核验新包后切换；保留原用户、data-dir 和 Linux 运行环境。当前磁盘格式为 2，不自动迁移格式 1 实验记录。游戏、地图和 SDK 更新需安排停服，不由核心执行。不要手改状态校验、按通用进程名杀进程或用新键掩盖失败。详见[运维说明](docs/operations.md)。

## 已知限制

- 只支持本机同用户管理，管理员/root 的系统权限不是隔离边界；不提供跨主机调度和远程认证。
- 路径采用 ASCII，可含空格；中文日志内容可记录。客户端必须解析兼容的 addon 标识。
- 就绪依赖模板中已验证的日志规则；Steam 登录和真实连接仍需验证。
- IPv6 通配监听的双栈覆盖未验证；不承诺任意网络环境、地图兼容性或容量。
- 日志读取有大小限制，历史及创建键受保留期限约束；资源更新、监控告警和备份由运维或上层平台负责。

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [Release Notes](RELEASE_NOTES.md) | v0.1.1 范围、版本与发布状态 |
| [交付说明](docs/delivery.md) | 包内容、校验、正式发布验收 |
| [操作说明](docs/operations.md) | 完整 CLI、端口、恢复、升级与失败回收 |
| [模板说明](examples/README.md) | Windows/Linux 模板和资源布局 |
| [本地 API](docs/local-api.md) | 请求、错误、状态和幂等契约 |
| [调用示例](examples/launcher/README.md) | 外部程序集成流程 |
| [A2S](docs/a2s.md) | 独立配置工具及边界 |

## 开发与源码构建（源码仓库）

普通部署无需执行本节。固定 Go 1.27.1，源码构建默认标识为 `0.1.1-dev`；正式版本由打包工具注入，不通过修改核心逻辑实现。

```sh
go test ./...
go vet ./...
go build ./cmd/d2core
```

测试和 CI 是开发验证，不等于真实游戏验收。开发/验证工具为非生产用途，M0 探针不能用于生产房间管理。当前源码仓库的开发/验证资料见 [开发说明](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/docs/development.md)、[阶段证据索引](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/docs/validation/README.md)；这些资料不随运行包交付。
