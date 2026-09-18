# 配置示例

[Windows 模板](template.windows.json) 是 draft-2 格式草案，展示参数数组、cfg 行与占位符的位置。当前程序只有 m0-inspect，尚不能加载此模板启动房间；M1 将固定格式与校验规则。

所有路径均为示意。将安装路径替换为自己的绝对路径，将 example_map 替换为 VPK 内实际地图名。sample_addon.vpk 表示两端可解析的 addon 标识；使用者须自行准备匹配资源，不代表核心会下载或分发文件。

{{game_port}}、{{log_path}}、{{cfg_name}} 与 {{instance_id}} 是计划中的核心占位符，当前尚未实现展开。Linux 正式模板在对应 Go 启动适配完成后补充；M0 实机成功不表示已有通用部署模板。
