# 项目修复与验证报告

日期：2026-10-18（Asia/Shanghai）  
输入：用户原始附件 `2112-main (1).zip`  
输出版本：`v3.1.1-repair20261018`

## 范围与结论

已解压原始附件，对全部文件建立清单，检查全部 Go 源文件的解析/类型/静态问题、内嵌 JavaScript 与 Worker 的语法、Shell 脚本语法和项目配置；对鉴权、远程输入、代理协议、测活、路由、状态恢复、安装更新等关键路径做了人工审查。对已确认问题实施以下修复，并运行原有测试和新增回归测试。

这不是绝对零 bug 证明，也不是逐行形式化验证。当前执行环境为 Windows，未提供 Linux 内核命名空间、OpenVPN/Xray 实机与面板账户。因此 Linux 构建成功不等于线上联调成功。原项目 `AUDIT.md` 中的历史通过记录不作为本次证据。

## 修改清单

| 模块 | 已实施修改 | 作用 |
|---|---|---|
| `ovpn_security.go`, `tunnel.go` | 对 OpenVPN 配置实行允许选项与内联证书块校验；拒绝脚本、插件、嵌套配置和任意文件路径；限制配置大小；强制 script-security=1 | 防止远程节点配置被当作高权限执行指令 |
| `livescan.go`, `ovpn_security.go` | OpenVPN 测活改为受超时约束的真实客户端握手，使用 dev=null/noexec，不修改母机接口或路由 | 不再将 TCP 可连接或任意 UDP 回包当作 VPN 握手成功；仍需正式隧道出口验证 |
| `livescan.go`, `tunnel.go` | 统一代理拨号与完整 HTTP 响应解析；必须取得合法出口 IP；移除失败后回退到节点 IP 的成功兜底 | 避免错误页、分片响应或 CONNECT 失败造成假成功 |
| `health.go` | 上游代理健康检查比较实际出口与已记录出口 IP | 不再只凭 HTTP 200 判定健康 |
| `netns.go`, `tunnel.go`, `health.go` | 命名空间拨号绑定 tun0，命名空间内 curl 也指定 tun0 | 降低隧道消失后回落母机 NAT 的风险，需 Linux 实机断线验证 |
| `native_xray.go` | 已绑定但离线的入站增加 block 路由 | 避免离线出口的入站落入默认 direct |
| `manager.go` | 身份键包括协议、地址与端口；配置节点统一为 ovpn；刷新、导入、去重采用同一键 | 保留同 IP 的不同端口/协议候选 |
| `manager.go` | 单源刷新保留其他源；导入缓存前复制节点列表 | 防止刷新清空其他来源；减少共享切片并发访问 |
| `manager.go` | 尊重小于 100 的 maxSlots 配置，默认 40，上限 254 | 修正原先小配置被强制改成 150，避免槽位产生非法 IPv4 子网 |
| `livescan.go` | 缓存命中分支与并发结果统一加锁；先获取并发额度再启动 goroutine；开始扫描清理旧结果；保留上游连接 IP | 修复结果切片竞态、无界等待 goroutine、旧结果残留与误改上游地址 |
| `socks5.go` | 校验协议版本/保留字段/空域名；只选择客户端提供的认证方式 | 拒绝畸形协议输入 |
| `udp.go` | 校验 UDP 来源 IP 与已鉴权 TCP 对端一致；单 association 限制 256 个目标会话；修复域名回包长度字段；拒绝空域名 | 防止任意外部来源借用中继，修复域名 UDP 兼容问题并限制资源占用 |
| `tunnel.go` | HTTP/HTTPS CONNECT 与 SOCKS5 上游拒绝 UDP 请求；严格解析 CONNECT 状态码、SOCKS 应答和不完整头部 | 防止 UDP 被错误转成 TCP、500 文本含 200 被误放行；https 保持常见代理列表的 HTTP CONNECT 语义 |
| `auth.go` | 拒绝空口令/订阅密钥文件；增加跨站 Origin/Fetch Metadata 校验；限制请求体；返回 no-store/no-referrer/nosniff/frame-deny；TLS 下设置 Secure Cookie；串行更新密钥文件与内存 | 强化登录与管理接口，减少凭据缓存/泄露与并发写入不一致 |
| `webserver.go`, `settings.go` | 设置读取校验；请求头/读取/空闲超时；串行应用监听设置；持久化失败不切监听，绑定失败回滚磁盘 | 修复设置保存失败后监听已改变等状态不一致 |
| `inbound_spec.go` | 校验手填端口 1–65535 | 提前拒绝非法入站配置 |
| `basepath.go` | 保留用户主动设置的空访问前缀 | 防止重启后意外重新生成随机前缀 |
| `state.go` | 状态恢复返回实际受限的恢复数量 | 修复界面/日志数量误报 |
| `tunnel.go`, `native_xray.go` | 用完成通道观察子进程退出 | 消除读取 exec.Cmd.ProcessState 与 Wait 并发写入的问题 |
| `job.go` | 清理任务通过加锁快照读取状态 | 修复任务状态的一个并发读取点 |
| `gatevpn_sources.go`, `vpngate.go`, `xui.go` | 网络 ReadAll 增加大小上限；出口响应限制 64 KiB | 降低超大响应造成内存耗尽的风险；不构成完整流量/来源配额系统 |
| `update.go` | 更新前先完整下载脚本并处理 HTTP 失败；执行超时限制与失败日志；修复短提交号切片 panic | 避免下载失败仍直接报告已提交执行；保留上游更新模式 |
| `install.sh` | 优先使用脚本同目录源码；先编译再停旧服务；Go 回退下载升级并校验 SHA-256 | 确保本 ZIP 修复能被安装，减少构建失败导致停机 |
| `web.go`, `web_check.js` | 去掉重复打开弹窗；搜索防抖；测活请求防重叠；关闭窗口停止轮询；请求超时覆盖响应体读取；缺失延迟显示未知；修正并发宣传文字 | 减少重复请求与错误反馈；两份脚本已同步 |
| `netns_unsupported.go` | 增加非 Linux 的明确失败实现 | 支持平台无关单元测试，绝不回退到母机网络 |
| `audit_regression_test.go`, `frontend_test.cjs`, `check.sh`, `.github/workflows/check.yml` | 新增 13 个 Go 回归测试、前端/Worker 冒烟检查与 Linux CI | 提供可重复验证入口；CI 配置已提供，未在远程仓库触发 |

