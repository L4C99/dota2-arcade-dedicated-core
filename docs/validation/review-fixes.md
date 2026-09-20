# 裁决修复候选（进行中）

唯一任务单：用户提供的汇总裁决，固定基线7bcbdb72dad5dc44c4c0a88fdad6a1c0fb3d38a1。开工工作树干净、HEAD一致。原始四份报告仅供证据查阅。本轮处理CJ-01—CJ-11，不纳入疑似、否决或可选项；此前M0—M4验收不代表修复候选已复验。

| 裁决项 | 级别 | 修复提交 | 自动回归及当前结果 | 待实机复验 |
| --- | --- | --- | --- | --- |
| CJ-01 | P1，阻塞 | 2e4dbad、fee0db3 | 双平台post-spawn故障注入；core部分身份回滚；Windows定向通过，补核后最终全套待跑 | Windows/Linux真实create→ready→restart→ready→stop |
| CJ-02 | P1，阻塞 | a9e3817 | 双平台可控回拨、watch/finish/recovery、reopen/stop/create通过 | 不需要客户端或修改系统时钟 |
| CJ-03 | P2，阻塞 | 446efcb、77a554b | 双平台文件/父目录替换、核验后交换、junction/symlink、旧format2、拒绝后重试通过；追加O_EXCL同内容Windows通过 | 不要求真实游戏；追加测试Linux最终回归待跑 |
| CJ-04 | P2 | 303f0fe | Linux身份未写+child退出→recover→stop→prune通过；未知FIFO仍拒绝 | Linux真实失败启动/回收及正常重启闭环 |
| CJ-05 | P2 | cccb9b4 | 双平台17项失败前缀轮转与恢复重试通过 | 无 |
| CJ-06 | P2 | 6043064、48ca073 | 双平台ready后日志/失败规则/binding异常、重复写入次数通过；症状消失补核Windows通过 | 不要求玩家；补核后Linux最终回归待跑 |
| CJ-07 | P2 | c3b1db2 | 双平台原生extra TCP/UDP/出站helper、冲突矩阵及双实例隔离通过；矩阵使用受控表注入，未冒称实际重复TCP绑定 | 双平台真实bindings无误报，纳入总体进房回归 |
| CJ-08 | P3 | 0cbb23f | 双平台CLI真实IPC与protocol错误码/阶段通过 | 无 |
| CJ-09 | P3 | ca7ba61 | 双平台低磁盘+busy/reclaimed及正常restart通过 | 无 |
| CJ-10 | P3 | 5720060 | 双平台按路径模拟不同卷余量、受理前零记录及原键重试/stop通过 | 无 |
| CJ-11 | P3 | edcaad5 | Windows case/dot/8.3（均未跳过）、symlink/junction及A2S全套通过；Linux原有分支全套通过 | Windows隔离安装a2s enable+实际A2S_INFO |

CJ-07用户确认：额外TCP监听、UDP服务端口参与跨实例检查；相同协议/端口且监听地址重叠报告冲突、房间不再ready；不自动杀房或换端口。普通UDP出站临时端点只诊断，无法区分用途时明确诊断不假称已检查。具体可判定依据在实现前记录。

CJ-01实际Windows命令（Go1.27.1）：go test ./internal/core ./internal/engine -run TestStart -count=1，通过。故障注入覆盖OpenProcess/nativeIdentity；core验证PID>0错误返回后的已退出状态可stop回收。Linux新增pidfd/read/mismatch/poll故障与resolved路径测试，尚未执行，不能标通过。首次构建发现测试seam与start方法同名，改为spawn后定向通过。

CJ-02：写入UpdatedAt/FinishedAt采用当前墙钟与已有记录时间最大值，未删除完整性校验、未改变启动截止时间/进程身份。Windows go test ./internal/core -run TestBackwardClock -count=1通过，覆盖回拨后restart/finish/stop、watch、future intent recovery、reopen/status/create。CJ-01修复提交2e4dbad；Linux全套待执行。

