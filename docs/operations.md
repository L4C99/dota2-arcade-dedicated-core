# 本地核心操作

真实引擎验收状态见 [M1 记录](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/docs/validation/m1.md)。M0—M4既定范围已完成验收；接管现有生产房间仍须单独部署授权。游戏、运行库和两端匹配的地图资源由使用者准备；核心不下载或更新它们。

默认使用数字工坊 ID，资源布局与 n6 规则见 [模板说明](../examples/README.md)。本说明面向 v0.1.0；历史候选差异仅供旧包排障参考。

## 完整命令与参数索引

正式运行使用下表的管理命令及独立 A2S 工具。m0-inspect 仅为兼容保留的M0 历史诊断入口（非生产用途），部署无需使用。

本表按 v0.1.0 发布准备版本的 `cmd/d2core/main.go` 注册项核对；RC.1 与 RC.2 的公开业务命令一致，帮助行为差异见下文。ABS 表示本平台绝对路径；INSTANCE 是 i_ 开头实例 ID，OPERATION 是 o_ 开头操作 ID。尖括号不是命令的一部分。

| 命令 | 必填参数/位置参数 | 可选参数及默认值 | 用途 |
| --- | --- | --- | --- |
| `version` | 无 | `--json` | 查看提交、构建时间及协议/格式版本 |
| `check` | `--template ABS` | `--json` | 静态校验模板，不启动游戏 |
| `serve` | 无 | `--data-dir ABS`、`--port-min 27015`、`--port-max 27064`、`--history-days 7`、`--min-free-mib 1024`、`--json` | 前台运行本地管理器 |
| `create` | `--template ABS --idempotency-key KEY` | `--data-dir ABS`、`--port 0`、`--json` | 受理新实例；0表示自动端口 |
| `list` | 无 | `--data-dir ABS`、`--json` | 活动及失败待回收实例、存储观察，不列已回收历史 |
| `status` | `INSTANCE` | `--data-dir ABS`、`--json` | 实例当前状态，也可查尚未过期的已回收实例 |
| `operation` | `OPERATION` | `--data-dir ABS`、`--json` | 查询异步操作结果 |
| `logs` | `INSTANCE` | `--data-dir ABS`、`--tail 100`、`--generation 0`、`--json` | 查看引擎日志，0表示当前/最后一代 |
| `restart` | `INSTANCE` | `--data-dir ABS`、`--json` | 沿用配置快照和端口重启，返回新操作 ID |
| `stop` | `INSTANCE` | `--data-dir ABS`、`--json` | 停止并回收，也用于 failed 实例显式清理 |
| `a2s enable` | `--dota-dir ABS` | `--json` | 显式修改 gameinfo 的 Advertise；详见 [A2S说明](a2s.md) |
| `m0-inspect` | 无 | `--executable ABS`、`--working-directory ABS`、`--cfg-directory ABS`、`--vpk ABS`、`--gameinfo ABS`、`--version-file ABS` | M0 历史诊断，非生产用途；不供正式部署或外部集成依赖 |

除 m0-inspect 外，上述公开命令支持 --json，结构化输出也是默认行为。m0-inspect 自带 JSON 输出，但**不接受 --json**；不要将该参数无差别加到所有命令。manage 类命令的 data-dir 默认为可执行文件旁的 data，建议每次显式传入以免连错管理器。check/version/a2s/m0-inspect 不需要运行中的管理器；其他客户端管理命令需要 serve 在线。

`serve --history-days` 合法范围1–3650，控制成功回收后的历史及创建键保留期限；不清理活动房间。`--min-free-mib` 合法范围1–1048576，默认1024 MiB，用于新创建/重启的空间门槛，不因磁盘不足自动杀死现有游戏。缩短历史期限前先备份所需证据，详见下文保留策略。

`logs --tail` 合法范围1–1000，输出最多256 KiB，可能 truncated；`--generation N` 可指定历史代次，例如：

```text
d2core logs i_实际实例ID --generation 1 --tail 200 --data-dir <绝对数据目录> --json
d2core a2s enable --dota-dir <Dota安装根目录> --json
```

