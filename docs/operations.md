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
| `fc:GetFunction`、`fc:CreateFunction`、`fc:UpdateFunction`、`fc:InvokeFunction` | `acs:fc:cn-shanghai:ACCOUNT_ID:functions/bigfoot-gate` 、`acs:fc:cn-hongkong:ACCOUNT_ID:functions/bigfoot-voyager`、`acs:fc:ap-southeast-1:ACCOUNT_ID:functions/bigfoot-voyager-singapore`、`acs:fc:ap-northeast-1:ACCOUNT_ID:functions/bigfoot-voyager-tokyo`、`acs:fc:us-west-1:ACCOUNT_ID:functions/bigfoot-voyager-siliconvalley` |
| `fc:GetTrigger`、`fc:ListTriggers`、`fc:CreateTrigger`、`fc:UpdateTrigger` | 上述函数 ARN 分别追加 `/triggers/*` |

区域变更时需要同步调整权限。程序不会自动创建 RAM 用户、授权或开放删除权限。

### 2. 初始化项目

```bash
./bigfoot init
python3 -m venv .bigfoot/venv
.bigfoot/venv/bin/pip install -r requirements-cloud.txt
```

编辑 `.bigfoot/config.json`，把 `deployment.access` 设为 `bigfoot-deploy`，与 CLI profile 同名。新配置为 version 3，初始节点为上海 Gate 和香港 Voyager，代码包部署无需填写 `image` 或预先获取域名。

`init` 只生成一次客户端密码、Gate/Voyager 口令和 Voyager 专属身份；重复运行会复用配置，损坏的配置会报错而非被覆盖。已有备份时恢复原文件，不必生成新身份。

### 3. 部署并验收

```bash
./bigfoot deploy --mihomo /path/to/mihomo
```

程序先编译本机版本校验配置，再构建 Linux amd64 静态程序，准备代码 ZIP。随后逐个部署已配置的 Voyager、保存 HTTPS 地址并检查健康，再部署上海 Gate，最后生成客户端配置。Voyager ZIP 只含可执行文件；Gate ZIP 另含不带凭证的规范化路由文件。

函数使用 `custom.debian11`，平台终止外层 HTTPS；HTTP 触发器使用 anonymous，由应用验证客户端密码和 Gate/Voyager 口令。程序监听平台内部端口 9000，公网入口使用 HTTPS/WSS。

不带 `--mihomo` 的 `deploy` 只做健康验收。健康通过不等于完整代理链路通过，命令输出会明确区分两者。

## 配置、凭证与有效期

用户的主配置入口是 `.bigfoot/config.json`，批量名单维护在该配置引用的本地文本文件中（新配置 `version: 3`，兼容读取 `version: 2`）；默认目录权限 0700、文件权限 0600，已排除 Git 跟踪。不要直接编辑 `generated/` 中的派生文件。

| 配置 | 用途 |
|---|---|
| `deployment.access` | 阿里云 CLI AK profile |
| `deployment.gate.region`、`deployment.voyagers.<节点>.region` | Gate/Voyager 部署地区 |
| `deployment.voyagers.<节点>.function_name` | 节点在该地区的独立函数名称 |
| `deployment.gate.domain`、`deployment.voyagers.<节点>.domain` | 代码包部署自动保存的 HTTPS 主机名 |
| `deployment.resources` | Gate 和全部 Voyager 统一的 CPU、内存和 FC 单实例并发规格 |
| `runtime` | 会话时间、空闲时间及应用单实例连接上限，不能超过 FC 并发 |
| `credentials.client_password` | 客户端密码 |
| `credentials.voyagers.<节点>.tunnel_token` | 每节点独立的 Gate/Voyager 口令 |
| `credentials.voyagers.<节点>.voyager_identity_ca_pem`、`credentials.voyagers.<节点>.voyager_identity_key_pem` | Voyager 专属身份公钥证书和私钥 |

应用配置和云账号凭证需要分别备份。

| 内容 | 有效期与更新方式 |
|---|---|
| 客户端密码、Gate/Voyager 口令 | 无自动到期时间，修改后需重新部署相关端 |
| Voyager 专属长期身份 | 约 50 年，并非无限期 |
| Voyager 内层 TLS 工作证书 | 7 天，剩余不足 48 小时时在新握手自动续签 |
| 外层 HTTPS 证书 | 由阿里云平台管理，独立于应用内层证书 |

Voyager 的专属签发私钥只分发给 Voyager；Gate 仅信任其公钥证书，始终校验有效期与 `voyager.internal` 主机名。自动续签不依赖共享磁盘、定时任务或重新部署。不要将其他服务的 CA 私钥填入 Voyager 的身份配置；身份泄露后应更换身份并重新部署两端。