部分原有测试更新了预期：离线绑定应产生阻断规则；测试初始化也必须使用合法管理端口。未删除原有测试。

## 实际测试结果

| 检查 | 结果与边界 |
|---|---|
| Go 版本 | 官方 Go 1.27.2，工具包下载后核验官方 SHA-256 |
| `go test -v -count=1 -timeout=120s -coverprofile=verification/coverage.out ./...` | 94 个顶层测试全部通过；Windows 上执行，覆盖率 15.5% |
| `go vet ./...` | Windows/amd64、Linux/amd64、Linux/arm64 均通过 |
| Linux amd64/arm64 `go build -trimpath` | 均成功；附重建二进制，未在 Linux 上执行 |
| `node --check` | 内嵌网页对应脚本、Worker 均通过 |
| `node frontend_test.cjs` | 页面脚本同步、路径拼接、HTML 转义、API 错误处理、Worker 鉴权/缓存/请求头隔离通过；未代替完整浏览器测试 |
| `bash -n` | install.sh、f.sh、check.sh 通过 |
| govulncheck | v1.9.0 在本次 Windows 源码调用图扫描中返回 No vulnerabilities found；主模块无第三方 Go 模块依赖 |
| `go test -race ./...` | 未运行成功：缺少 cgo/C 编译工具链；原始错误保存在 verification/race-unavailable.txt |
| UI 浏览器、移动端与 Linux 外部组件 | 未完成实机验收，不标记通过 |

`verification/` 保存测试输出、覆盖文件和构建/静态检查记录。空的 vet 日志表示命令未输出诊断；构建记录单独标记成功。`FILE_MANIFEST.json` 给出最终文件 SHA-256 清单。

## 仍需解决或验证的风险

1. **并发风险仍未全面闭环**：Tunnel 的可变导出字段仍由启动、重连、健康检查和 HTTP 读路径共享，锁使用需要系统性梳理。已修复几个明确的竞态点，但未获得 race 动态验证，不能称为无竞态版本。建议优先在 Linux 上执行检查脚本并进行并发启动/停止/重连压力测试。
2. **Linux 实机功能尚未验证**：需要验证 netns/tun0、SO_BINDTODEVICE、路由、DNS、iptables、OpenVPN 2.5/2.6、Xray 与 3x-ui/xray-cf-lite。握手探测的 dev=null 模式以及配置允许列表可能拒绝某些原本可用但带额外选项的配置；这是有意保守处理，需要按真实样本扩展允许列表，不能直接恢复任意指令加载。
3. **断流保护范围**：本次对 fanout 隧道拨号和自建 Xray 路由增加保护；接管第三方面板的路由行为未做线上验证，不能保证所有后端均不存在回落直连。已经建立的连接、内核状态变化与 DNS 解析也需专门验收。现有域名预解析仍在母机侧，未实现完整的 VPN 内 DNS 隐私方案。
4. **外部数据可信度**：出口探测仍使用部分 HTTP 接口；恶意上游可伪造 HTTP 数据。国家、学校/政府标签、住宅类型与纯净度含启发式/第三方推测，不能当作权威真实性或安全保证。公开节点可随时失效。
5. **安装更新信任链**：安装仍会获取 Xray/依赖，F11 仍信任上游 main 代码和安装器；未实现发布签名、固定提交端到端验证或后台更新结果查询。一次“提交更新”不等于安装已成功，失败需查日志。本地安装可能下载外部组件，不是完全离线安装。
6. **兼容/运维**：未完成浏览器视觉与 iOS/Android 验证；管理服务仍通常为 HTTP，需要按部署环境保护传输。旧接口存在 GET 修改操作，本次以 Origin/Fetch Metadata 加固，未完成全量 POST+CSRF token 迁移；旧浏览器无相关请求头时仍需进一步处理。限流表、缓存与全部命令执行的资源约束也不是全覆盖。
7. **覆盖边界**：15.5% 语句覆盖率说明大量网络、系统管理和面板路径未被测试执行；govulncheck 无告警不代表没有业务漏洞。CI 的 Linux race 检查是后续验证入口，不能记为本次已执行。

## 使用修复版

先在测试服务器备份 `/var/lib/fanout` 及第三方面板配置。解压后进入 `2112-main/fanout-main`：

```bash
# 复查（需要 Go、Node、bash；race 还需要 C 编译器）
FANOUT_RACE=1 bash check.sh

# 安装本地修复源码；安装器仍会安装缺少的系统组件
sudo bash install.sh
```

`fanout` 为本次重建的 Linux amd64 二进制；`dist/fanout-linux-arm64` 为 arm64 版本。手动使用时先 `chmod +x`，并确认架构和依赖。ZIP 不包含用户运行口令或线上配置。更新上游仓库前，不应使用 README 历史远程命令或 F11 来安装本次修复。

官方参考：[Go 下载与校验信息](https://go.dev/dl/)、[OpenVPN 2.6 配置与脚本选项手册](https://openvpn.net/community-docs/community-articles/openvpn-2-6-manual.html)。
