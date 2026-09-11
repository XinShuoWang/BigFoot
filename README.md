# BigFoot

双端 TCP 代理：Gate 部署在阿里云上海，Voyager 部署在香港。客户端使用 **Trojan over WSS** 连接 Gate，Gate/Voyager 之间使用独立口令和内层 TLS 1.3。

```text
手机 / Mihomo → HTTPS 网关 → Gate → WSS + 内层 TLS → Voyager → 公网 TCP 目标
```

| 名称 | 职责 | 配置键 / 命令参数 | 云函数 |
|---|---|---|---|
| **Gate** | 上海入口，认证客户端 | `deployment.gate` / `--only gate` | `bigfoot-gate` |
| **Voyager** | 香港出口，连接目标 | `deployment.voyager` / `--only voyager` | `bigfoot-voyager` |

## 快速使用

首次部署请先完成[环境与云账号配置](docs/operations.md#首次配置)。配置完成后，在项目根目录运行：

```bash
./bigfoot deploy                          # 更新 Voyager、Gate，检查健康并导出客户端配置
./bigfoot verify                          # 仅检查线上两端健康
./bigfoot verify --mihomo /path/to/mihomo  # 检查完整 HTTPS 链路及错误密码拒绝
./bigfoot render                          # 从保存的 Gate 地址离线导出客户端配置
```

手机使用 `.bigfoot/generated/client.yaml`，具体步骤见下方“手机与桌面客户端”。完整链路验收需要本机已有 Mihomo 可执行文件；工具不会自动安装客户端。

## 手机与桌面客户端

1. 将 `.bigfoot/generated/client.yaml` 私下传到设备。
2. 在支持 Mihomo 配置的客户端导入文件，在手机端开启其 VPN 功能；桌面端按需开启系统代理。
3. 如需保留自己的分流规则，只合并生成文件中的 `proxies` 节点。完整生成配置会将进入代理的 TCP 请求全部转发到 BigFoot。

文件含客户端密码，但不含 Gate/Voyager 口令或 Voyager 私钥。不要向手机传输 `.bigfoot/config.json` 或云账号凭证。Android/iOS 图形界面尚未逐个平台测试，已有测试范围见[验收记录](docs/validation.md)。

其他支持 Trojan + WebSocket 的客户端可手动填写：

| 字段 | 值 |
|---|---|
| 类型 | Trojan |
| 服务器、SNI | 主配置中的 `deployment.gate.domain` |
| 端口 | `443` |
| 密码 | 生成客户端配置中的 `password` |
| TLS | 开启，保留证书校验 |
| 传输、路径 | WebSocket，`/proxy` |
| ALPN | `http/1.1` |
| UDP、mux、early data | 关闭 |

只支持原生 Trojan/TCP、没有 WebSocket 选项的客户端不能连接。生产环境无需导入本地测试 CA。

桌面也可直接运行 Mihomo：

```bash
mihomo -t -f .bigfoot/generated/client.yaml
mihomo -f .bigfoot/generated/client.yaml
```

另开一个终端测试：

```bash
curl --noproxy '' --proxy http://127.0.0.1:7890 --fail https://example.com/
```

## 使用限制

- 仅支持公网 TCP，不支持 UDP/QUIC、mux 或旧会话恢复。
- 默认单次会话最长 14 分钟，双向空闲上限 2 分钟；冷启动和实例更新可能中断连接。
- 长连接会产生计算和流量费用；单实例并发上限不等于账户费用上限。

完整资源默认值、安全边界和故障排查见[部署与运维](docs/operations.md)。

## 文档导航

- [部署与运维](docs/operations.md)：首次配置、凭证、更新和故障恢复。
- [开发说明](CONTRIBUTING.md)：代码结构和本地验证命令。
- [验收记录](docs/validation.md)：历史验证范围及未验收事项。
- [Agent 工作约定](AGENTS.md)：供自动化编程助手读取的项目规则。
