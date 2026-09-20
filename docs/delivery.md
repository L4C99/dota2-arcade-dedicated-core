# 双平台交付包

M0—M4既定范围已验收。交付包用于独立部署；迁移或接管现有生产房间须另行确认。目标为已测试的Windows 10 x64、Ubuntu 24 x64；包中不包含游戏、地图、运行库或凭据，不提供许可证。

每个平台ZIP包含d2core、launcher-example、BUILD.json、平台模板、协议和操作说明；附独立.sha256校验文件。运行两个可执行文件不需要Go、Python或PowerShell。M0实验说明仅供参考，实验脚本可在源码仓库取得，不是产品运行依赖。

## 使用者步骤

1. 核对SHA256后解压到可写的ASCII路径，例如Windows的C:/d2core-test或Linux用户目录下d2core-test；Linux确认d2core与launcher-example可执行。运行 `d2core version --json`，核对BUILD.json中的提交和构建时间，gitDirty应为false。
2. 准备Dota专服安装、依赖和用户地图VPK，保证客户端解析同一addon标识。核心不会下载或更新共享资源。复制对应examples模板到自己的配置目录，填写绝对路径及实测就绪规则。
3. 以同一普通用户运行 `d2core check --template <绝对模板> --json`；通过只代表静态检查，不代表能进房。
4. 选择全新data-dir并运行 `d2core serve --data-dir <绝对目录> --port-min <已预留起点> --port-max <终点> --json`。不要将生产目录、旧版本数据或其他用户的目录作为试用位置，不需要开放管理API的TCP端口。
5. 在另一终端运行create并查询operation/status，客户端实际进房；再执行restart、再次进房、stop，确认历史可查及回收。完整命令见operations.md，也可运行launcher-example演示受限本地协议流程；示例默认30秒后停止自己的房间。
6. 同一data-dir只能有一个管理端。管理端退出不结束游戏；错误时保留实例ID、创建键和现场，先核验再stop，不按通用进程名杀进程。运行包更新前结束并回收实例，备份需要的历史；游戏/VPK更新由运维安排停服后完成，不由核心执行。

A2S是显式独立工具，见a2s.md。仅在隔离配置上验证，不能将同机客户端安装或生产gameinfo当作可随意修改的测试文件。磁盘告警、离线日志边界及历史期限见operations.md和local-api.md。

## 开发者构建

固定Go 1.27.1，使用干净Git工作树，在仓库根执行：

```text
go run ./tools/package --go <Go绝对路径> --output dist --version 0.1.0-rc.2 --build-time <本次构建UTC时间，RFC3339>
```

build-time可省略并使用实际当前时间；复现同一包时必须传入原BUILD.json的buildTime。sourceTime与buildTime分别记录提交时间和构建时间。程序拒绝脏工作树，使用固定文件清单、GOARCH=amd64、CGO_ENABLED=0、trimpath、稳定归档顺序和时间；不包含local、data、VPK、私钥或原始日志。已存在的输出不覆盖。构建/归档失败可能留下无校验文件的不完整产物，应核对后清理或改用新的输出目录。

交叉构建只是生成文件，不能代替目标平台实机运行。CI只做辅助测试与构建；不自动连接测试主机、不运行Dota、不创建GitHub Release。M0—M4真实证据见validation目录。GitHub源码推送、交付包生成与GitHub Release发布分别记录，当前未创建GitHub Release。

## v0.1.0-rc.1 交付与产物烟测

该RC保持已独立复核的1a0b79c6c61c87be87fbb7881376bf031bd7b464产品代码，仅补充验收和运维文档。先推送文档提交与RC tag，再从远端克隆、detach到tag指向的完整提交，并以干净工作树运行上述tools/package流程；不得复用开发目录二进制。生成的提交命名ZIP可复制为 `d2core-v0.1.0-rc.1-<windows|linux>-amd64.zip`，内容不重打包，并重新为最终文件名计算校验清单。

包内BUILD.json记录真实构建提交、UTC时间、Go1.27.1及目标平台。现有程序version.version固定为0.1.0-dev，本阶段禁止产品代码修改，因此不伪改此字段；RC版本以远端v0.1.0-rc.1 tag和随包RC-MANIFEST.json为准。核验程序gitCommit等于tag指向提交和BUILD.json.gitCommit、gitDirty=false、buildTime一致。BUILD.json.status沿用工具的local build标记，不能视为RC产物已通过真实环境烟测。原有代码实机验收不替代新ZIP烟测。

发布产物烟测由使用者执行/确认。2026-09-21 已收到 Windows 与 Linux 本轮单房间烟测确认，范围、失败准备过程和最终回收证据见 [RC.1 烟测](validation/rc1-smoke.md)。以下为复验步骤，不创建正式生产 Release。此段为包外勘误，原 RC.1 包和校验清单不覆盖：

1. Windows执行 `Get-FileHash <ZIP> -Algorithm SHA256`，Linux执行 `sha256sum -c <ZIP>.sha256`；与随包校验值一致后解压到全新ASCII可写目录，Linux必要时 `chmod +x d2core launcher-example`。
2. 运行 `d2core version --json` 并按上文核对版本元数据。准备独立模板和全新data-dir，勿接管生产或旧data-dir；使用包中examples平台模板、docs/local-api.md及examples/launcher/README.md。
3. 同一普通用户执行check，然后serve；另一终端create（唯一键），轮询operation/status，核对本代ready与Steam认证，客户端实际进房。
4. restart原实例，核对旧代退出、新代递增，再次进房。保持玩家在房间，退出管理器并确认游戏正常；使用原用户、data-dir及Linux运行库环境恢复serve，核对原实例状态，不重新create。最后stop后核对operation成功、reclaimed/stopped/cleanup=complete，日志和历史仍可查。failed时按operations.md显式回收，不换键掩盖失败。
5. 记录ZIP SHA256、version JSON、操作/实例ID和客户端结果。按需另用launcher-example演示同用户外部协议流程；示例默认30秒后停止自己创建的房间。没有Go/Python运行依赖，不开放TCP管理API。
6. 向交付方确认Windows/Linux结果或提交失败片段。未收到真实环境烟测确认前，不把该RC标成生产Release或宣称产物实机通过。已知限制与旧cfg/FIFO处理见operations.md。

## RC.2 与平台集成

用户已授权帮助与版本标识修正。打包工具 --version 接受 X.Y.Z、X.Y.Z-dev 或 X.Y.Z-rc.N；指定候选版本后写入程序version.version及BUILD.json.version，并生成对应版本文件名。不指定仍为开发标识。生产发布需要最终候选确认，不因构建成功自动发布。命令验证范围见 [验收矩阵](validation/rc2-commands.md)。平台开发可使用协议v1和client库，操作受理不等于完成，应保留创建key和operationId并轮询。

RC.2从远端干净提交新构建，ZIP中含本轮勘误和验收范围。产物命令矩阵结果另随包提供，最后真实进房确认仍由使用者进行。不要覆盖RC.1或将RC.1的真实进房记录冒充RC.2测试结果。