CJ-02提交a9e3817。CJ-03新增format2可选cfgOwnership，checksum覆盖；旧记录可打开，但现存文件无ownership或CFGCreated=false拒绝删除。Windows按原句柄删除并禁止delete-sharing；Linux固定父目录，私有随机暂存后复核对象/内容，遇到交换保留并尝试无覆盖恢复；未知崩溃残留不猜测删除。Windows records全套通过；Linux Go1.27.1普通用户原生 go test -mod=vendor ./internal/engine ./internal/core ./internal/records -run 'TestStart|TestBackwardClock|TestCFG|TestCleanupUnrecorded' -count=1通过，覆盖CJ01—03新增用例。Linux工具链官方直连失败后经本机下载传入，未动游戏。新增测试覆盖同内容不归属、旧格式、父目录替换、symlink/junction及核验后交换；拒绝后可显式重试。

CJ-03提交446efcb。CJ-04：spawn前durable InputOwnership，仍为format2可选字段；恢复没有进程身份但已确认退出时按输入inode/device清理，不放宽未知FIFO白名单。Windows core/records全套通过；Linux普通用户 core/records/engine全套 -count=1通过（含新TestOrphanFIFOIdentityGapExpires、TestUnknownFIFORetentionRefused），证明身份未写/child退出→reopen→stop→prune，外来FIFO仍拒绝。仅helper自动测试，真实Dota回收待验收。

CJ-04提交303f0fe。CJ-05增加内存轮转游标，每周期仍最多limit次尝试；失败保留重试，后续候选不被固定前缀饿死；manager重启游标重置但持续维护会前进。Windows TestRetentionFailedPrefixCannotStarve通过：17项前16失败，第二轮第17成功，移除外来阻塞后16项可重试并同步清除keys/operations。Linux合并回归待执行。

CJ-05提交cccb9b4。CJ-06统一observe失败时room/evidence失效，未知身份保持unknown；watch仅状态/错误语义变化保存，不改历史成功operation。Windows TestObservedFailureInvalidatesReadyOnce通过：ready后failureAny、日志非普通文件、binding错误三种；2.2秒内持久化恰好一次，之后stop可回收。Linux全套合并验证待执行。

CJ-06提交6043064。CJ-08：Fingerprint相对路径返回INVALID_PATH/validate，logs tail越界返回INVALID_REQUEST/validate（0仍默认）；未把真实I/O错误改为参数错误。Windows core TestCallerErrorCodes和cmd TestCLICallerErrorCodes通过；CLI实际通过本地命名管道，首次沙箱ACL拒绝后正常权限重跑通过。Linux合并回归待执行。

CJ-08提交0cbb23f。CJ-09：restart先检查RECLAIMED/BUSY/INVALID_STATE，再查两卷空间；Windows TestRestartStateBeforeLowSpace与TestLifecycleSnapshotRestartStopAndRetry通过，低空间下BUSY、RECLAIMED不再被遮蔽，合法运行态仍拒绝不足空间。

CJ-09提交ca7ba61。CJ-10：CreateChecked在同键命中后、单次模板规范化后对准确快照执行两卷检查，再分配和持久化；不重复读取模板。Windows TestCreateChecksCFGVolumeBeforeAcceptance、TestLowDiskRejectsNewIntentButAllowsRetryAndStop通过，按路径注入data足够/cfg不足，新意图零记录，旧键模板失效且低空间时仍可重试/stop。

CJ-10提交5720060。CJ-11改Windows逐路径组件检查REPARSE属性，不再按大小写敏感字符串判断链接。Windows a2s全套-v通过，case/dot/8.3别名均执行通过（非跳过），真实symlink与junction拒绝、备份/幂等/冲突/写入失败/权限测试保留。首次junction命令因路径斜杠解析失败，规范化测试路径后通过。Linux分支保持原有严格解析规则，Windows隔离真实A2S验收待执行。

