# 裁决修复候选（进行中）

唯一任务单：用户提供的汇总裁决，固定基线7bcbdb72dad5dc44c4c0a88fdad6a1c0fb3d38a1。开工工作树干净、HEAD一致。原始四份报告仅供证据查阅。本轮处理CJ-01—CJ-11，不纳入疑似、否决或可选项；此前M0—M4验收不代表修复候选已复验。

| 裁决项 | 级别 | 修复/验证状态 | 实机责任 |
| --- | --- | --- | --- |
| CJ-01 | P1，阻塞 | spawn身份失败用创建时Process回滚，确认退出才允许回收；Linux执行resolved路径，argv契约不变。Windows定向TestStart通过；Linux与全套待执行 | 双平台真实create/ready/restart/stop待验收 |
| CJ-02 | P1，阻塞 | 待处理 | 可控clock自动回归，不改系统时间 |
| CJ-03 | P2，阻塞 | 待处理 | 双平台原生文件系统回归 |
| CJ-04 | P2 | 待处理 | Linux原生FIFO恢复；真实引擎回收待验收 |
| CJ-05 | P2 | 待处理 | 有界维护公平性自动测试 |
| CJ-06 | P2 | 待处理 | 可控日志/helper自动回归 |
| CJ-07 | P2 | 待处理；用户已确认下述规则 | 双平台原生socket及真实bindings待验收 |
| CJ-08 | P3 | 待处理 | CLI/协议自动回归 |
| CJ-09 | P3 | 待处理 | 状态/磁盘优先级自动回归 |
| CJ-10 | P3 | 待处理 | 分路径磁盘注入自动回归 |
| CJ-11 | P3 | 待处理 | Windows原生路径及隔离A2S待验收 |

CJ-07用户确认：额外TCP监听、UDP服务端口参与跨实例检查；相同协议/端口且监听地址重叠报告冲突、房间不再ready；不自动杀房或换端口。普通UDP出站临时端点只诊断，无法区分用途时明确诊断不假称已检查。具体可判定依据在实现前记录。

CJ-01实际Windows命令（Go1.27.1）：go test ./internal/core ./internal/engine -run TestStart -count=1，通过。故障注入覆盖OpenProcess/nativeIdentity；core验证PID>0错误返回后的已退出状态可stop回收。Linux新增pidfd/read/mismatch/poll故障与resolved路径测试，尚未执行，不能标通过。首次构建发现测试seam与start方法同名，改为spawn后定向通过。

CJ-02：写入UpdatedAt/FinishedAt采用当前墙钟与已有记录时间最大值，未删除完整性校验、未改变启动截止时间/进程身份。Windows go test ./internal/core -run TestBackwardClock -count=1通过，覆盖回拨后restart/finish/stop、watch、future intent recovery、reopen/status/create。CJ-01修复提交2e4dbad；Linux全套待执行。

CJ-02提交a9e3817。CJ-03新增format2可选cfgOwnership，checksum覆盖；旧记录可打开，但现存文件无ownership或CFGCreated=false拒绝删除。Windows按原句柄删除并禁止delete-sharing；Linux固定父目录，私有随机暂存后复核对象/内容，遇到交换保留并尝试无覆盖恢复；未知崩溃残留不猜测删除。Windows records全套通过；Linux Go1.27.1普通用户原生 go test -mod=vendor ./internal/engine ./internal/core ./internal/records -run 'TestStart|TestBackwardClock|TestCFG|TestCleanupUnrecorded' -count=1通过，覆盖CJ01—03新增用例。Linux工具链官方直连失败后经本机下载传入，未动游戏。新增测试覆盖同内容不归属、旧格式、父目录替换、symlink/junction及核验后交换；拒绝后可显式重试。

CJ-03提交446efcb。CJ-04：spawn前durable InputOwnership，仍为format2可选字段；恢复没有进程身份但已确认退出时按输入inode/device清理，不放宽未知FIFO白名单。Windows core/records全套通过；Linux普通用户 core/records/engine全套 -count=1通过（含新TestOrphanFIFOIdentityGapExpires、TestUnknownFIFORetentionRefused），证明身份未写/child退出→reopen→stop→prune，外来FIFO仍拒绝。仅helper自动测试，真实Dota回收待验收。

CJ-04提交303f0fe。CJ-05增加内存轮转游标，每周期仍最多limit次尝试；失败保留重试，后续候选不被固定前缀饿死；manager重启游标重置但持续维护会前进。Windows TestRetentionFailedPrefixCannotStarve通过：17项前16失败，第二轮第17成功，移除外来阻塞后16项可重试并同步清除keys/operations。Linux合并回归待执行。

CJ-05提交cccb9b4。CJ-06统一observe失败时room/evidence失效，未知身份保持unknown；watch仅状态/错误语义变化保存，不改历史成功operation。Windows TestObservedFailureInvalidatesReadyOnce通过：ready后failureAny、日志非普通文件、binding错误三种；2.2秒内持久化恰好一次，之后stop可回收。Linux全套合并验证待执行。
