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
