# 本地核心操作（开发版本）

真实引擎验收状态见 [M1 记录](validation/m1.md)。M2—M4 尚未完成，不用于接管生产。游戏、运行库和两端匹配的地图资源由使用者准备；核心不下载或更新它们。

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

管理端退出保留专服；重新启动必须使用同一 data-dir。当前版本尚未完成 M2 的全部中断恢复，遇到 unknown 或持久化故障时保留现场和日志，不能删状态或重新 create 来掩盖残留。数据损坏、不兼容版本、目录不可写均明确报错，不自动换目录。不要同时用外部工具修改核心生成的 cfg 或运行记录。

当前持久化格式为2，模板与协议仍为1。格式1实验目录不会自动升级；保留旧目录及证据，确认旧实例已停止回收后为新测试使用独立目录。state.json 与 store.marker 缺失、不匹配或校验失败时保留现场，不手工删标记或修改校验和来绕过检查。启动恢复不自动派生游戏进程；操作返回 INTERRUPTED 时先查询实例状态，确认并显式停止回收后，再以新创建意图申请实例。整机重启验收仍未完成，详见 [M2记录](validation/m2.md)。

普通创建/重启不修改 gameinfo 或 VAC 配置。本地管理不监听 TCP；Windows 用命名管道，Linux 用 Unix socket，不需要给管理 API 开放公网防火墙端口。游戏端口的外部可达性应单独验证。