a2s 的 dota-dir 是包含 game 子目录的安装根目录，不是 game/dota。它会写文件，不能当只读探测使用；核心没有 a2s query/disable 子命令。m0-inspect 的全部参数及采集边界见 [M0工具说明](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/tools/m0/README.md)。

程序还注册了内部 `__engine-quit` 控制台辅助入口，仅供核心调用，不是用户停止实例的接口；用户用 stop。没有 start/delete/update-template/shutdown 等公开子命令：已回收后用新 key create，修改源模板不改变旧实例快照，管理器用 Ctrl+C 退出。

### 帮助与退出码

v0.1.0 沿用 RC.2 的帮助行为，支持 `d2core help`、`d2core --help`、`d2core -h` 输出完整命令列表，帮助退出0；无参数输出完整用法但仍退出2。`d2core help logs` 或 `d2core logs --help` 查看参数，A2S 用 `d2core a2s enable --help`。RC.1 原包没有顶层help，无参数提示不完整，子命令帮助通常退出2、m0-inspect帮助退出1；不要把旧包帮助退出码误判为启动失败。

正常管理调用退出0表示请求成功，create/restart/stop 仍需查询 operation；执行错误通常退出1，CLI用法错误通常退出2。m0-inspect 使用独立错误路径，参数/检查失败返回1。

### 随包 launcher-example

这是另一个可执行文件，不是 d2core 子命令。必填 `--data-dir ABS --template ABS --key KEY`；可选 `--connect-host 127.0.0.1`、`--port 0`、`--hold 30s`。hold 支持0至24h的时长（如10m）。它演示创建、等待、显示连接地址并在持有时间结束后停止自己创建的房间，**默认30秒自动关房**，不是长期常驻启动器。不接受 --json，帮助用 --help；详见 [外部调用示例](../examples/launcher/README.md)。

## Linux 运行环境

管理器和游戏使用同一普通用户。先确认游戏自带库目录（本次为 game/bin/linuxsteamrt64）及 Steam SDK 可被该用户访问，包含所有父目录的遍历权限。root 能读取不代表运行用户能读取。为该用户准备可写 HOME，将经验证的64位 Steam SDK 放在 `$HOME/.steam/sdk64/`（本轮使用 steamclient.so、libtier0_s.so、libvstdlib_s.so、crashhandler.so）。不使用其他用户私有目录作为隐含依赖，不放宽其权限，不从不明来源下载同名库。

下面路径须换成实际已准备路径。目录准备不由核心自动完成；不要在游戏运行时更新共享地图或库。

```bash
export HOME=/srv/my-d2core/home
export LD_LIBRARY_PATH="$HOME/.steam/sdk64:/srv/dota2/client/game/bin/linuxsteamrt64"
test -r "$HOME/.steam/sdk64/steamclient.so"
test -r /srv/dota2/client/game/bin/linuxsteamrt64/libv8.so
ldd /srv/dota2/client/game/dota/bin/linuxsteamrt64/libserver.so
```

在实际普通用户环境下检查 ldd 中没有 `not found`，但这仍不替代运行时 dlopen/Steam 登录验证。随后在相同环境启动 serve；管理器恢复时也要保留 HOME、LD_LIBRARY_PATH。libv8.so 缺失或无法访问会导致 server 模块加载失败，steamclient.so 缺失或无权访问会导致 Steamworks 初始化失败。检查该代 engine.log 和 output.log，不只增加启动超时。核心不会自动配置这些环境。

## 游戏端口范围与指定端口

`serve --port-min N --port-max M` 设置新实例自动分配游戏端口的范围，包含两端。省略时默认27015–27064；端口须在1–65535内，起点不大于终点，范围最多4096个端口。起点和终点相同表示只有一个自动候选端口。该范围不是管理API监听端口，也不限制引擎可能使用的其他辅助端口。

例如允许新房间自动使用27000–27020：

```powershell
# Windows，使用前设置前文所需的实际变量
& $core serve --data-dir $dataDir --port-min 27000 --port-max 27020 --json
```

