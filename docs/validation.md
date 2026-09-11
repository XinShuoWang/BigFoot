# 历史验收记录

以下是从原 README 迁移的历史记录，不代表当前环境或最新代码已重新验收。返回 [README](../README.md)。

## 2026-09-10

Go 回归、race、vet、Linux amd64 构建及 16 项 Python 测试通过；协议 fuzz 和当时的已知漏洞检查通过。云端两端健康、真实 Mihomo HTTPS 和错误密码拒绝通过。部署工具通过模拟 FC 的失败恢复测试，并已实际部署上海 Gate 与香港 Voyager，完成完整链路验收。

尚未实测各手机 GUI、长连接极限、容量、冷启动统计及实际费用。

- 此前发布构建使用 Go 1.27.1。
- 实际验证客户端为 Mihomo v1.19.30（macOS arm64）。
- 当次浏览器 OAuth 授权能够访问 RAM，但访问 FC 返回 `missing parameter SecurityToken`，实际部署改用专用 RAM 用户的 AK profile。
