# GitHub 与本地凭据

仓库：L4C99/dota2-arcade-dedicated-core。用户已明确选择公开、暂不添加许可证。公开不代表已有开源许可证授权；本项目当前仅完成 M0 运行验证。

本工作区采用独立的可写 Deploy key，只授予该仓库 Git 访问权限；不复用 Ubuntu 登录密钥，不要求 GitHub CLI OAuth 登录。私钥仅在本机忽略目录保存，Windows DACL 限制为当前用户和 SYSTEM，公钥在 GitHub 仓库 Settings → Deploy keys 管理。需要撤销时只删除该项目的 Deploy key，不影响 Ubuntu 连接。

SSH 使用项目专用配置和 known_hosts，严格验证主机密钥；主机公钥取自 GitHub 官方 HTTPS 元数据并核对指纹。主机密钥变化必须重新核验，不能关闭 StrictHostKeyChecking。仓库本地 core.sshCommand 指向该配置，不改用户全局 SSH/Git 配置。

本机连接配置、私钥、原始日志、VPK 和二进制均不提交。上传前检查当前跟踪文件和整个提交历史；模式扫描只是检查手段之一，不声称可以发现所有敏感数据。

日常代码同步使用普通 git push，不强制推送。Deploy key 必须勾选 Allow write access 才能推送；只读 key 可以通过 SSH 身份验证，但 push 会被拒绝。仓库推送和 CI 结果单独记录，CI 通过不代替真实 Dota 实机验收。

如需在其他机器工作，应单独配置凭据，不从仓库获取或分发本机私钥。没有提供许可证、游戏文件或第三方地图内容。
