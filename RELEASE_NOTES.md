# v0.1.2 发布加固

发布版本：**v0.1.2**。正式发布身份由 v0.1.2 tag、精确源码提交、Release 资产、BUILD.json 和公布的 SHA256 摘要共同确定。

## 产物来源与核验

- 固定 LF checkout，并逐字节核对受版本控制的构建输入与 Git blob；不一致时拒绝打包。
- ZIP 静态文件逐字节核对固定源码提交。
- 机器生成并核验 Manifest 和校验值；支持外部提供的 SHA256 身份锚点。
- 构建器、验证器和双平台 CI 脚本共用 VERSION 入口，以回归测试防止契约漂移。
- 操作示例使用版本中性路径；不修改 Runtime。

## Runtime 边界

生命周期、本地协议、进程身份核验与恢复、持久化、端口分配和就绪匹配引擎均保持不变。兼容身份仍为 protocolVersion=1、模板 schemaVersion=1、磁盘 formatVersion=2。本补丁不增加 Runtime 功能或 client API。

## 发布加固内容

- Windows/Linux amd64 自包含归档除操作/API 文档和示例外，还包含 LICENSE、LICENSING.md、CHANGELOG.md，以及 THIRD_PARTY_NOTICES.md 中的第三方再分发文本。
- 公网 Dota 模板的 successAll 必须在已验证的地图/脚本就绪规则之外，包含精确的 `SV:  Connection to Steam servers successful.`。这是模板契约，不是引擎新增特判。
- 打包回归检查覆盖必需文件、归档路径、构建身份、模板日志规则和 SHA256；CI 在两个平台原生运行包内程序核验版本。

Steam 连接成功是必要条件，不是充分条件。Ready 不证明 NAT/防火墙可达、JoinInfo 映射正确、Steam URI 行为或真人进房成功，部署方仍须分别验收。不要为了更快达到 Ready 而删除出现较慢的就绪日志规则。

## 安装与限制

参见[交付说明](docs/delivery.md)、[操作说明](docs/operations.md)、[模板说明](examples/README.md)、[本地 API](docs/local-api.md)和 [A2S](docs/a2s.md)。游戏二进制、SDK、运行库及兼容地图需另行准备。仅支持同用户本地管理及 ASCII 路径；IPv6 通配监听的双栈覆盖仍未验证；不承诺容量，也不自动迁移格式 1。failed 实例须显式 stop/reclaim。m0-inspect 保留为非生产用途的历史诊断命令。

产物检查不构成新一轮真实 Dota 验收。本版本保留既有已验收 Runtime 行为；发布加固验证覆盖分发材料、源码来源、可重复构建和产物身份。

## 授权与历史资产

参见 [LICENSE](LICENSE)、[历史授权范围](LICENSING.md)及[第三方声明](THIRD_PARTY_NOTICES.md)。**历史 v0.1.1 tag 与 Release 资产保持不变**，不重建、替换或重新标记；新增材料纳入 v0.1.2 归档，不改变历史资产。
