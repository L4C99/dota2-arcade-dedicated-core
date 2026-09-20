# 本地核心操作（开发版本）

真实引擎验收状态见 [M1 记录](validation/m1.md)。M0—M4既定范围已完成验收；接管现有生产房间仍须单独部署授权。游戏、运行库和两端匹配的地图资源由使用者准备；核心不下载或更新它们。

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

当前持久化格式为2，模板与协议仍为1。格式1实验目录不会自动升级；保留旧目录及证据，确认旧实例已停止回收后为新测试使用独立目录。state.json 与 store.marker 缺失、不匹配或校验失败时保留现场，不手工删标记或修改校验和来绕过检查。启动恢复不自动派生游戏进程；操作返回 INTERRUPTED 时先查询实例状态，确认并显式停止回收后，再以新创建意图申请实例。双平台整机重启验收已完成，详见 [M2记录](validation/m2.md)。

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