CJ-11提交edcaad5。CJ-07规则按用户确认落地：portCheck公开complete/partial/conflict及诊断，额外TCP与可确认UDP服务端点比较同协议/同号/重叠地址；未知UDP和双栈只报告partial。Windows与Linux完整core通过，含原生额外TCP/UDP/出站helper及纯冲突矩阵（矩阵注入不冒称OS实际允许重复TCP绑定），双实例停止隔离通过。Windows全套test/vet通过。Linux完整test出现TestLinuxLifecycleAndIdentity退出期permission denied；原固定7bcbdb7同环境100次复验15次同样失败，确认为既有间歇路径，本轮未擅自修复/跳过，完整Linux验收因此仍有阻塞。Windows默认race在进入测试前0xc0000139，改外部链接的config测试通过，完整race进行中。

CJ-01补核：rollback的Wait必须产出ProcessState才标Exited=true，避免等待API错误被当成退出；core回归现在明确返回缺少creation/boot/tick的partial identity，仍确认真实child已退出且stop可收敛。Windows TestStart定向通过。

CJ-06补核：已经failed的生命周期即使下一次日志或binding症状消失也不能重新ready；保持原错误直到显式回收。Windows TestObservedFailure三类各追加症状消失检查通过。

CJ-03追加直接O_EXCL碰撞回归：事先创建同名、字节完全一致cfg→PrepareRun失败且不取得ownership→CleanupRun拒绝，外来文件未变。Windows TestPrepareConflictSameContentNeverOwnsCFG通过。CJ-01补核fee0db3；CJ-06补核48ca073。


## 全套验证与新增阻塞

- Windows Go1.27.1：go test ./... -count=1、go vet ./...通过（CJ07代码快照）。默认go test -race在进入测试前均0xc0000139；现有MinGW8.1与默认链接组合不兼容。改用go test -race -ldflags=-linkmode=external ./... -count=1后完整通过，未关闭race或跳过用例。其后的CJ01/CJ06边界补核已定向通过，需在最终固定候选再跑全套。
- Ubuntu24普通用户Go1.27.1：go test -mod=vendor -race ./... -count=1全部通过（CJ07代码快照）；普通全套test在engine的TestLinuxLifecycleAndIdentity报permission denied，其余包通过，因此未执行串接的vet，不标全套通过。
- 同环境原始固定7bcbdb7，TestLinuxLifecycleAndIdentity -count=100出现15次同样失败。退出过程中的/proc身份读取与pidfd退出可读存在窗口；保持原失败证据，未改测试标准。该项不在裁决任务单内，已经单独请求用户是否授权追加处理；未获确认前不修改。即便某次race或单次test成功，也不能消除这个已复现的阻塞。
- Linux编译器只在授权的新隔离机安装，0个包升级、无服务重启；旧生产机与本机系统配置未改动。原始报告、工具链、测试原始结果均留在ignored local/review及隔离机项目review目录，不上传地图/凭据。

暂停状态：用户因移动电脑要求暂停。本轮代码候选77a554b；Linux裁决外问题用户答复‘等我稍后决定’，未授权追加修复。真实专服/客户端复验尚未进行，不得沿用旧验收结论。当前仅暂停，不改变任务范围、严重程度或待决事项。私有接续说明local/review/PAUSED.md。

## 关机前最终收尾（2026-09-20）

固定代码候选77a554b5aa88fc639998b22c4aff676e94682ff7的最终测试已全部结束，上文“待跑/进行中”为历史记录，以本节为最新结果：

| 平台 | 普通全套 test | vet | 全套 race |
| --- | --- | --- | --- |
| Windows | 通过 | 通过 | 失败：TestMultipleAutomaticInstancesStayIndependent/different-templates，multi_test.go:66，NO_PORT_AVAILABLE |
| Ubuntu | 失败：TestBackwardClockRecoveryAndWatch，time_test.go:51，创建阶段 START_FAILED / recorded process has exited | 通过 | 通过 |

Windows race实际使用外部链接，日志未出现数据竞争报告，但测试失败，不能标通过。Linux本次失败发生在CJ-02测试的初始创建阶段，根因尚未调查，不得与此前待授权的/proc权限问题混为一谈。CJ-03追加用例所在records包本次双平台通过；CJ-01/CJ-06补核用例本次未报告失败，但不能据此将整体验收标通过。

两项新失败均保留，恢复后先调查，不跳过、不降低断言、不因早一轮通过而忽略。本轮仍未完成真实专服/客户端复验，不建议发布。Linux裁决外追加修复继续等待用户决定。

