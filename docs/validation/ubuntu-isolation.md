# Ubuntu M0 隔离约定

仅推进 M0；完成后创建用户授权的公开仓库 `L4C99/dota2-arcade-dedicated-core`，不添加许可证，不继续 M1。

## 连接与资源

本机 `local/ssh/d2core_config` 使用项目专用别名 `d2core-m0-coreyun`，单独复制 known_hosts；IdentityFile 引用用户已有私钥，不复制或读取其内容。严格主机密钥校验、禁用转发、BatchMode。地址、SSH 端口和凭据路径只在忽略目录保留，主机密钥变化必须由用户核验。

实际系统 Ubuntu 24.04.5 / Linux 6.8.0-139 / x86_64，systemd 255，Python 3.12，cgroup v2。初次只读观测：8 个逻辑 CPU、约 14 GB 可用内存、42 GB 空闲磁盘，6 个现有游戏进程。数字会变化，启动前复核；现有管理系统会自行创建/回收房间，不把 PID 变化当作接管许可。

引擎 6933 / SourceRevision 10999813，与 Windows 相同。整个项目使用用户在本地提供的 OMG AI 4+3 VPK；上传到 `/srv/d2core-m0/assets/3564393242.vpk` 的 481987687 字节文件已校验 SHA256 `27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c`。不使用云端现用地图作为测试资源。上传限速约 2 MiB/s。

## 不可修改对象

- `/srv/dota/server`、`/srv/dota/steamcmd`、`/srv/dota/map-versions`、正式 dota_addons。
- `/srv/dota/room1-mode-test`、`/usr/local/lib/dota-room-share`、`/var/lib/dota-room-share`、`/run/dota-room-presence`、现有 roomctl 锁。
- 所有 `dota-room-*` 单元及网页/API/在线监测/自动关房服务；无人、停止、failed 均不构成授权。
- 主机/SSH/防火墙/NAT/系统级限制；不更新安装、不执行 SteamCMD、不全局杀进程、不升级主机、不做压测。

## 本项目允许的实验范围

- 唯一根目录 `/srv/d2core-m0`，只控制 `d2core-m0-*` 临时单元。
- 通过该单元的私有挂载命名空间只读使用现有引擎；`engine-cfg` 覆盖该命名空间中的 cfg，私有 addons 覆盖该命名空间中的 dota_addons。宿主及其他服务看到的目录保持不变。
- `ProtectSystem=strict`、仅本项目目录可写、PrivateTmp、NoNewPrivileges、独立 HOME。首次隔离探针实测确认安装文件不可写、cfg 标记落在本项目目录，宿主正式 cfg 不出现标记，探针退出后已删除标记。
- 使用既有非 root 用户 dota 运行引擎，root 仅用于建立本项目隔离单元与读取身份；不把 root 登录当作修改全机的许可。
- 一次仅一个测试服；计划单元上限 CPUQuota=150%、MemoryMax=4G、MemorySwapMax=0、TasksMax=128，Nice=10、IOWeight=10。这些只作用于新单元，绝不更改正式服务或系统级限制。达限应报告实验失败，不扩大负载。
- 引擎日志、stdout/stderr、cfg、输入与身份分别保存；工具退出不依赖 SSH 会话继续提供日志管道。

## 端口

用户明确正式 UDP 27117–27127 和现有网页/API 端口受保护；新预留公网 UDP 40000–40002，域名连接，不固定 IP。用户已确认公网到内网逐一同号映射。主机端已检查监听及相关 service/socket/房间配置，未发现冲突预留引用；不擅自修改映射。

该区间位于机器当前临时端口范围内；每次实验仍需检查实际绑定及抢占，不能因为配置未命中就保证可用。不修改系统临时端口范围。公网只确认 UDP，不把 TCP 可达性写成事实。

## 已执行与未执行

