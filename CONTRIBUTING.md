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
internal/routing/    不进行 DNS 查询的不可变目标规则索引
internal/tunnel/     Gate/Voyager 内层 TLS、认证及目标策略
internal/service/    会话限制、健康检查与排空
scripts/deployment/  配置、构建、凭证、FC 适配、编排和验收
scripts/             CLI、兼容工具与客户端测试
tests/              Python 工具测试
```

## 服务配置约定

命令和配置统一使用 `gate` / `voyager`；服务模式为 `-mode gate` / `-mode voyager`，单出口 Gate 上游地址参数为 `-voyager-url`（环境变量 `BIGFOOT_VOYAGER_URL`）；多出口使用 `-routing-config`（`BIGFOOT_ROUTING_CONFIG`）指向不含凭证的 JSON 文件，两种入口互斥，内层证书主机名为 `voyager.internal`。

历史验收结果见[验收记录](docs/validation.md)，不能替代当前修改的验证。

多节点配置用 `./bigfoot check` 执行名单校验、所有节点的本机服务预检和 Linux 构建，不访问云端。规则文件先由 Python 检查路径、大小和语法，再由 Go 执行最终 IDNA 规范化及服务校验。部署只使用一次读取的规则快照，发现节点域名后仅替换路由文件中的上游 URL。

`cmd/tunnel/routing_test.go` 启动三个独立身份的真实 Voyager，通过同一 Trojan/WSS Gate 检查节点标记、默认出口、认证前不拨号、口令/证书隔离和禁止故障切换。`tests/test_multi_voyager.py` 覆盖迁移、规则打包、逐节点部署与验收失败后的正式策略恢复。云端 `verify --mihomo` 仅检查当前路径，不能代表所有出口；`deploy --only gate --mihomo` 会逐个部署单上游临时策略实测，最后恢复正式策略并再次验收。

资源配置由 `deployment_resources()` 统一校验并传入 FC 创建、更新以及临时路由验收/正式策略恢复，避免验收过程还原成旧规格。`tests/test_resources.py` 通过实际适配器的受控 FC 请求检查 CPU/内存/平台并发和应用连接上限一致性，以及历史配置兼容。
