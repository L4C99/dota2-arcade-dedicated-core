# 配置示例

[Windows](template.windows.json) 和 [Linux](template.linux.json) 展示模板 v1。字段、错误和占位符规则以 [本地协议](../docs/local-api.md) 为准。配置模块已提供静态校验和展开；正式生命周期的实现与实机验收状态见 [M1 记录](../docs/validation/m1.md)，示例本身不表示已经通过进房验收。

使用前须替换程序、工作目录和 cfg 目录为目标机器上已有的绝对路径，并将 `example_map` 改成 VPK 内实际地图名。`sample_addon.vpk` 是两端均可解析的 addon 标识；使用者自行准备匹配资源，核心不下载或分发文件。不要将只在服务端有效的绝对路径当成跨机客户端资源标识。

`readiness` 中的 `REPLACE_WITH_VERIFIED_...` 是需要替换的普通文本，不是核心占位符。应按目标地图真实本代日志填写加载、Host activate 和地图级就绪证据，以及已确认的失败信号。所有 successAll 字面规则均命中且进程仍存活才可能就绪；不能只用端口开放或通用 `ss_active`，也不能直接套用未经验证的其他地图规则。保留这些示意文本会导致观察不到就绪并超时。

核心占位符仅有 `{{instance_id}}`、`{{game_port}}`、`{{cfg_name}}`、`{{log_path}}`。argv 中必须各占完整参数；cfg 中可在行内引用。必需的 -port、-con_logfile、+exec 参数对各出现一次。核心逐个传递 argv，不经 shell；模板本身仍属于受信任的本机配置。

日志绝对路径须使用 ASCII 字符，可含空格，中文日志内容不受此限制。静态 check 只读取路径，不测试目录写权限、端口可用性或任意 cfg 引用；实际启动前仍需预检。默认超时为启动 120 秒、优雅停止 10 秒、强制退出确认 5 秒。

M1 使用显式端口。示例保留默认 VAC 和 `sv_hibernate_when_empty 0`，核心不向自定义模板强制注入这些设置。Linux 运行库与资源权限须由使用者准备；此示例不要求 root 或 systemd。
