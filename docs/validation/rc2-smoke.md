# RC.2 双平台真实游戏最终烟测

日期：2026-09-21（北京时间）。本记录补充原交付验证汇总中“真实进房待确认”的状态，不修改已发布候选 ZIP、tag 或原 SHA256SUMS。

- Tag：v0.1.0-rc.2
- 完整提交：9bbb86c0c7260687dfa8f35dd0376e7916048ae0
- 两平台使用新解压的 RC.2 包，版本 0.1.0-rc.2，gitDirty=false。
- 场景：n6，customgamemode=3564393242；服务端 dota_addons/3564393242/pak01_dir.vpk 使用此前验收的联机修改版，SHA256 27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c。
- 两平台首次创建后进房、restart 后重连操作均由用户分别确认“正常”；随后 stop 操作成功，最终 reclaimed / stopped / cleanup=complete，27015 无监听，测试管理器已退出。

## Linux

实例 i_107dfe6a30a0a4eeafbb2d20cc33b242；generation 1 PID 46819，generation 2 PID 47224。
create：o_49e32fb569da22254fcbc75ddce1ebbc。
restart：o_db79331ec57c843924158b7c7934d700；新进程 Steam 登录成功，用户重连正常。
stop：o_f9809fcfd5c419145e232769fb7dc0a2；2026-09-20T18:09:51.502786239Z 完成。

## Windows

隔离 Windows 测试虚拟机，使用普通测试账户。
实例 i_8aeb240ee693d700eb162fa51d6a4dd9；generation 1 PID 7828，generation 2 PID 2696。
create：o_bea6110a81a2577c5805317dd25fa3a7；首次进房用户确认正常。
restart：o_77b62e993b6ed50ed66f2c9bae7fe05b；2026-09-20T18:20:08.5838273Z 完成，18:20:45Z Steam 登录成功；用户重连确认正常。
stop：o_e95f9ba04395f8bd7b0b79cdc8322e76；2026-09-20T18:22:45.2593199Z 完成。
回收后停止专用管理器计划任务；临时任务和防火墙规则已删除，批处理登录权限恢复基线，测试账户恢复禁用。地图及 gameinfo.gi 哈希未变。历史记录保留。恢复验证时间 2026-09-20T18:23:41.0814227Z。

## 范围与下一步

本轮覆盖真实创建进房、重启重连和显式停止回收。未重新执行真实双实例隔离、管理器离线时客户端操作、整机重启、长时稳定性或真实 A2S 网络测试；不将以往结果扩写为本轮覆盖。IPv6 wildcard dual-stack coverage 仍是已知未验证项。
本轮无产品代码变更。RC.2 双平台上述真实烟测通过；尚未创建正式生产 Release。正式 v0.1.0 的标记、构建与发布仍需单独确认，不能将 RC.2 二进制仅改文件名视为正式版。

## 已验收候选包 SHA256

Windows：7263ef14084edbc30c53473797caba22d3309a3bf4fe026c37527620a6a16525
Linux：83fd3f748a36faae9c9f1d085f5381dbf586c747f1d2f8bfd3fec5b302eb81ad