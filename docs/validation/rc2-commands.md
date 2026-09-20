# RC 命令验收矩阵与 RC.2 范围

2026-09-21，用户授权补齐命令验证、帮助/版本修正、交付文档及新候选包。无生命周期、协议、状态格式或地图代码功能变更。正式生产 Release 仍待最终候选确认。

## RC.1 发布包基线验证

实际使用固定提交 a04e6022470f2de6864ab94c6113a494b0e20592 的已交付二进制。Windows 本机完整用户令牌、Linux Ubuntu 普通 steam-fetch 用户，在全新隔离 data-dir 上执行；生命周期使用仅监听回环 TCP/UDP、输出固定就绪日志并处理 quit 的受控测试引擎，不启动 Dota。A2S 仅修改临时 gameinfo 夹具。

| 命令/场景 | Windows RC.1 | Linux RC.1 |
| --- | --- | --- |
| version JSON、提交、版本字段 | 通过 | 通过 |
| 全部子命令 help、无参数、未知命令、create缺参数 | 通过（帮助旧退出码已记录） | 通过（帮助旧退出码已记录） |
| check 合法模板、重复JSON字段拒绝 | 通过 | 通过 |
| serve 自定义范围、history-days、min-free-mib、非法值与范围过大、同目录锁 | 通过 | 通过 |
| create 自动端口、范围耗尽、范围外显式端口 | 通过 | 通过 |
| 相同key重试返回原实例/操作，变更请求冲突，回收后同key仍返回历史 | 通过 | 通过 |
| list 空/最终空、status ready/历史/not-found、operation轮询 | 通过 | 通过 |
| logs tail=1/1000、非法1001、generation=1历史日志 | 通过 | 通过 |
| restart 新代次沿用端口、管理器恢复后重新ready | 通过 | 通过 |
| stop 回收、reclaimed不能restart、早退failed后显式stop | 通过 | 通过 |
| 高磁盘阈值阻止create | 通过 | 通过 |
| a2s enable 插入、重复幂等、冲突拒绝 | 通过（夹具） | 通过（夹具） |
| m0-inspect 无输入、六个路径参数、非法相对路径 | 通过 | 通过 |
| launcher-example 帮助、非法hold、指定port/host/key/hold创建与自动回收 | 通过（假引擎） | 通过（假引擎） |

Windows 最终73次、Linux最终71次命令调用全部符合预期（包括重复轮询，数量不是独立场景数）。原始 argv、stdout/stderr、退出码与汇总保存在本地验证目录，未包含在公共包内。Windows二进制SHA256为 fa033a959c2b1f6ca5297864a3f25f84e99df6d48f8f61d407454983a2dbbac2；Linux为0960c9715c7f46094a2b96f6c23b7c1a3c94b8a41dae4f3dad40aec73d12cbee。

准备过程保留失败证据：受限Windows令牌无法运行私有目录测试，改用完整用户令牌；测试脚本最初未等待恢复观察的loading阶段，修正轮询后通过；Linux ZIP解压未保留launcher执行位，补执行位后通过。没有因此修改核心生命周期代码。

## RC.2 修改和产物复验

- 顶层 `help` / `--help` / `-h` 列出全部公开命令；支持 `help <command>`，子命令帮助退出0。无参数仍返回完整用法及退出2。
- 版本由打包工具 `--version 0.1.0-rc.2` 写入二进制和 BUILD.json；源码直接构建仍标0.1.0-dev，不伪造已发布版本。
- 交付勘误、工坊ID模板和验收记录加入新包；RC.1 ZIP/tag不覆盖。
- 已对本轮源码运行全套 go test ./... 与 go vet ./...，通过。针对帮助无副作用及注入版本添加回归检查。
- RC.2 必须从远端固定提交的干净checkout重新构建，再对 Windows/Linux 原生产物重复上述矩阵，并验证顶层help、帮助退出0和注入版本。最终产物结果记录在随包的独立验证汇总，不把本构建前文档当作产物已通过的证据。

## 不扩大结论

本矩阵覆盖全部公开命令及所列主要参数场景，不代表所有参数组合、所有错误分支或任意地图兼容。保留期到期删除、磁盘真实耗尽、A2S真实网络查询、整机重启等不在本次矩阵重测范围；既有阶段验证需明确标为旧证据。RC.1真实Dota烟测见 [记录](rc1-smoke.md)，不替代RC.2最终产物进房确认。内部 __engine-quit 仅通过 stop 生命周期间接使用，不作为用户接口直接调用。
