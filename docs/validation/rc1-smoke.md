# v0.1.0-rc.1 发布产物烟测

记录日期：2026-09-21（Asia/Shanghai）；Linux 操作日志时间为2026-09-20 UTC。用户执行并确认真实客户端结果；Linux 失败诊断另经只读服务器日志核实。未创建正式生产 Release。

## 固定产物

- tag：v0.1.0-rc.1；提交：`a04e6022470f2de6864ab94c6113a494b0e20592`。
- Windows ZIP SHA256：`9b3e62dc82ae6bbcb46c360c45a6b18b3909610009511422e825e48d035d92dd`。
- Linux ZIP SHA256：`b45485d394c8446cea16da022724ba4dec1aabb3e560f2a167547167be0b6e06`。
- 本文为包外补充记录；原包、BUILD、校验清单、manifest 及 tag 均不覆盖。程序版本字段仍为0.1.0-dev，固定提交及包校验值用于辨识 RC。

## 范围和结果

| 项目 | Windows | Linux |
| --- | --- | --- |
| 数字工坊 ID 创建、客户端进 n6 并操作 | 用户确认通过 | 用户确认通过 |
| 重启后再次进房 | 用户确认整套流程正常 | 单独确认通过，generation=2 |
| 管理器退出及恢复期间游戏正常 | 用户确认整套流程正常 | 用户确认正常；恢复状态 running/ready |
| 显式停止、回收 | 用户确认整套流程正常 | operation succeeded，reclaimed/stopped/cleanup=complete |

Windows 根据用户整体流程确认记录，不冒充拥有每一步完整原始输出。Linux 本轮未专门证明真实 PID 重用、主机重启、多实例隔离或 A2S；旧阶段验收不冒充本轮覆盖。最终停止后已提示用户退出管理器，未另行确认管理器已退出。

## 资源与环境

标准模板填 `customgamemode="3564393242"`；服务端布局 `game/dota_addons/3564393242/pak01_dir.vpk`。Linux 服务端使用先前验收的联机修改版，SHA256 为 `27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c`；客户端使用原游廊订阅资源。不要求修改版服务端与客户端原版字节相同，必须实际验证兼容性。不记录与项目无关的客户端问题。

Linux 普通运行用户为 steam-fetch；game=/srv/dota2/client/game，独立测试根=/srv/d2core-rc1-smoke。SDK 副本位于测试根 home/.steam/sdk64，管理器使用对应 HOME 与 LD_LIBRARY_PATH。未改产品代码、系统动态库配置或旧测试用户私有目录权限。

首次 run 缺少库搜索路径：libserver.so 无法加载 libv8.so，进程退出。run02 加入库搜索路径，但 steamclient.so 位于另一测试用户不可遍历的私有父目录，Steamworks 初始化失败。两次属于烟测准备不完整，不计为通过；失败记录保留，显式回收后才准备新轮。run03 使用普通用户可访问的独立 SDK/HOME 和库搜索路径，完成本轮流程。这些运行环境前提已补入操作说明。

## Linux 最终证据

- 实例：`i_240f2408bb6125b1737f33deb2924eb0`，端口27015，最终generation=2。
- 管理器恢复后用户贴出的状态：active/running/ready，PID41486，error=null，三个 n6 就绪规则命中。未单独保存本记录中的离线前 PID 对照，因此不扩大为完整身份对照证明。
- 停止操作：`o_3a918b07274d31291d64719fc0147be7`。
- 开始 `2026-09-20T17:20:17.228190774Z`，完成 `2026-09-20T17:20:17.826555359Z`，约0.60秒，status=succeeded，error=null。
- 最终 lifecycle=reclaimed、process=stopped、cleanup=complete；旧 evidence.valid=false。bindings 是停止前观测快照，不能解读为停止后仍监听。portCheck.partial 为 IPv6 双栈覆盖未验证，没有已报告冲突，不宣称端口覆盖检查完整。
- 原始日志和辅助脚本响应保留在服务器测试根 run、run02、run03；不向公共仓库提交玩家日志或本地访问凭据。

## 结论

两平台本轮所列单房间发布产物烟测通过。资源部署示例、Linux 运行环境和操作步骤需要勘误；本轮没有确认需要修改产品代码的阻塞项。修订文档不追溯改变 RC.1 包内容，若后续需要包含新文档的发布包，应另行制作候选版本。保持生产 Release 暂停。
