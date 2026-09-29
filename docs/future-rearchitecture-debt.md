# Future Core + Platform Re-architecture Debt Ledger — d2core

本账本只保存当前文档明确确认、在稳定版本中仍需人工处理的遗留技术债，供未来获得单独授权的 Core 重构参考。它不安排 v0.1.3 工作，也不改变 v0.1.2 的 Runtime、协议或发布身份。下列历史验证记录按形成时的事实引用；当前操作以 [本地协议](local-api.md) 和 [操作说明](operations.md) 为准。

未来功能不等于技术债。受限控制台命令、自动 `status_json` 和原始输出增量接口仍属于 [路线图的可能扩展](roadmap.md#后续扩展不在当前交付范围)，不能因尚未实现就列为未修缺陷。

## CORE-DEBT-001 — 旧 format2 cfg 缺少可证明的文件归属

- Status: ACCEPTED DEBT / FUTURE RE-ARCHITECTURE。
- Source: [CJ-03 修复与旧记录边界](validation/review-fixes.md)、[升级遗留记录](operations.md#无归属-cfg-与升级前遗留-fifo)。
- Current limitation: CJ-03 为 format2 增加可选 `cfgOwnership` 后，旧 format2 记录仍可打开，但既有 cfg 若缺少该归属证据，或文件对象已被替换，`stop` 不会仅凭路径、名称或相同内容删除它；清理可能保持 `cleanup=failed`，阻止该实例完成回收。
- Why not fixed now: 旧记录无法事后证明当前文件就是 Core 创建的对象。为让稳定版本自动清理而推测归属，会带来删除外来文件的风险；当前版本保留文件并要求人工核实。
- Current safety boundary: 核对 cfg 父目录对象、文件对象和内容指纹；证据不足时拒绝删除、保留实例及分配，允许在人工处理确切文件后重试同一实例的 `stop`。不得猜测归属、按名称批量删除或递归清空目录。
- Future direction: 新 Core 从创建前到回收后保持可验证的生成文件归属；为旧记录设计独立、可审计的迁移或人工处置流程。无法证明归属的旧 cfg 仍须保留，不能靠新格式声明自动变成可删文件。
- Cross-project impact: Platform / Node Controller 只能以 `reclaimed / stopped / cleanup=complete` 作为释放相应实例资源的依据；遇到清理失败应保留实例关联信息并提示运维，不绕过 Core 的归属判断。
- Revisit trigger: 设计新持久化格式或迁移旧 data-dir，或旧 cfg 清理失败在实际运维中反复出现时。

## CORE-DEBT-002 — 旧代次 stdin.fifo 缺少输入文件归属

- Status: ACCEPTED DEBT / FUTURE RE-ARCHITECTURE。
- Source: [CJ-04 修复与未知 FIFO 边界](validation/review-fixes.md)、[升级遗留记录](operations.md#无归属-cfg-与升级前遗留-fifo)。
- Current limitation: 升级前“已尝试 spawn、身份未落盘、子进程已退出”的旧记录可能同时没有 `InputOwnership` 和 `Identity`，但留下 `stdin.fifo`。当前 Core 不推测该 FIFO 的归属；实例即使显示 `reclaimed / cleanup=complete`，到期 retention 仍可能报 `CLEANUP_FAILED`，并保留历史、操作及创建键。
- Why not fixed now: CJ-04 已对新启动流程在 spawn 前持久化 `InputOwnership`，但旧残留没有可追补的原生归属证据。扩大清理白名单或仅按文件名删除会误删非本代文件。
- Current safety boundary: 未知 FIFO 保留并报告清理错误。运维须核实确切 run 目录、FIFO 类型、所属实例及无进程使用，才可处理单个遗留文件；随后等待在线维护重试并检查 `list.storage`。不能批量删除 FIFO。
- Future direction: 新 Core 将输入文件的创建、持久化归属、进程身份与到期回收作为同一可恢复生命周期设计；对旧记录提供有证据的人工迁移路径，不自动认领未知文件。
- Cross-project impact: Platform / Node Controller 若依赖 Core 的历史和创建键保留期限，应监测 `list.storage` 的清理错误；不能将实例已回收等同于历史已到期删除。
- Revisit trigger: 设计新恢复/保留模型或迁移旧 data-dir，或旧 FIFO 导致长期 retention 失败时。

## 已评估但不登记

- 已修复：CJ-01—CJ-11、LX-01/LX-02 和 Windows 早退分类/terminal 后 restart 时序问题，见 [最终复核](validation/review-fixes.md#最终验收结论2026-09-20) 与 [Windows 时序修复](validation/windows-timing-fixes.md)。历史失败仍留在原记录，不代表当前仍未修。
- 已裁定的当前边界：`portCheck=partial` 对未知 UDP 用途和 IPv6 通配双栈属性如实报告；进程身份无法确认时拒绝认领/控制；同用户本地 API、ASCII 日志路径和日志离线期间的运维磁盘保障均按现行合同处理。现有证据未把这些边界确认为待重构缺陷，见 [技术决策](decisions.md)、[M3 策略](validation/m3.md) 与 [本地协议](local-api.md)。
- 旧格式与容量合同：格式 1 实验目录不自动迁移、format2 `state.json` 的 64 MiB 上限及保留期内不提前淘汰记录均已明示；现有证据没有把集中状态文件本身裁定为待重构债，见 [M2 记录](validation/m2.md)、[M3 记录](validation/m3.md) 与 [本地协议](local-api.md)。
- 未来功能：受限控制台命令、自动 `status_json`、原始输出增量接口属于可能扩展，不登记为已知缺陷。
- 环境与外部兼容：Steam Directory / CM 网络不稳、测试机公网连通、游戏依赖准备、Steam URI 冷启动或 NAT 映射兼容现象，没有形成 Core 架构缺陷裁决，见 [复核记录](validation/review-fixes.md) 与 [A2S 说明](a2s.md)。

本账本只表达已核实的遗留处理成本与未来设计方向。任何跨项目接口或迁移行为，均须在新项目中分别设计、授权和验证。
