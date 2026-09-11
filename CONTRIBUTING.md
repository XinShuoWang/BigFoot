# 开发说明

面向参与代码开发的人。使用和部署见 [README](README.md) 与[部署运维文档](docs/operations.md)。

## 本地验证

需要 Go 1.25+、Python 3；Mihomo smoke 另需 OpenSSL、curl 和 Mihomo。涉及云 SDK 的测试需先安装 `requirements-cloud.txt`。

```bash
go test -race -count=1 -timeout 90s ./...
go vet ./...
python3 -m unittest discover -s tests
go build -o ./tunnel ./cmd/tunnel
python3 scripts/smoke_mihomo.py ./tunnel /path/to/mihomo --auto-identity
```

本地 smoke 自动生成临时凭据、启动 Gate/Voyager/Mihomo、测试 HTTPS 与错误密码，然后清理进程。程序参数可用 `./tunnel -h` 查看，`-check` 仅校验配置；参数对应 `BIGFOOT_` 大写下划线环境变量，命令行优先。`local` 模式是仅监听回环地址的 HTTP CONNECT 适配器。

## 代码结构

```text
cmd/tunnel/          服务配置与启动
internal/identity/   Voyager 身份与证书自动续签
internal/ingress/    Trojan over WSS 入站
internal/tunnel/     Gate/Voyager 内层 TLS、认证及目标策略
internal/service/    会话限制、健康检查与排空
scripts/deployment/  配置、构建、凭证、FC 适配、编排和验收
scripts/             CLI、兼容工具与客户端测试
tests/              Python 工具测试
```

## 服务配置约定

命令和配置统一使用 `gate` / `voyager`；服务模式为 `-mode gate` / `-mode voyager`，Gate 上游地址参数为 `-voyager-url`（环境变量 `BIGFOOT_VOYAGER_URL`），内层证书主机名为 `voyager.internal`。

历史验收结果见[验收记录](docs/validation.md)，不能替代当前修改的验证。
