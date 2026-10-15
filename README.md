# BigFoot

双端 TCP 代理：Gate 部署在阿里云上海，Voyager 可部署在香港、新加坡、东京、美国硅谷等地区，由 Gate 按规则选择出口。客户端使用 **Trojan over WSS** 连接 Gate，Gate/Voyager 之间使用独立口令和内层 TLS 1.3。

```text
手机 / Mihomo → HTTPS 网关 → 上海 Gate → 按规则选择 Voyager → 公网 TCP 目标
                                      ├─ 香港
                                      ├─ 新加坡
                                      ├─ 东京
                                      └─ 美国硅谷
Gate/Voyager 之间始终使用 WSS + 内层 TLS。
```

| 名称 | 职责 | 配置键 / 命令参数 | 云函数 |
|---|---|---|---|
| **Gate** | 上海入口，认证客户端 | `deployment.gate` / `--only gate` | `bigfoot-gate` |
| **Voyager** | 多地区出口，连接目标 | `deployment.voyagers` / `--only voyager` | 每节点独立函数 |

## 快速使用

首次部署请先完成[环境与云账号配置](docs/operations.md#首次配置)。配置完成后，在项目根目录运行：

```bash
./bigfoot deploy                          # 更新 Voyager、Gate，检查健康并导出客户端配置
./bigfoot verify                          # 检查 Gate 和所有 Voyager 健康
./bigfoot verify --mihomo /path/to/mihomo  # 只读检查当前规则选中的 HTTPS 链路及错误密码拒绝
./bigfoot render                          # 从保存的 Gate 地址离线导出客户端配置
```

手机使用 `.bigfoot/generated/client.yaml`，具体步骤见下方“手机与桌面客户端”。多节点完整验收使用 `./bigfoot deploy --only gate --mihomo /path/to/mihomo`，会临时逐个选择出口并恢复正式规则。需要本机已有 Mihomo 可执行文件；工具不会自动安装客户端。

## Gate 集中分流

客户端始终连接同一个 Gate，规则在服务端管理，修改名单后无需重新导入客户端配置。

- 在 `.bigfoot/config.json` 的 `routing.default` 中选择默认 Voyager。
- `routing.rules` 按顺序把文本名单分配给不同 Voyager；第一组命中生效，否则走默认出口。
- 文本名单支持精确域名、域名后缀、IPv4/IPv6 网段及目标端口，可按用途拆成多个文件。
- 名单更新后运行 `./bigfoot deploy --only gate`；指定出口故障时连接失败，不自动更换地区。

已有单出口配置可显式迁移，香港身份及客户端密码保持不变。具体步骤和名单格式见[多出口与规则管理](docs/operations.md#多出口与规则管理)。

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
- 新配置默认每端 0.05 vCPU / 128 MiB，单实例并发 200；规格统一在主配置维护，实际容量未验收。
- 默认单次会话最长 14 分钟，双向空闲上限 2 分钟；冷启动和实例更新可能中断连接。
- 长连接会产生计算和流量费用；单实例并发上限不等于账户费用上限。

完整资源默认值、安全边界和故障排查见[部署与运维](docs/operations.md)。

## 文档导航

- [部署与运维](docs/operations.md)：首次配置、凭证、更新和故障恢复。
- [开发说明](CONTRIBUTING.md)：代码结构和本地验证命令。
- [验收记录](docs/validation.md)：历史验证范围及未验收事项。
- [Agent 工作约定](AGENTS.md)：供自动化编程助手读取的项目规则。