```bash
# Linux，在配置好 HOME 和 LD_LIBRARY_PATH 的普通用户环境中
"$CORE" serve --data-dir "$DATA" --port-min 27000 --port-max 27020 --json
```

创建时不传 `--port`（或传0），会从范围内选择未预约且TCP/UDP占用检查可用的端口。实际分配结果查看 status 的 port；范围耗尽返回 NO_PORT_AVAILABLE，不自动扩展范围。

```text
d2core create --template <绝对模板路径> --idempotency-key room-auto-001 --data-dir <数据绝对路径> --json
```

如需固定端口，在 create 上指定：

```text
d2core create --template <绝对模板路径> --port 27016 --idempotency-key room-fixed-001 --data-dir <数据绝对路径> --json
```

显式端口可以在自动范围之外，但仍须合法、未预约且通过占用检查；自动范围不是端口访问白名单。每个新房间使用新的创建键，同一次请求重试沿用原键与参数。

- restart 沿用原端口，不重新自动分配；原端口被外部占用时失败，不偷偷换号。
- failed 实例仍保留端口预约，需要显式 stop 回收。成功回收后端口可供新实例使用，实际分配仍检查系统占用。
- 改范围需要正常退出管理器并以新范围、同一 data-dir 恢复 serve；现有实例端口不迁移，也不因此停止游戏。Linux恢复时保留原运行库环境。
- 核心不配置防火墙或 NAT。客户端连接地址与外部映射端口由部署方提供；不要把0.0.0.0当作客户端地址。

下方单房间烟测故意把起止端口设为相同值，便于连接验证；日常多房间部署可按上述方式扩大范围。

## 手动单房间烟测命令

以下路径仅示意，先按模板说明准备 n6.json。在两个终端分别设置相同变量；所有命令使用同一普通用户。

Linux：

```bash
CORE=/srv/my-d2core/package/d2core
DATA=/srv/my-d2core/data
TEMPLATE=/srv/my-d2core/n6.json
# 终端 A：先按上节配置环境，保持运行
"$CORE" serve --data-dir "$DATA" --port-min 27015 --port-max 27015 --json
# 终端 B：设置上述相同变量后执行；新意图只生成一次 key
KEY="n6-$(cat /proc/sys/kernel/random/uuid)"
"$CORE" check --template "$TEMPLATE" --json
"$CORE" create --template "$TEMPLATE" --idempotency-key "$KEY" --data-dir "$DATA" --json
```

Windows PowerShell：

```powershell
$core = 'D:\d2core-v0.1.0-rc.1-windows-amd64\d2core.exe'
$dataDir = 'D:\d2core-v0.1.0-rc.1-windows-amd64\dota-data'
$template = 'D:\d2core-v0.1.0-rc.1-windows-amd64\omgai-n6.json'
# 窗口 A：保持运行
& $core serve --data-dir $dataDir --port-min 27000 --port-max 27000 --json
# 窗口 B：设置上述相同变量后执行
$key = 'n6-' + [guid]::NewGuid().ToString('N')
& $core check --template $template --json
& $core create --template $template --idempotency-key $key --data-dir $dataDir --json
```

两平台 check 返回 ok=true 后才执行 create。create 仅返回受理结果。从结果复制 instanceId 和 operationId，分别代入以下位置（Windows 将 d2core 换为 `.\d2core.exe` 或 `& $core`，Linux 用实际可执行文件路径）：

```text
d2core operation <operationId> --data-dir <相同数据绝对路径> --json
d2core status <instanceId> --data-dir <相同数据绝对路径> --json
d2core logs <instanceId> --tail 200 --data-dir <相同数据绝对路径> --json
d2core restart <instanceId> --data-dir <相同数据绝对路径> --json
```

