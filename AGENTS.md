# Agent 工作约定

本文件面向在此项目中工作的编程 agent。产品使用说明放在 README，开发者说明放在 CONTRIBUTING。

## 阅读入口

- `README.md`：产品用途、使用方式与限制。
- `CONTRIBUTING.md`：代码结构、服务参数约定和验证命令。
- `docs/operations.md`：部署流程、配置、凭证与故障恢复。
- `docs/validation.md`：历史验收记录，不代表当前状态。

## 修改时遵守的约定

- 两端统一命名为 Gate / Voyager；配置键和命令值使用 `gate` / `voyager`。
- 用户维护的配置入口是 `.bigfoot/config.json`；`generated/` 是派生产物，不应作为源配置修改。
- 保留客户端密码与 Gate/Voyager 口令的分离。Voyager 身份私钥只分发给 Voyager，Gate 仅持有信任证书。
- 保留内层 TLS 的有效期和 `voyager.internal` 主机名校验，以及公网目标地址检查。
- 不把应用配置、客户端密码、身份私钥或云账号凭证写入文档、日志或提交内容。

## 验证与报告

- 按修改范围执行 `CONTRIBUTING.md` 中相关验证，并说明实际执行结果。
- 区分本地测试、云端健康检查与完整 Mihomo 链路验收；健康通过不等于代理链路通过。
- 不将历史测试结果表述为本次验证，也不假定当前电脑已有云账号或构建依赖。
- 未实测的手机 GUI、长连接、容量、冷启动和费用等事项应保留为未验证状态。

## 文档维护

- README 保持为使用者入口；详细操作放在 `docs/operations.md`。
- 人类开发者需要的命令和代码说明放在 `CONTRIBUTING.md`。
- Agent 工作指令放在本文件，带日期的验收结果和环境经历放在 `docs/validation.md`。
- 修改命令、配置或默认行为时，同步更新对应文档，避免在多个文件重复维护详细说明。
