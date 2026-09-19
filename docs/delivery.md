# 双平台开发交付包

当前为开发验收包，尚非正式发布。M2整机重启与M4完整验收未通过前不得用于正式接管生产。目标为已测试的Windows 10 x64、Ubuntu 24 x64；包中不包含游戏、地图、运行库或凭据，不提供许可证。

每个平台ZIP包含d2core、launcher-example、BUILD.json、平台模板、协议和操作说明；附独立.sha256校验文件。运行两个可执行文件不需要Go、Python或PowerShell。M0实验说明仅供参考，实验脚本可在源码仓库取得，不是产品运行依赖。

## 使用者步骤

1. 核对SHA256后解压到可写的ASCII路径，例如Windows的C:/d2core-test或Linux用户目录下d2core-test；Linux确认d2core与launcher-example可执行。运行 `d2core version --json`，核对BUILD.json中的提交和构建时间，gitDirty应为false。
2. 准备Dota专服安装、依赖和用户地图VPK，保证客户端解析同一addon标识。核心不会下载或更新共享资源。复制对应examples模板到自己的配置目录，填写绝对路径及实测就绪规则。
3. 以同一普通用户运行 `d2core check --template <绝对模板> --json`；通过只代表静态检查，不代表能进房。
4. 选择全新data-dir并运行 `d2core serve --data-dir <绝对目录> --port-min <已预留起点> --port-max <终点> --json`。不要将生产目录、旧版本数据或其他用户的目录作为试用位置，不需要开放管理API的TCP端口。
5. 在另一终端运行create并查询operation/status，客户端实际进房；再执行restart、再次进房、stop，确认历史可查及回收。完整命令见operations.md，也可运行launcher-example演示受限本地协议流程；示例默认30秒后停止自己的房间。
6. 同一data-dir只能有一个管理端。管理端退出不结束游戏；错误时保留实例ID、创建键和现场，先核验再stop，不按通用进程名杀进程。运行包更新前结束并回收实例，备份需要的历史；游戏/VPK更新由运维安排停服后完成，不由核心执行。

A2S是显式独立工具，见a2s.md。仅在隔离配置上验证，不能将同机客户端安装或生产gameinfo当作可随意修改的测试文件。磁盘告警、离线日志边界及历史期限见operations.md和local-api.md。

## 开发者构建

固定Go 1.27.1，使用干净Git工作树，在仓库根执行：

```text
go run ./tools/package --go <Go绝对路径> --output dist --build-time <本次构建UTC时间，RFC3339>
```

build-time可省略并使用实际当前时间；复现同一包时必须传入原BUILD.json的buildTime。sourceTime与buildTime分别记录提交时间和构建时间。程序拒绝脏工作树，使用固定文件清单、GOARCH=amd64、CGO_ENABLED=0、trimpath、稳定归档顺序和时间；不包含local、data、VPK、私钥或原始日志。已存在的输出不覆盖。构建/归档失败可能留下无校验文件的不完整产物，应核对后清理或改用新的输出目录。

交叉构建只是生成文件，不能代替目标平台实机运行。CI只做辅助测试与构建；不自动连接测试主机、不运行Dota、不创建GitHub Release。完整交付需汇总M0—M4真实证据后另行记录。
