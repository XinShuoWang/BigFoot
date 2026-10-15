# 历史验收记录

以下记录仅代表对应日期实际执行的验证，不能自动视作后续代码或环境的验收结果。返回 [README](../README.md)。

## 2026-10-15

多 Voyager 与 Gate 集中规则分流实现后的本地验证：

- `go test -race -count=1 -timeout 90s ./...`：全部包通过，包括三个独立身份 Voyager 经同一 Trojan/WSS Gate 的实际节点标记、默认出口、认证前不拨号、节点口令/证书隔离及禁止故障切换测试。
- `go vet ./...`、`go build -o ./tunnel ./cmd/tunnel`：通过。
- `python3 -m unittest discover -s tests`：25 项通过，包括旧配置兼容、显式迁移和并发编辑拒绝、名单打包与路径校验、逐节点部署，以及验收/日志写入失败后的 Gate 策略恢复。
- `./bigfoot check`：实际三节点私有配置及名单通过本机服务预检、Linux amd64 构建；`git diff --check` 通过。
- 独立代码审查发现的 PEM 文本去重和证书隔离测试干扰问题已修正并复查。

本地私有配置已备份并迁移，香港身份与客户端密码保留；新加坡、东京各生成独立身份和口令，名单文件初始为空，默认出口仍为香港。以上配置和身份不在本记录中保存。

首次云端只读检查时，新加坡、东京返回 `AccessDenied`。用户随后在云端补齐权限；重新查询确认上海/香港现有函数可访问、新加坡/东京函数可创建。本次 agent 未修改 RAM 授权。

授权后执行 `./bigfoot deploy`，部署香港、新加坡、东京三个 Voyager，再更新上海 Gate；四个函数均通过健康与就绪检查。随后独立运行 `./bigfoot verify`，四个函数的 `/healthz` 和 `/readyz` 再次通过。部署记录为 `healthy`，`link_verified` 为 false。客户端入口和密码与迁移前备份一致，默认出口仍为香港，东京/新加坡名单当前为空。

首次发布在香港函数的读回阶段超时：实际请求字段均匹配，但 FC 自动补充了未请求的 `customRuntimeConfig.healthCheckConfig`。修正适配器，仅允许该未显式设置字段的默认值，代码摘要、启动命令、端口、凭证及其他字段仍严格比较；新增回归测试先复现失败，修正后全量 25 项 Python 测试通过，针对读回的 4 项测试再次通过。重跑部署成功协调已更新的香港资源，并继续完成其余节点。

本次未提供可用 Mihomo 路径，因此未执行本地 Mihomo smoke 或云端完整代理链路验收。四个函数健康不代表三个出口的完整代理链路已通过。手机 GUI、长连接、容量、冷启动和费用仍未验证。

### 同日追加：硅谷节点与全节点最低规格

用户要求上海 Gate 与全部 Voyager 统一使用 0.05 vCPU、128 MiB、FC 单实例并发 200，并新增美国硅谷出口。主配置增加 `deployment.resources`，应用 `runtime.max_connections` 同步为 200；历史未配置资源字段的配置继续保留原规格。新增硅谷节点使用 `us-west-1`，具有独立身份与口令，默认出口仍为香港；硅谷名单当前为空。

- `python3 -m unittest discover -s tests`：30 项通过。新增 5 项资源测试覆盖配置边界、FC 创建/更新、应用与平台并发一致性、历史配置兼容，以及临时路由验收和正式策略恢复时保留资源规格；独立审查未发现实质问题。
- `./bigfoot check`：四个 Voyager 与 Gate 的本机预检和 Linux amd64 构建通过。
- 初次全量发布完成香港、新加坡、东京降配后，硅谷代码包上传触发 SDK 传输超时（异常链最终为 TimeoutError）。GetFunction 查询正常，没有收到资源规格被拒绝的响应。将云 SDK 连接/上传与读取超时从 10/15 秒延长至 120/120 秒后，单独重跑硅谷部署成功，再部署 Gate 成功。
- 部署后独立通过 FC GetFunction 读回五个函数：上海 Gate、香港、新加坡、东京、硅谷均为 CPU 0.05、内存 128 MiB、instance_concurrency 200；各函数的应用连接上限环境设置均为 200。
- 随后 `./bigfoot verify` 检查五个函数的 `/healthz` 与 `/readyz`，全部通过；`git diff --check` 通过。

本轮为部署规格与健康验证，未实测 200 条活跃代理连接的容量、低规格下的延迟与冷启动，也未执行完整 Mihomo 链路验收；不据此推算实际费用。


### 同日追加：新加坡规则发布与提交前复验

本地私有新加坡名单已更新并执行 `./bigfoot deploy --only gate`，Gate 部署及健康检查通过；默认出口保留香港。私有配置和名单均不纳入 Git。

- `./bigfoot check`：全部节点本机配置预检和 Linux amd64 构建通过。
- 使用实际 Go 路由匹配器校验 42 个新加坡域名及子域名目标、后缀边界，以及 5 个默认出口对照目标，全部通过；核对本次 Gate 部署包中的规则与源名单一致。
- 临时 Trojan over WSS 探针经线上 Gate 请求 OpenAI 和 Claude 的 HTTPS trace 端点，均返回 HTTP 200，出口国家为 SG；TikTok 和普通 HTTPS 对照目标返回 HTTP 200，未报告出口国家。探针保留目标 TLS 证书验证，没有执行账号登录或业务功能测试。
- 提交前重新执行 `go test -race -count=1 -timeout 90s ./...`、`go vet ./...` 和 Python 单元测试，全部通过，Python 共 30 项。

本次没有可用 Mihomo 可执行文件，未执行 Mihomo 完整链路验收；上述临时探针不能替代该验收，也不能证明账号地区限制已经解除。

## 2026-09-10

Go 回归、race、vet、Linux amd64 构建及 16 项 Python 测试通过；协议 fuzz 和当时的已知漏洞检查通过。云端两端健康、真实 Mihomo HTTPS 和错误密码拒绝通过。部署工具通过模拟 FC 的失败恢复测试，并已实际部署上海 Gate 与香港 Voyager，完成完整链路验收。

尚未实测各手机 GUI、长连接极限、容量、冷启动统计及实际费用。

- 此前发布构建使用 Go 1.27.1。
- 实际验证客户端为 Mihomo v1.19.30（macOS arm64）。
- 当次浏览器 OAuth 授权能够访问 RAM，但访问 FC 返回 `missing parameter SecurityToken`，实际部署改用专用 RAM 用户的 AK profile。