轮询 operation 至 succeeded，再核对 status 的 running/ready；实际客户端执行 `connect <可达服务器地址>:<游戏端口>`，确认操作正常。restart 会断开房间，须查询返回的新 operationId，成功后再次连接。随后留在房间，在 serve 终端 Ctrl+C；确认游戏仍正常，再用原用户、原 data-dir 和原环境启动 serve，查询原实例，不重新 create。重开终端时要重新设置变量，并保存/取回实际 ID；可用 list 查找，不把创建 key 当实例 ID。

最后显式停止：

```text
d2core stop <instanceId> --data-dir <相同数据绝对路径> --json
d2core operation <stop返回的新operationId> --data-dir <相同数据绝对路径> --json
d2core status <instanceId> --data-dir <相同数据绝对路径> --json
```

确认 succeeded 与 reclaimed/stopped/cleanup=complete 后，再退出管理器，保留日志。已回收实例不能 restart；下次开房以新 key 创建新实例。同一次不确定请求则保留原 key 重试，不因 CLI 超时另开一房。

## 接口与状态说明

先修改 [平台模板](../examples/README.md) 中的路径、地图与经实测的就绪规则。使用同一操作系统用户启动管理端和 CLI；data-dir 必须是绝对 ASCII 路径。建议选择尚不存在的独立目录，让核心创建私有权限；已有目录若不满足权限会被拒绝，不会自动改 ACL。Linux socket 完整路径最长107字节，目录过长时明确报错。

```text
d2core check --template <模板绝对路径> --json
d2core serve --data-dir <数据绝对路径> --json
```

serve 前台常驻，在 stdout 输出一次 JSON 就绪信息，诊断写 stderr。其他终端执行：

```text
d2core create --data-dir <数据绝对路径> --template <模板绝对路径> --port 27015 --idempotency-key request-001 --json
d2core operation <操作ID> --data-dir <数据绝对路径> --json
d2core status <实例ID> --data-dir <数据绝对路径> --json
d2core logs <实例ID> --tail 100 --data-dir <数据绝对路径> --json
d2core restart <实例ID> --data-dir <数据绝对路径> --json
d2core stop <实例ID> --data-dir <数据绝对路径> --json
d2core list --data-dir <数据绝对路径> --json
```

受理后查询 operation，成功后仍须检查当前 status。room=ready 表示当前代日志组合和进程/监听符合规则，不代替实际客户端进房。客户端可达域名和 NAT 映射由上层提供，监听0.0.0.0不是客户端连接地址。

请求超时用相同幂等键和相同参数重试，不能换键以掩盖不确定结果。重启沿用已保存配置与端口；修改模板后须结束旧实例再新建。stop 无需等待玩家离开，确认退出后清理自身生成文件，保留历史日志和结果；清理失败可以再次 stop，只重试清理，不重新开服。

管理端退出保留专服；重新启动必须使用同一 data-dir。遇到 unknown 或持久化故障时保留现场和日志，不能删状态或重新 create 来掩盖残留。数据损坏、不兼容版本、目录不可写均明确报错，不自动换目录。不要同时用外部工具修改核心生成的 cfg 或运行记录。

当前持久化格式为2，模板与协议仍为1。格式1实验目录不会自动升级；保留旧目录及证据，确认旧实例已停止回收后为新测试使用独立目录。state.json 与 store.marker 缺失、不匹配或校验失败时保留现场，不手工删标记或修改校验和来绕过检查。启动恢复不自动派生游戏进程；操作返回 INTERRUPTED 时先查询实例状态，确认并显式停止回收后，再以新创建意图申请实例。双平台整机重启验收已完成，详见 [M2记录](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/docs/validation/m2.md)。

普通创建/重启不修改 gameinfo 或 VAC 配置。本地管理不监听 TCP；Windows 用命名管道，Linux 用 Unix socket，不需要给管理 API 开放公网防火墙端口。游戏端口的外部可达性应单独验证。

M3 可用 serve --port-min 27015 --port-max 27064 --history-days 7 --min-free-mib 1024 指定自动端口、回收后历史保留天数和磁盘余量阈值；create 省略 --port 即自动选择。历史到期后实例、操作和创建键一起移除，旧ID返回NOT_FOUND，调用方的新意图始终使用唯一键。缩短保留期前备份需要长期留存的证据。list 的storage返回最近空间检查及清理错误；在线每分钟维护，离线期间以运维磁盘告警兜底。活跃日志没有硬大小上限，不因日志容量停止房间。

