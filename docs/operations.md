# 部署与运维

面向部署和维护 BigFoot 的使用者。返回 [README](../README.md)。

## 首次配置

部署工具使用 macOS/Linux 环境，需要 Go 1.25+、Python 3、OpenSSL；验收另需 curl 和 Mihomo。

### 1. 配置阿里云账号

安装[阿里云 CLI](https://help.aliyun.com/en/cli/install-update-alibaba-cloud-cli)，为专用 RAM 用户创建访问密钥，然后在本机交互式配置：

```bash
aliyun configure --mode AK --profile bigfoot-deploy
chmod 600 ~/.aliyun/config.json
```

代码包部署直接读取该 CLI profile，当前只支持 AK 模式。请使用专用 RAM 用户，不要将主账号密钥填入应用配置。

专用用户需要以下权限，资源中的 `ACCOUNT_ID` 替换为自己的账号 ID：

| 操作 | 资源范围 |
|---|---|
| `fc:ListFunctions` | `*` |
| `fc:GetFunction`、`fc:CreateFunction`、`fc:UpdateFunction`、`fc:InvokeFunction` | `acs:fc:cn-shanghai:ACCOUNT_ID:functions/bigfoot-gate` 和 `acs:fc:cn-hongkong:ACCOUNT_ID:functions/bigfoot-voyager` |
| `fc:GetTrigger`、`fc:ListTriggers`、`fc:CreateTrigger`、`fc:UpdateTrigger` | 上述两个函数 ARN 分别追加 `/triggers/*` |

区域变更时需要同步调整权限。程序不会自动创建 RAM 用户、授权或开放删除权限。

### 2. 初始化项目

```bash
./bigfoot init
python3 -m venv .bigfoot/venv
.bigfoot/venv/bin/pip install -r requirements-cloud.txt
```

编辑 `.bigfoot/config.json`，把 `deployment.access` 设为 `bigfoot-deploy`，与 CLI profile 同名。默认区域为上海和香港，代码包部署无需填写 `image` 或预先获取域名。

`init` 只生成一次客户端密码、Gate/Voyager 口令和 Voyager 专属身份；重复运行会复用配置，损坏的配置会报错而非被覆盖。已有备份时恢复原文件，不必生成新身份。

### 3. 部署并验收

```bash
./bigfoot deploy --mihomo /path/to/mihomo
```

程序先编译本机版本校验配置，再构建 Linux amd64 静态程序，只将可执行文件打入 ZIP。随后部署香港 Voyager、保存 HTTPS 地址并检查健康，再部署上海 Gate，最后生成客户端配置。

函数使用 `custom.debian11`，平台终止外层 HTTPS；HTTP 触发器使用 anonymous，由应用验证客户端密码和 Gate/Voyager 口令。程序监听平台内部端口 9000，公网入口使用 HTTPS/WSS。

不带 `--mihomo` 的 `deploy` 只做健康验收。健康通过不等于完整代理链路通过，命令输出会明确区分两者。

## 配置、凭证与有效期

用户只维护 `.bigfoot/config.json`（`version: 2`）；默认目录权限 0700、文件权限 0600，已排除 Git 跟踪。不要直接编辑 `generated/` 中的派生文件。

| 配置 | 用途 |
|---|---|
| `deployment.access` | 阿里云 CLI AK profile |
| `deployment.gate.region`、`deployment.voyager.region` | Gate/Voyager 部署地区 |
| `deployment.gate.domain`、`deployment.voyager.domain` | 代码包部署自动保存的 HTTPS 主机名 |
| `runtime` | 会话时间、空闲时间及单实例连接上限 |
| `credentials.client_password`、`credentials.tunnel_token` | 客户端密码、Gate/Voyager 独立口令 |
| `credentials.voyager_identity_ca_pem`、`credentials.voyager_identity_key_pem` | Voyager 专属身份公钥证书和私钥 |

应用配置和云账号凭证需要分别备份。

| 内容 | 有效期与更新方式 |
|---|---|
| 客户端密码、Gate/Voyager 口令 | 无自动到期时间，修改后需重新部署相关端 |
| Voyager 专属长期身份 | 约 50 年，并非无限期 |
| Voyager 内层 TLS 工作证书 | 7 天，剩余不足 48 小时时在新握手自动续签 |
| 外层 HTTPS 证书 | 由阿里云平台管理，独立于应用内层证书 |

Voyager 的专属签发私钥只分发给 Voyager；Gate 仅信任其公钥证书，始终校验有效期与 `voyager.internal` 主机名。自动续签不依赖共享磁盘、定时任务或重新部署。不要将其他服务的 CA 私钥填入 Voyager 的身份配置；身份泄露后应更换身份并重新部署两端。

单配置入口保存单个客户端密码和 Gate/Voyager 口令。底层服务另支持多行密码/token 分阶段轮换，需通过自定义环境变量或文件配置，不能直接在单配置字段中粘贴多行值。

## 更新与失败恢复

```bash
./bigfoot deploy --only voyager
./bigfoot deploy --only gate
./bigfoot --config /private/path/config.json deploy
```

只部署 Gate 时必须已有 Voyager 地址。已有同名函数必须带本脚本的管理描述才允许更新。

代码包部署将角色、阶段、构建摘要和结果保存到 `.bigfoot/generated/deployment.json`，不保存凭证或 SDK 错误正文。ZIP 按 SHA-256 保存在同目录，可用于复现；历史产物需自行清理。部署锁阻止同目录并发部署，进程退出后自动释放。

如果 Voyager 成功而 Gate 失败，已获取的 Voyager 地址会保留。修复原因后重跑同一命令：程序重新查询云端、更新已有函数并补齐触发器，不盲信上一次记录。请求超时但云端实际已完成，也按此方式处理。当前没有自动回滚，已断开的 TCP 会话需要客户端重新建立。

部署期间手动编辑配置可能使写回检查失败，停止并发编辑后重跑即可。常见检查方向：

| 现象 | 检查 |
|---|---|
| 找不到 Go 或云 SDK | Go 是否在 PATH；是否安装 `requirements-cloud.txt` |
| `AccessDenied` | profile、RAM 权限、函数地区是否一致 |
| HTTPS 健康失败 | 函数启动配置、端口 9000、平台入口和函数日志 |
| 健康通过但代理失败 | 客户端密码、Gate→Voyager 的 WSS/TLS 连接、Voyager 的公网出站；运行完整验收 |

## 默认资源与限制

| 设置 | 默认值 |
|---|---|
| 每端实例规格 | 1 vCPU / 1024 MiB，按需实例 |
| FC 超时 / 程序会话上限 | 900 秒 / 14 分钟 |
| 双向应用空闲上限 | 2 分钟 |
| 单实例并发 | FC 128，程序最多 128，含认证中连接 |
| 停机排空 | 5 秒，之后关闭剩余连接 |
| 健康入口 | `/healthz` 存活，`/readyz` 就绪；排空时就绪返回 503 |

- 只支持公网 TCP，不支持 UDP/QUIC、mux、旧会话恢复或按用户带宽配额。
- 单条 WebSocket 消息上限 1 MiB；TCP 流可跨多条消息。标准客户端 WS 关闭代表完整关闭，Gate/Voyager 内部保留单方向 FIN。
- Voyager 校验 DNS 结果并固定实际目标 IP，拒绝私网、特殊地址及混合 DNS 回答。
- 两端平台网关终止外层 TLS；Gate/Voyager 内层明文只在应用进程终止。云宿主仍可访问进程内存，目标网站 HTTPS 另有目标 TLS 保护。
- 冷启动、网关超时、实例更新都可能中断连接。单实例并发上限不是全账户费用上限，长连接会产生计算和流量费用。
- 程序输出 JSON 生命周期日志；日志收集、全局容量和费用需要在云平台管理，部署脚本不自动配置 SLS、预算或域名购买/绑定。
