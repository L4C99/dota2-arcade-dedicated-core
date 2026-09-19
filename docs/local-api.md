# 本地协议 v1（M1-01 契约）

本文件固定 M1 实现目标，不表示接口已可用。实现和验收状态见 [M1 记录](validation/m1.md)。模板 schemaVersion=1、磁盘 formatVersion=1、本地 protocolVersion=1 分开编号；程序 version 输出版本、Git 提交、构建时间及上述版本。未知版本/字段拒绝，不覆盖已有数据。

## 访问与报文

仅支持同一操作系统用户。Windows 使用当前用户 ACL、拒绝远程客户端的命名管道；Linux 使用私有 0700 目录内的 0600 Unix socket，并校验 peer UID。不监听 TCP 管理端口。管理员/root 的操作系统权限不在同用户隔离保证之外另作承诺。

默认 data-dir 为可执行文件旁 data；--data-dir 必须为绝对路径，CLI 与管理端一致。serve 先取得独占锁；已有管理端明确失败。客户端从 manager/endpoint.json 读取端点，包含 protocolVersion、传输类型和地址。端点文件不是信任凭据，仍须校验操作系统访问控制。

每连接一个请求及响应，UTF-8 JSON 单行、换行结束；最大请求 1 MiB，响应最大 2 MiB，连接读写超时 5 秒。拒绝重复 JSON 键、未知字段、尾随 JSON。管理端在连接断开后继续已受理的操作。

请求：

```json
{"protocolVersion":1,"method":"create","params":{"template":"C:/Templates/map.json","port":27015,"idempotencyKey":"example-request-001"}}
```

受理：

```json
{"protocolVersion":1,"ok":true,"result":{"accepted":true,"instanceId":"i_<random>","operationId":"o_<random>","state":{"lifecycle":"active","process":"stopped","room":"unknown","cleanup":"pending","generation":0,"operationStatus":"running"}}}
```

错误：

```json
{"protocolVersion":1,"ok":false,"error":{"code":"PORT_IN_USE","stage":"validate","message":"requested port is occupied"}}
```

产生记录后的错误附 instanceId/operationId。稳定错误码：INVALID_REQUEST、UNSUPPORTED_VERSION、INVALID_TEMPLATE、INVALID_PATH、PORT_REQUIRED、PORT_IN_USE、NOT_FOUND、BUSY、RECLAIMED、INVALID_STATE、IDEMPOTENCY_CONFLICT、MANAGER_UNAVAILABLE、MANAGER_LOCKED、IDENTITY_UNVERIFIED、START_FAILED、START_TIMEOUT、STOP_FAILED、CLEANUP_FAILED、IO_ERROR、INTERNAL_ERROR。stage 为 protocol/validate/persist/spawn/observe/stop/cleanup/transport。message 供人阅读，不能作为程序分支依据。

## CLI 与请求参数

所有命令支持 --json；stdout 仅输出一份结构化结果，诊断写 stderr。退出码 0 表示请求成功（变更仅受理），1 为请求或执行错误，2 为 CLI 用法错误。默认无需 --json 也使用同一 JSON 响应，便于管道调用。M1 CLI 不默认等待异步完成。

| 命令 / method | 参数 | 结果 |
| --- | --- | --- |
| serve（仅 CLI） | --data-dir 可选 | 前台运行管理端；退出不终止专服 |
| check（仅 CLI，离线） | --template ABS | 校验规范化模板；不启动、不承诺资源可加载或端口可用 |
| create | template: ABS，port: 1..65535，idempotencyKey: 必填 | accepted、instanceId、operationId；M1 缺 port 返回 PORT_REQUIRED，M3 增加自动分配 |
| list | 无 | 活动与失败待回收实例摘要；不列已回收历史 |
| status | instanceId | 实例快照、当前代次、状态及证据 |
| operation | operationId | kind、instanceId、generation、status、error、createdAt、finishedAt |
| logs | instanceId；generation 可选（默认当前/最后代）；tail 默认100，上限1000 | generation、文本、truncated；最多256 KiB，历史可读；不提供控制台命令接口 |
| restart | instanceId | 新 operationId；仅运行中的活动实例；沿用快照/端口 |
| stop | instanceId | 停止并回收的 operationId；加载时可中止 create/restart |
| version（仅 CLI） | 无 | 程序及格式版本 |

对应 CLI：create --template ABS --port N --idempotency-key KEY；status/restart/stop 接位置 instanceId；operation 接位置 operationId；logs INSTANCE --tail N --generation N。所有管理端调用命令支持 --data-dir ABS。本地协议只接受上表字段，不允许执行任意 shell 或新增控制台命令。

三个变更命令统一返回上述 ID 和 state，state 是受理时的真实当前状态，不是完成预测。status 的 result 为实例对象：instanceId、templateName、port、generation、lifecycle、process、room、cleanup、currentOperationId、createdAt、updatedAt、evidence、bindings、error。list 的 result 为 {instances:[实例对象]}；operation 的 result 为操作对象，字段为上表所列，另含 operationId。不存在的可选证据/error/finishedAt 使用 null；数组为空时使用 []。所有时间使用 UTC RFC3339Nano 字符串。