单配置入口保存单个客户端密码和每个节点独立的 Gate/Voyager 口令。底层服务另支持多行密码/token 分阶段轮换，需通过自定义环境变量或文件配置，不能直接在单配置字段中粘贴多行值。

## 多出口与规则管理

### 迁移和新增节点

已有 version 2 配置先迁移；新运行 `init` 的 version 3 配置可跳过迁移：

```bash
./bigfoot migrate
./bigfoot add-voyager singapore --region ap-southeast-1
./bigfoot add-voyager tokyo --region ap-northeast-1
./bigfoot add-voyager siliconvalley --region us-west-1
```

地区 ID 见[阿里云 FC 支持地区](https://www.alibabacloud.com/help/en/functioncompute/regions)。迁移保留香港现有函数名称、域名、客户端密码和身份，并创建权限 0600 的 `.bigfoot/config.json.v2.bak`。新增节点各自生成身份和口令；重复添加相同地区的已有节点不会轮换凭证。迁移和新增命令复用部署锁。

新函数默认命名为 `bigfoot-voyager-<节点>`，请先为专用 RAM 用户补齐对应区域和函数的 FC 权限。脚本不会自动授权。默认出口和规则引用的所有节点都必须定义；只部署 Gate 前所有节点必须已有地址。

### 批量名单

主配置的路由片段示例：

```json
{
  "routing": {
    "default": "hongkong",
    "rules": [
      {"rule_set": "rules/tokyo.txt", "voyager": "tokyo"},
      {"rule_set": "rules/singapore.txt", "voyager": "singapore"}
    ]
  }
}
```

自行创建 `.bigfoot/rules/tokyo.txt` 等文件，使用 UTF-8，每行一条：

```text
# 精确域名
DOMAIN,api.example.com
# 域名本身及其所有子域名
DOMAIN-SUFFIX,example.jp
# IP 网段（示例仅说明格式，文档地址不是可用公网目标）
IP-CIDR,203.0.113.0/24
IP-CIDR6,2001:db8::/32
# 该目标端口的全部流量
DST-PORT,8443
```

同一名单内各行是“或”关系。按主配置中的名单顺序匹配，首组命中决定出口，未命中使用 `routing.default`；空名单或空规则列表均有效。精确域名忽略大小写和末尾根点，支持国际域名；后缀按域名标签边界匹配，`example.jp` 不匹配 `badexample.jp`。重复项可保留，跨名单冲突由顺序决定。

路径相对主配置文件所在目录解析，禁止目录逃逸和符号链接。只支持列出的五种指令，空行和整行 `#` 注释；不执行 PAC JavaScript，不支持通配符、DIRECT 或在线订阅。错误报告文件和行号，超限或语法错误会在云端修改前拒绝。每文件上限 8 MiB，原始文本合计 32 MiB，最多 200,000 条有效规则；规范化 JSON 也受 32 MiB 限制。这些是输入上限，未经容量验收。

Gate 根据客户端提交的目标域名/IP 和端口选择出口，不解析 HTTPS URL 路径。只收到 IP 时无法匹配域名；IP 网段规则只匹配直接提交的 IP，不在 Gate 额外解析域名。Voyager 独立执行公网地址安全检查。一条连接固定使用所选出口，出口不可用时连接失败，不自动切换地区。

### 更新和验收

```bash
./bigfoot check                                      # 离线语法、身份预检和构建
./bigfoot deploy                                     # 部署全部出口，再更新 Gate
./bigfoot deploy --only voyager --voyager tokyo       # 仅部署指定出口
./bigfoot deploy --only gate                         # 更新名单或默认出口
./bigfoot verify                                     # 所有节点健康检查
./bigfoot verify --mihomo /path/to/mihomo             # 只读验证当前规则选中的路径
./bigfoot deploy --only gate --mihomo /path/to/mihomo # 逐个出口完整验收
```

名单和默认出口随 Gate 部署生效，无热加载；客户端入口、密码不变，无需重新导入。Gate 更新仍可能中断已有连接。

多节点 `deploy --mihomo` 在持有部署锁时，临时逐次部署仅允许一个 Voyager 的 Gate 策略，通过同一个上海 Gate 验证各出口 HTTPS 和错误密码拒绝，最后恢复正式策略并再次验证。隔离配置中只有被测节点，不能把经其他节点成功的请求算作该节点通过。验收期间实际流量也可能受临时策略影响；正常异常和中断会尝试恢复，强制终止/主机断电不能保证恢复，应立即重跑 `deploy --only gate`。正式策略代码包的摘要保存在部署记录中，可用于恢复。仅部署 Voyager 时不能同时做多节点完整验收，需随后部署 Gate 验收。

`verify --mihomo` 不修改云端规则，只代表当前路径，不证明所有出口。未提供 Mihomo 时只记健康通过，不记代理链路通过。

## 更新与失败恢复

```bash
./bigfoot deploy --only voyager
./bigfoot deploy --only gate
./bigfoot --config /private/path/config.json deploy
```

只部署 Gate 时必须已有 Voyager 地址。已有同名函数必须带本脚本的管理描述才允许更新。

代码包部署将角色、阶段、构建摘要和结果保存到 `.bigfoot/generated/deployment.json`，不保存凭证或 SDK 错误正文。ZIP 按 SHA-256 保存在同目录，可用于复现；历史产物需自行清理。部署锁阻止同目录并发部署，进程退出后自动释放。云 SDK 的连接/上传和读取超时为 120 秒，覆盖跨洋代码包上传；网络超时后仍需重新查询云端实际状态再协调。

如果 Voyager 成功而 Gate 失败，已获取的 Voyager 地址会保留。修复原因后重跑同一命令：程序重新查询云端、更新已有函数并补齐触发器，不盲信上一次记录。请求超时但云端实际已完成，也按此方式处理。当前没有自动回滚，已断开的 TCP 会话需要客户端重新建立。

部署期间手动编辑配置可能使写回检查失败，停止并发编辑后重跑即可。常见检查方向：

| 现象 | 检查 |
|---|---|
| 找不到 Go 或云 SDK | Go 是否在 PATH；是否安装 `requirements-cloud.txt` |
| `AccessDenied` | profile、RAM 权限、函数地区是否一致 |
| HTTPS 健康失败 | 函数启动配置、端口 9000、平台入口和函数日志 |
| 健康通过但代理失败 | 客户端密码、Gate→Voyager 的 WSS/TLS 连接、Voyager 的公网出站；运行完整验收 |

## 默认资源与限制

新建 version 3 配置将下列规格写入主配置，统一用于 Gate 和全部 Voyager；`runtime.max_connections` 同步设为 200：

```json
{
  "deployment": {
    "resources": {
      "cpu": 0.05,
      "memory_mb": 128,
      "instance_concurrency": 200
    }
  },
  "runtime": {
    "max_connections": 200
  }
}
```

这是局部配置示例，合并到已有字段中。修改后运行 `./bigfoot check` 再运行 `./bigfoot deploy`，使所有函数生效。未配置 `deployment.resources` 的历史配置保留原来的 1 vCPU / 1024 MiB / FC 并发 128，避免仅更新工具就悄然调整线上规格。

校验遵循 [FC 资源约束](https://www.alibabacloud.com/help/tc/functioncompute/fc/developer-reference/api-fc-2023-03-30-struct-function)：CPU 为 0.05..16、步长 0.05；内存为 128..32768 MiB、步长 64；CPU 与内存 GiB 的比例须在 1:1 到 1:4 之间。单实例并发为 1..200，程序连接上限不得超过配置的 FC 并发。云端部署会读回 CPU、内存和并发逐项确认；这些配置上限不代表实测可承载 200 条活跃代理连接。


| 设置 | 默认值 |
|---|---|
| 新配置每端实例规格 | 0.05 vCPU / 128 MiB，按需实例 |
| FC 超时 / 程序会话上限 | 900 秒 / 14 分钟 |
| 双向应用空闲上限 | 2 分钟 |
| 新配置单实例并发 | FC 200，程序最多 200，含认证中连接 |
| 停机排空 | 5 秒，之后关闭剩余连接 |
| 健康入口 | `/healthz` 存活，`/readyz` 就绪；排空时就绪返回 503 |

- 只支持公网 TCP，不支持 UDP/QUIC、mux、旧会话恢复或按用户带宽配额。
- 单条 WebSocket 消息上限 1 MiB；TCP 流可跨多条消息。标准客户端 WS 关闭代表完整关闭，Gate/Voyager 内部保留单方向 FIN。
- Voyager 校验 DNS 结果并固定实际目标 IP，拒绝私网、特殊地址及混合 DNS 回答。
- 两端平台网关终止外层 TLS；Gate/Voyager 内层明文只在应用进程终止。云宿主仍可访问进程内存，目标网站 HTTPS 另有目标 TLS 保护。
- 冷启动、网关超时、实例更新都可能中断连接。单实例并发上限不是全账户费用上限，长连接会产生计算和流量费用。
- 程序输出 JSON 生命周期日志；日志收集、全局容量和费用需要在云平台管理，部署脚本不自动配置 SLS、预算或域名购买/绑定。