运行前提：Linux专服需要可加载的steamclient.so等游戏依赖，可单独准备SteamCMD运行库，不要求核心运行时安装Go。Windows新用户需要Steam SDK能找到该用户可用的Steam安装及DLL路径。具体准备依安装方式而定；核心不自动改注册表或安装运行库。若用独立cfg挂载做隔离，必须保留安装自带默认cfg/vcfg，并确认目录可由运行用户写入。地图ready不代表Steam认证完成或客户端可达，仍需独立核验网络及实际进房。

## 修复候选升级与人工回收（N-1 / N-2）

以下限制适用于以1a0b79c为产品代码基线的RC。独立复核未发现确认的RC阻塞项；这些说明不新增自动删除或修复行为。

### failed 实例必须显式回收

观察失败后，即使日志、端口或外部条件恢复，实例也不会自行从failed回到ready。历史create/restart操作成功不代表当前房间可用；以当前status的lifecycle、process、room、evidence和error为准。

1. 保存实例ID、原创建键、operation/status与日志，先确认错误原因。结果未知的请求继续用原键和原参数查询/重试，不能创建新键掩盖残留。
2. 执行 `d2core stop <实例ID> --data-dir <绝对数据目录> --json`，查询返回的operation并检查status。
3. 确认lifecycle=reclaimed、process=stopped、cleanup=complete后，才把替代房间作为新意图，以新的唯一键create。failed实例不能直接restart；其他不相关实例可独立运行。
4. stop清理失败时先处理归属或权限问题，再对同一实例重试stop；不得按通用进程名杀进程、删state.json/store.marker或改校验和。无法确认进程身份时保留现场，不强行删除文件。

### 无归属 cfg 与升级前遗留 FIFO

当前cfg清理要求父目录对象、文件对象及内容指纹一致。旧format2记录缺乏cfgOwnership、外来同名文件或替换后的对象会被保留，可能停在cleanup=failed。内容相同不代表归属相同。应备份状态和相关文件，核对实际路径、对象归属、相关进程已退出且无人使用后，由运维决定是否移走或删除确切文件，再重试同一实例stop；不要递归删除data-dir或整个cfg目录。

N-1：升级前经历“已尝试spawn、身份未落盘、子进程已退出”的旧记录，可能同时缺少InputOwnership与Identity，却留下stdin.fifo。新版本不会推测该FIFO归属；即使实例显示reclaimed/cleanup=complete，该遗留文件仍可能使到期retention报CLEANUP_FAILED，保留该实例历史、操作与创建键。只有人工核实确切run目录、FIFO类型、所属实例与无进程使用后，才可处理该单个遗留FIFO；之后等待在线维护重试并检查list.storage清理结果。无法证明归属则保留并告警。不得扩大白名单或批量删除所有FIFO。该限制不妨碍其他实例运行，也不因升级自动消失。

### portCheck 的判定边界

portCheck=partial不等于已确认冲突，也不等于检查完全部端口。可确认的TCP监听、主端口UDP及与TCP同号的UDP服务端点参与跨实例冲突检查；独立UDP端点用途、IPv6通配的双栈覆盖无法确定时，在notes中明确保留。确认冲突时status=conflict、room不再ready；核心不自动杀房或换端口。运维需结合notes、模板、实际监听和网络映射验证，不把partial改写为“全部无冲突”。

其他保留边界：模板ready不等于Steam认证完成或客户端可达；历史键受retention期限约束；普通整机重启验收不等于断电验收；不承诺一般性子进程树管理，真实PID复用未自然观察到。Windows Go1.27.1配合现有MinGW8.1验证race时使用 `go test -race -ldflags=-linkmode=external ./... -count=1`；默认链接曾在测试开始前报0xc0000139，不能把该失败解释为数据竞争或跳过race。