evidence 为 {generation,observedAt,source,matched,valid}：source 为本代日志绝对路径，matched 为已命中的规则及首次 observedAt 列表；规则与适用地图保存在模板快照中。未读取本代日志时为 null；退出或身份不明确后 valid=false，room 不再为 ready，历史证据仍可诊断。bindings 为 {protocol,address,port,pid,observedAt} 数组，只表达已核验进程的实际监听，不表示公网可达；不以 0.0.0.0 生成客户端连接地址。

端口同时检查管理端分配及同号 TCP/UDP 实际绑定，启动后再次核对监听归属；预检不是抢占保证。重启遇到外部占用失败并保留原分配，不能换号。附加实际端口进入诊断和冲突检查，不全部当成公网预约端口。

## 实例与操作状态

| 维度 | 值 | 含义 |
| --- | --- | --- |
| lifecycle | active / failed / reclaimed | 管理归属；failed 仍占用分配，reclaimed 只能查历史 |
| process | stopped / running / unknown | 由当前身份核验决定，不能仅凭 PID |
| room | unknown / loading / ready / failed | ready 必须对应本代新日志和活进程 |
| operation | running / succeeded / failed / cancelled | 受理后 running；stop 中止的旧操作为 cancelled |
| cleanup | pending / complete / failed | 与进程停止分开；失败保留归属信息以便重试 |

创建：验证→持久化快照与操作→启动→本代就绪后操作成功。早退/超时为 failed，保留记录与分配，不自动重启；启动超时同时尝试停止，未确认退出保留 unknown。重启：确认旧代退出→清理旧代生成 cfg→新代；旧代不确认退出则不派生下一代。主动停止：抢先取消加载或重启→核验并停止→清理→回收。任何正在执行的变更除 stop 中止启动外返回 BUSY。

已回收实例 restart 返回 RECLAIMED；未知 ID 返回 NOT_FOUND。重复 stop：进行中的停止返回同操作；已回收返回原停止结果；清理失败时显式再次 stop 创建新的清理操作，只重试文件清理，不启动进程。旧失败操作保留不改写；实例 cleanup 反映最新结果。清理完成前保持 failed/pending 归属及端口记录，不能伪装成已回收。

启动/重启期间收到 stop 后须在每次启动边界检查取消，避免取消后再派生下一代。若进程身份不明确，保持 unknown 和分配并报告错误。历史操作成功不覆盖后续意外退出导致的当前状态失败。

## 配置、覆盖与模板 v1

模板保留 name、executable、workingDirectory、arguments、cfg.directory、cfg.lines，新增 readiness 和 timeouts。所有路径按目标操作系统验证，文件/目录存在性和类型静态检查；check 不写入目录。模板为受信任本机配置，不支持安全执行不可信模板。

readiness.successAll 是必须全部命中的非空字面字符串列表；failureAny 是失败字面字符串列表。匹配仅扫描本代 engine.log，失败优先，不使用旧日志或固定延时推断就绪；缺证据持续 loading 到超时。M0 地图级组合只适用于已测地图。列表各最多32项，每项最多512字节、非空；不使用正则表达式。

timeouts 包含 startupSeconds / stopSeconds / forceSeconds，省略或0使用120/10/5；负数或大于3600拒绝。M1 唯一 CLI 配置覆盖是 port，最终规范化模板和 port 一起持久化；重启不用重新读取源模板。快照不冻结 VPK/额外 exec 内容。

参数数组至少且仅一次包含相邻项 -port / {{game_port}}、-con_logfile / {{log_path}}、+exec / {{cfg_name}}；保持原有顺序，不通过 shell，不隐式注入 VAC/hibernate。保留 instance_id、game_port、cfg_name、log_path 四种占位符，路径字段/name 不允许占位符，未知/残缺占位符拒绝。arguments 中占位符作为完整参数，cfg 中可嵌入行内；cfg 每项只允许一行，不接受 NUL/CR/LF，双引号须闭合。

instance_id/game_port/cfg_name 为核心生成的安全 ASCII token。log_path 是本代绝对 ASCII 路径：argv 中原样传值；cfg 引号内展开为转义文本，引号外展开为完整双引号字符串。cfg 路径反斜杠规范化为正斜杠，插入引号按 cfg 语法转义，禁止注入换行。源模板不修改。唯一 cfg 名含随机实例标识与代次，不以模板名/端口作为归属。

## 幂等与保留

创建键为1–128字节 ASCII 字母、数字、点、下划线或短横线；作用域为同一 data-dir，同用户。请求指纹包含规范化模板路径及请求 port；同键同请求返回原 instanceId/operationId，不重读修改后的模板或开新房；同键不同请求返回 IDEMPOTENCY_CONFLICT。M1 固定并实现基本契约；并发、落盘窗口及重启恢复由 M2 全面验收。

M1/M2 默认保留全部实例历史、操作和幂等映射，不因 stop 回收删除。M3 明确保留期限和容量策略并同步更新契约；删除历史时必须协调幂等保留，不能让旧键静默变为新建。调用者按原键重试超时请求，再查询操作；不要改用新键来掩盖不确定结果。