- 已执行 Ubuntu 本机 Go 测试二进制，路径/指纹测试、正常及直接异常退出父进程后测试子进程继续写日志均通过；属于测试进程证据，不代替 Dota。
- Python Linux 探针已运行真实引擎。身份采用 boot ID、启动 ticks、可执行路径、完整参数、项目 cgroup，并在操作前打开 pidfd，信号通过 pidfd 发送。
- 初次隔离探针执行通过，但采集脚本末尾 stat 因 Windows 管道末尾 CR 失败；后续单独只读 stat 已通过。未掩盖初次非零退出。
- `iptables -t nat -S` 返回无该表/链，不能据此宣称没有云端 NAT；公网映射以用户面板信息为准。
- 本项目 Ubuntu 引擎两次真实进房、重开、停止及故障验证已完成；最终结论见 [M0 验收](m0-results.md)。

## 真实引擎实验（2026-09-18，时间 UTC）

| 运行 | 结果 |
| --- | --- |
| run01 | 失败：私有 cfg 遮住引擎自带 user_keys_default 配置，主进程 signal 11。保留失败日志，不能算地图加载通过 |
| run02 | 复制 10 个明确的引擎默认 .vcfg 到私有目录后可启动；addon 目录中放 pak01.vpk 未成功挂载 n7。FIFO quit 实测 177 ms 退出，systemd Result=success/MainPID=0 |
| run03 | 本地 VPK 顺序解包，5131 文件、481770493 字节、每文件 CRC 通过。n7 ss_active，但 addon_game_mode 主脚本失败，不能算地图就绪。quit 实测 502 ms 退出 |
| run04 | 使用原始 VPK 绝对路径，服务端脚本与 VAC 就绪；Windows 客户端收到 Linux 绝对路径后报路径警告并因重复技能 ID 崩溃，未 FULL，不能算进房通过 |
| run05 | 改为两端唯一同名原始 VPK，10:08:07 FULL 且用户确认进入；错误 boot ID 的 quit 被拒绝，恢复记录后正常 quit 553 ms |
| run06 | 同配置重开；启动工具在保存进程身份前 Exit(73)，引擎继续启动；重开探针核验身份后 10:10:03 FULL 且用户确认；quit 556 ms，cgroup 空 |
| run07 | 注入不可写日志，引擎仍存活但无引擎日志；暂停目标进程后 quit 10 秒明确超时，再 pidfd 强杀，确认 signal 9 / cgroup 空 |
| run08/run10 | 中文空格/纯中文日志目录可写，但引擎没有生成日志；失败如实保留 |
| run09 | ASCII 空格日志正常，无效地图的加载失败与 Idle 有明确日志；正常退出 |
| run11 | 修正探针后以实际引擎用户预开不可写日志，明确拒绝，未 exec 引擎 |
| run12 | 最终修订探针正常加载原始 VPK，脚本/VAC 就绪；采集全部 TCP/UDP、无直接子进程；quit 496 ms，归属套接字消失 |

解包实验仅发生在本项目 addons 目录，未复制云端现用地图，也未修补地图代码。直接 VPK 与解包加载表现不同，现有证据不能确定主脚本失败的具体原因。run04 使用原始 VPK，保持用户提供资源字节不变。

从 run02 起设置 LimitCORE=0；没有更改主机 core 策略。默认 .vcfg 文件名单和哈希保存在私有 default-cfg-snapshot.json；未复制生产房间 cfg 或 autoexec。run04 观测到游戏端口同时使用 UDP/TCP，以及额外的回环 TCP 临时监听；公网目前仅确认 UDP 映射。

所有测试单元最终 MainPID=0；失败单元保留 failed 状态作为诊断，不通过 reset-failed 掩盖。私有生成 cfg 按归属与归档内容一致性清理，FIFO 在单元退出后清理；原始 VPK、默认配置副本和运行证据保留。测试端口无绑定，证据打包下载到本机忽略目录；线上服务未停止、未重配。