全部原始日志与退出码已保存到本地ignored local/review/final-windows-*、final-linux-*。只读检查确认本机无go/*.test/d2core/dota2进程，隔离机d2coretest用户无进程，项目manager服务inactive。未关停或修改无关资源。任务因用户移动电脑暂停，代码提交仅本地，未push或发布。

## 恢复后诊断（2026-09-20）

用户已允许继续；开工HEAD aa4fcc0，工作树干净。暂停期间没有丢失或替换修复提交。Windows虚拟机已由用户启动且原IP可连接，尚未启动真实专服。

- Windows原失败已复现。失败瞬间netstat确认测试选中的第二端口被外部HTTP连接占用（TIME_WAIT）；本机TCP动态范围实际为1024–15000。测试原先通过Listen(:0)选第一端口并假设相邻端口会保持空闲，存在环境竞争；核心PORT_IN_USE拒绝符合契约。仅改多实例测试的候选端口准备：随机探测20000–29999服务端口对，保留全部原并发、唯一端口、耗尽、退出隔离及回收重用断言，不重试失败create、不跳过用例，不改系统端口范围或产品分配逻辑。检查仍不是OS预约，其他环境的动态范围可能不同。
- Linux原CJ02启动失败单独15次未复现，完整core三轮出现另一个loading阶段失败；进一步独立启动探针捕获readIdentity因/proc/cmdline短暂为空返回ErrGone，但紧接着同进程stat仍为R、完整argv可读。修复候选探针200次发生2次；原7bcbdb7独立副本200次发生4次，同一进程随后可取得完整匹配身份并安全清理。此为另一项既有启动竞态，不能把它当成墙钟回拨用例已通过或以延时掩盖。
- 新Linux问题与此前退出期EACCES分开记录，已请求用户决定是否追加，两项均尚未改动。原始诊断日志保存在ignored local/review；诊断探针只在独立副本运行，不混入最终正式回归。
- 测试helper补充失败时stderr、状态及输出日志，便于保留后续失败现场；未改变成功条件或断言。
Windows多实例修正后：go test -race -ldflags=-linkmode=external ./internal/core -run TestMultipleAutomaticInstancesStayIndependent -count=20通过（115.709s），含40个同/不同模板子场景，未关闭race，未重试失败create。完整最终回归与真实专服仍待，不把定向通过扩写为整体通过。

用户明确授权单独追加修复两项Linux既有竞态（不改变CJ裁决分类）：
- LX-01：退出时/proc权限撤销早于pidfd退出可读。仅在原已持有pidfd上对EACCES/EPERM与既有ENOENT路径一样等待最多100ms；只有pidfd证明退出才返回stopped，持续权限错误仍ErrIdentity，身份不符不重试，未发送任何附加信号。Linux原生TestExitPermission*和TestLinuxLifecycleAndIdentity -count=30通过（4.729s），正反向验证真实退出、持续拒绝、身份不符与poll失败。
- LX-02：启动时/proc/cmdline短暂为空，后续实现及验收单独记录。

3381295 Windows完整go test ./... -count=1、go vet ./...、go test -race -ldflags=-linkmode=external ./... -count=1均通过；原失败日志继续保留。此结果不覆盖后续Linux追加修改，也不代替真实游戏验收。

LX-01修复提交4608feb。LX-02只在Start已经持有原child pidfd且readIdentity返回空cmdline对应的ErrGone时，最多100ms重新读取；pidfd确认退出立即失败，其他读取错误立即失败，读取成功后原exe/argv/FIFO完整匹配仍必需，身份不符不重试，超时走CJ-01确切子进程回滚。未放宽普通Open/信号权限，也不更改格式或协议。
Linux原生TestStartup*、TestStartIdentityRollback、TestExitPermission*、TestLinuxLifecycleAndIdentity -count=10全部通过（5.124s），其中真实Start/Stop重复1000次；确定性测试覆盖空读后恢复、持续空读超时与FIFO回滚、身份不符、真实pidfd退出、poll错误。完整test/vet/race和真实Dota仍待。
