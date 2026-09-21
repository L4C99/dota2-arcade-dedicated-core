# v0.1.1 配置示例

[Windows](template.windows.json) 和 [Linux](template.linux.json) 展示模板 v1。字段、错误和占位符规则以 [本地协议](../docs/local-api.md) 为准。配置模块已提供静态校验和展开；正式生命周期的实现与实机验收状态见 [M1 记录](https://github.com/L4C99/dota2-arcade-dedicated-core/blob/main/docs/validation/m1.md)，示例本身不表示已经通过进房验收。

使用前须替换程序、工作目录和 cfg 目录为目标机器上已有的绝对路径，并将 `example_map` 改成 VPK 内实际地图名。将 `REPLACE_WITH_WORKSHOP_ID` 替换为实际数字工坊 ID。标准部署布局为 `game/dota_addons/<工坊ID>/pak01_dir.vpk`，客户端订阅对应原游廊地图，`customgamemode` 填工坊 ID，而不是服务器绝对路径。核心不下载或分发文件，也不验证工坊 ID 与资源是否对应。上述提示文字是普通文本，静态 check 不会替你判断是否已正确替换。

例如本轮 n6 验证使用 `map n6 gamemode=15 customgamemode="3564393242" nomapvalidation=1`。服务端使用经验证的联机修改版，客户端使用原订阅资源；两端须版本及协议兼容，不要求修改版与原版 SHA256 相同。应分别记录资源版本/哈希。自定义别名 VPK 的早期测试要求客户端额外准备同名资源，不作为标准玩家部署方案。绝对路径方式按用户实测要求双方具备同路径、同名 VPK，不适合作为跨平台默认示例，也不承诺适用于任意地图。

本轮 n6 / windy_standalone_start 协议 v2 在 Windows、Linux 均观察到以下 successAll；仅适用于该版本，不直接套用其他地图：

```json
"successAll": [
  "ss_loading -> ss_active",
  "Host activate: Loading (n6)",
  "[StandaloneServer] command-ready: command=windy_standalone_start, protocol=v2, map=n6"
]
```

没有验证过的地图失败短语时可使用 `"failureAny": []`，进程退出和启动超时处理仍保留；不能凭空填写通用 Error 文本。日志 ready 不等于 Steam 登录完成或客户端进房通过。

`readiness` 中的 `REPLACE_WITH_VERIFIED_...` 是需要替换的普通文本，不是核心占位符。应按目标地图真实本代日志填写加载、Host activate 和地图级就绪证据，以及已确认的失败信号。所有 successAll 字面规则均命中且进程仍存活才可能就绪；不能只用端口开放或通用 `ss_active`，也不能直接套用未经验证的其他地图规则。保留这些示意文本会导致观察不到就绪并超时。

核心占位符仅有 `{{instance_id}}`、`{{game_port}}`、`{{cfg_name}}`、`{{log_path}}`。argv 中必须各占完整参数；cfg 中可在行内引用。必需的 -port、-con_logfile、+exec 参数对各出现一次。核心逐个传递 argv，不经 shell；模板本身仍属于受信任的本机配置。

日志绝对路径须使用 ASCII 字符，可含空格，中文日志内容不受此限制。静态 check 只读取路径，不测试目录写权限、端口可用性或任意 cfg 引用；实际启动前仍需预检。默认超时为启动 120 秒、优雅停止 10 秒、强制退出确认 5 秒。

M1 使用显式端口。示例保留默认 VAC 和 `sv_hibernate_when_empty 0`，核心不向自定义模板强制注入这些设置。Linux 运行库与资源权限须由使用者准备；此示例不要求 root 或 systemd。

Linux 启动管理器前需要准备同一普通用户可访问的 Steam SDK、HOME 和动态库搜索路径；游戏继承管理器环境，恢复管理器时也要保留这些设置。完整命令见 [操作说明](../docs/operations.md#linux-运行环境)。不要只确认库文件存在，还要确认该用户可以遍历全部父目录。

## 托管实例生命周期策略

两个标准模板保留 `sv_hibernate_when_empty 0`，并显式设置 `dota_quit_after_game 0`：空服不依赖 Dota 自动 hibernate，一局结束后不依赖 Dota 自行退出，由 d2core / 上层管理端显式执行 stop → reclaim。它们是可按场景修改的示例部署策略，不是 protocol v1 或模板 schema 的硬要求，核心不强制注入这些 cvar。未加入其他 quit / hibernate 参数。
