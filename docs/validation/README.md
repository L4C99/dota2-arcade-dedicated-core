# 验证资料索引

本目录保存真实研发与测试的历史证据，不是当前部署操作手册。记录中的失败、暂停、待验证等中间状态反映当时事实；不要将某一历史片段直接当作当前版本状态。

当前稳定产品状态以[项目首页](../../README.md)、[正式 v0.1.1 Release](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1) 和[固定 tag 源码](https://github.com/L4C99/dota2-arcade-dedicated-core/tree/v0.1.1)为准。后续版本以相应正式 Release 和固定 tag 为准。

| 资料 | 验证范围 |
| --- | --- |
| [M0](m0.md) | 双平台引擎启动、资源、就绪与退出的早期可行性验证 |
| [M1](m1.md) | 本地契约、模板、实例记录与创建/停止生命周期 |
| [M2](m2.md)、[重启补测](m2-reboot.md) | 持久化、身份核验、恢复、幂等及整机重启 |
| [M3](m3.md) | 多实例、端口分配、隔离、空间与历史回收 |
| [M4](m4.md) | 双平台交付、A2S、外部调用与打包验证 |
| [RC.1 烟测](rc1-smoke.md) | 历史候选包的真实环境验收 |
| [RC.2 命令检查](rc2-commands.md)、[烟测](rc2-smoke.md) | 历史候选的命令与真人进房验证 |
| [复核修复](review-fixes.md) | 历史独立复核、修复及其边界 |
| [CI 健康检查](ci-health.md) | 历史失败分类、runner 条件与检查结果 |
| [Windows 时序修复](windows-timing-fixes.md) | 早退分类与 terminal 后 restart 竞态的根因及回归证据 |

review/repair 类材料属于历史过程记录，不是新的生产管理接口或长期功能承诺。原始私有日志、玩家数据、凭据和游戏资源不应提交；开发/验证工具仅用于非生产环境，见[开发说明](../development.md)。
