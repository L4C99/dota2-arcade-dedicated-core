# M0 环境基线

记录日期：2026-09-18。真实路径、主机地址、凭据及未脱敏输出保存在忽略的 `local/`，不入库。

| 项目 | Windows 当前观测 | Ubuntu |
| --- | --- | --- |
| 系统 | Windows 10 Pro 22H2，10.0.19045，UBR 6466，amd64；来自注册表及 RuntimeInformation | Ubuntu 24.04.5，Linux 6.8.0-139，amd64，systemd 255，cgroup v2 |
| Go | Go 1.27.1，官方包 SHA256 校验；本机 test/vet/build 通过 | 同工具链交叉构建后在目标机执行测试通过；不是仅编译 |
| Git | 已初始化 main；已有本机作者配置；基线提交 3bdd52b | 不适用 |
| 游戏程序 / 工作目录 / cfg | 用户已提供 Steam 安装；同一 dota2.exe 用作专服和客户端；工作目录 game，cfg 位于 game/dota/cfg | linuxsteamrt64/dota2；工作目录 game；只读共享引擎，私有 cfg/addons 挂载 |
| 游戏版本 / 安装改动 | steam.inf: Client/ServerVersion 6933，SourceRevision 10999813，Sep 15 2026；确认 gameinfo 历史新增 GMS/Advertise，本轮备份后还原修改前文件，详见 M0 对照记录 | 同版本；gameinfo 未改，哈希与 Windows 无 Advertise 对照文件相同 |
| 地图 / VPK / SHA256 | OMG AI 4+3，3564393242.vpk，481987687 字节；SHA256 `27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c`；包内 addoninfo 声明 dota/n6/n7/rd/abyss/custom，本轮 n7 | 上传本地同一文件，进程打开文件和进程命名空间内哈希均一致；不用云端现用地图 |
| Steam / 登录 / 运行库 / 权限 | 引擎加载和默认 VAC 进房通过；唯一 cfg 写入成功；未做运行库逐个移除实验 | dota 用户运行，独立 HOME；只读使用现有 steamclient.so 与运行库，默认 VAC 进房通过 |
| 客户端 / 网络路径 | 用户本地客户端通过 127.0.0.1 连接；run02/run03 均实际进房确认 | 同一 Windows 客户端经域名和 NAT UDP 40000 连入；run05/run06 均 FULL 且用户确认 |
| 测试目录 / 端口 | 用户确认本地无专服；项目 local/validation/windows，27015/27016 | /srv/d2core-m0；用户预留 UDP 40000–40002 公私网同号；每次核验占用 |
| 保护对象 | 未授权安装和共享文件不覆盖，玩家客户端自行控制 | 线上全部 dota-room-*、安装、地图版本、网页/API/监测、网络设置均受保护；未修改 |

首次 `Get-CimInstance Win32_OperatingSystem` 返回拒绝访问；改用只读注册表与 RuntimeInformation 取得上表系统信息，不将 CIM 查询记为成功。默认 Go 缓存访问失败后，改用项目 `local/go-cache`、`local/go-tmp`，测试与构建成功。

## 当前阶段必须补齐的输入

双平台输入均已由用户补齐，实际进房均已确认。以下清单保留用于后续复验，不表示仍缺少输入。

1. 测试地图名称、原始 VPK 绝对路径；Windows 和 Ubuntu 是否使用同一资源，客户端需要哪些资源。
2. Windows 程序、工作目录、cfg 目录、客户端位置；允许测试的安装及禁止触碰的在用专服；独立可写目录和可用端口。
3. Ubuntu 的访问方式（引用已配置 SSH 别名/密钥位置，不在对话或 Git 保存密钥）、具体系统版本、游戏/VPK 路径、可写目录、可用端口及是否有 sudo 权限。
4. 谁实际执行客户端进房，目标地址及经过的防火墙/NAT；连接结果须留证，日志命中不能替代。

GitHub 已授权 L4C99/dota2-arcade-dedicated-core，公开，暂不添加许可证；M0 后上传。

## 实机隔离记录（每次运行前填写）

- 平台、授权安装、工作目录、cfg 目录、资源原始 SHA256、已存在的安装改动。
- 允许监听的地址和游戏端口；附加端口未知时先只读记录现有监听，并评估共享主机冲突后启动。
- 唯一实验 ID、专用 evidence 目录、唯一 cfg 文件名；禁止覆盖既有 cfg。
- 当前在用专服的保护清单；停止仅限本次创建且身份已核验的进程，不按进程名批量终止。
- 预期写入和清理文件列表；原始 VPK、模板、安装目录不属于删除范围。
- 实际监听不能按 `-ip` 推断；需要网络限制时先确认防火墙隔离，不擅自更改全局规则。

未填完的运行不得开始。收到 Windows 输入后已进行首轮 Dota 启动：仅新增唯一 cfg `d2core_m0_20260918_run01.cfg`，未修改 gameinfo 或防火墙。原始输入和完整身份见 `local/validation/windows/run01/`。用户确认没有在用专服；未终止客户端或其他进程。

后续为排查安全握手失败，备份并还原历史修改前 gameinfo，完成 run02/run03 两次实际进房。三轮测试专服现已退出、唯一 cfg 已清理；客户端仍由用户自行控制。当前 gameinfo 保留无 Advertise 版本，修改前后文件与操作记录均在 `local/validation/windows/gameinfo-control/`。防火墙未改动。

实测 `-ip 127.0.0.1` 时 TCP 游戏端口为回环，但 UDP 27015 监听 IPv4/IPv6 全地址，不能声称网络仅限回环。该结果已告知用户。引擎自身可能向 Steam/地图服务连接，端口记录保留在忽略目录。

正常运维部署为异地专服，玩家客户端与服务器安装分离（用户已明确）。Windows 共用安装是本轮测试条件，相关 Advertise/VAC 注意事项不作为异地部署的普遍限制。

跨机测试最终采用两端专用名称 d2core_m0_3564393242.vpk；Windows 新增该文件副本，未覆盖历史同 ID 目录或创意工坊文件。双方原始文件哈希一致。中文玩家名已分别在两平台日志中完整核验；用户同意引擎日志保存路径限 ASCII（允许空格），不自动选择替代目录。失败对照及具体限制见 [M0 结论](m0-results.md)。
