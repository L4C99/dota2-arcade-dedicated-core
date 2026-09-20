# Go 本地协议客户端

这是外部 Go 程序可导入的客户端库，不是服务端或常驻管理器。调用 New(dataDir)、Call(ctx, method, params)，使用与管理端相同用户和绝对 data-dir。

契约见 [本地 API](../docs/local-api.md)，示例见 [launcher](../examples/launcher/README.md)。传输错误可能意味着结果未知，应保留原键与参数；create/restart/stop 受理后轮询 operation。

client_test.go 是开发自动测试，不随 Release 运行包分发。Go 集成通过源码模块获取本库；普通 CLI 用户不需要 Go。
