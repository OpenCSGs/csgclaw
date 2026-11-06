# 通过社区发布与使用本地 Agent

登录目标 OpenCSG 站点后，可在 Agent 详情选择「发布到社区」。支持宿主机 Codex/DSH，
凭据优先来自 `CurrentSiteCredentials()`，缺失或过期时回退到现有
`CSGHUB_API_BASE_URL` / `CSGHUB_USER_TOKEN`（兼容 `CSGHUB_ACCESS_TOKEN`）。
登录存储损坏时返回错误，不静默切换账号；Token 不复制到发布状态或浏览器。

## 外部 Agent

1. 向当前社区 `/api/v1/relay/v1/agents` 注册 relay。
2. 保存返回的 `agent.id`，作为 `relay.agent_id`。
3. 调用 `POST /api/v1/agent/instances`，发送 `type=csgclaw`、`kind=runtime`、
   `source=external`、`content_id=agent.id`、`relay.agent_id=agent.id` 及展示信息。
4. 注册按 relay ID 幂等，失败重试、重启恢复不会重复创建目录记录。

社区仍会执行手机号验证和 Agent 准入检查。原有「先创建 instance 后注册 relay」流程
已替换，不再使用 `metadata.subscription_id` 或写入服务端管理的 metadata 镜像字段。

## 模板 sandbox 自动发布

镜像启动时设置 `TEMPLATE_ID` 和 `CSGCLAW_AGENT_AUTO_PUBLISH=true`。
脚本调用 `participant create --bind create --auto-publish=true`，`csgclaw` 与
`csgclaw-cli` 共用该参数。默认关闭，只作用于启动脚本的目标 Agent。
没有模板时不通过此镜像创建 Agent 或注册 relay，不新增这类启动场景。

社区已创建对应的 platform instance。`CSGCLAW_NAME` 是它的 content ID，
不是本地 Agent ID、数字 instance ID 或 relay ID。服务端在自动发布排队时保存该目标，
注册 relay 后调用：

```http
PUT /api/v1/agent/instances/by-content-id/csgclaw/{CSGCLAW_NAME}
Content-Type: application/json

{"relay":{"agent_id":"agt_0123456789abcdef01234567"}}
```

只修改 relay 字段，不创建 external 记录，也不覆盖名称、public 或 metadata。
如果平台提供 `OPENCSG_SANDBOX_TYPE=csgclaw-workspace`，使用该类型的按 content ID 更新接口。
Relay 最初以 private 注册。平台回写接受 HTTP 200 的空或 null 结果，返回实例时会校验实例身份。
发布端根据 StarHub 实例列表的 `public` 字段设置 Relay 可见性，并在运行期间检查变更。
目标保存在发布状态中，重启后继续使用。显式下架后再手动发布同一 Agent 仍回写原 instance。

## 转发与认证

统一使用 `{登录站点}/api/v1/relay`，后接 relay 的 `/v1/*` 路径。
csgclaw 的 HTTP、SSE 和 `csgrelay.v1` WebSocket 握手携带 Bearer Token、
`X-CSG-Client-ID`、请求 ID 和幂等 Key。社区验证 Token、覆盖可信用户 UUID，
移除 Authorization/Cookie 后转发。客户端不自行发送可信用户身份头。

同一 csgclaw 服务的所有 Agent 共用持久化 client ID；按本地 API 地址和端口隔离，
重启保持不变。`~/.csgclaw/relay/<地址 sha256>/` 保存权限为 0600 的身份和发布状态。
托管凭据没有本机登录 UUID 时，仍以注入凭据的指纹绑定本地状态，更换凭据前需先下架旧发布。

## 广场与下架

广场以当前 StarHub 登录身份查询 `registered=true&kind=runtime`。公开页按列表的 `public` 字段筛选，
「我的」也显示本人已注册的私有 Agent。查询遍历分页，同时包含 platform
与 external 的 `csgclaw` / `csgclaw-workspace` runtime，并以 `relay.agent_id` 召唤。
缺失、非法注册及 logical 记录不进入广场。external 的 `is_running=true` 不能表示在线，
客户端仍查询 relay 状态；仅确认离线或注册不存在（404）时显示离线。
认证、限流、网络及服务端错误返回目录查询失败，不伪装为离线。
广场来源展示社区 Agent 名称，发布端根据 instance.source 映射：platform 为 Agentichub，
external 为 CSGClaw，不将 relay publisher_id 作为产品名称。

浏览器仅保存“已添加”关系及不含 Token 的展示元数据。我的智能体和召唤列表必须与当前
社区目录的 relay ID、URL、登录连接引用同时匹配，不使用缓存复活目录已不存在的 Agent。
仍在目录中的离线 Agent 保留展示但不能启动任务；目录请求失败时暂时隐藏云端列表，
保留添加关系以便同一注册恢复后重新显示。添加、移除同步到已挂载组件和其他标签页。
“移除”仅取消本地添加关系，不下架或删除社区 instance。
升级为 v3 缓存时丢弃包含旧 mock 注册的 v2 缓存，之前添加的 Agent 需重新添加。

外部 Agent 下架会先停止执行端，再删除 external 目录记录和 relay 注册。
平台 Agent 下架只停止 relay 执行连接和注册，保留原 instance 与 sandbox，不调用其删除、
暂停或重启接口。当前社区接口不能清空 relay 字段，因此平台目录可能保留离线记录。
失败的下架会持久化清理状态，重启不会重新注册；再次 DELETE 可继续完成清理。

## 本地接口与恢复

- `GET/POST/DELETE /api/v1/agents/{id}/relay`：状态、发布、下架；普通 POST 使用 `{}`。
- POST `{"auto_publish":true}` 返回 202，先持久化发布意图，等待本地 Agent 就绪后重试。
- `publishing` 表示排队中；`published` 表示注册与社区同步完成。
- `GET /api/v1/relay/agents`：返回社区广场记录，不返回 Token。

启动脚本在新建、marker 恢复和已有 Agent 恢复时均检查发布状态，回写完成后才开放 8888 readiness。
持久挂载 `/home/picoclaw/.csgclaw` 保留身份、目标和恢复进度。关闭自动发布开关不会自动下架。
`csgclaw relay` CLI 保留为底层协议诊断工具，不创建社区 instance。

契约见 [StarHub Agent Relay API](https://git-devops.opencsg.com/product/starhub/starhub-server/-/blob/feat-agent-register/.specs/agent-register/api_ee.md)。
定向 HTTP/WebSocket 测试覆盖新接口字段、外部注册顺序、平台回写、可见性、失败恢复及下架。
真实社区许可、准入、代理、NATS 链路仍需部署后联调。

## 异常恢复与删除保护

首次 relay 注册失败不会锁定空实例的登录身份；旧版本遗留的空绑定在启动时解除。
确认注册后，登录绑定保持到清理完成。external instance 创建已提交但响应丢失时，
下架按 relay/content ID 查回记录并删除；查询或删除失败保留清理标记，重启不再注册，
允许继续重试。平台 instance 仍禁止删除。

发布状态提供 publishing、pending、removing、can_unpublish。弹窗支持取消排队、
清理未完成注册、重试待同步操作；尚无远程进度的队列可在未登录、断网时取消。
本地 Agent 删除与发布操作串行，存在发布、排队或清理状态时返回
relay_unpublish_required，须先下架或取消再删除。

广场最多并发探测 8 个注册，超时或取消返回目录查询错误，不将未确认状态报告为离线。
创建云端任务只探测选中的社区注册。

待完成的社区同步独立于 WebSocket 重连，以 1～15 秒退避重试，连接保持正常时也可恢复。
下架会取消并等待同步重试线程结束后再删除注册。平台绑定在下架时保留，便于原 Agent
再次发布；本地 Agent 删除成功后释放绑定，允许替换 Agent 关联同一 sandbox instance。

本地删除成功后，在发布操作锁内持久化已删除 ID。手动发布和自动排队在获取同一锁后
检查该标记，拒绝删除前已通过 API 存在性检查的旧请求；重启后仍生效。
删除失败不标记 ID，同名但不同 ID 的新 Agent 可正常发布并使用已释放的平台绑定。

恢复的发布在缺少或过期凭据时按 1～15 秒退避等待，不丢弃执行端。
重新登录原账号后自动恢复注册和 WebSocket，其他账号无法接管既有发布；关闭服务会取消等待。

## 执行端所有权与续聊

新版版本化执行日志保存每条请求的接收实例身份。进程重连保留当前身份；重启后先以
旧身份建立 ready=false 的恢复连接，只回传旧结果并等待 result.ack，完成后再以新身份
接收新任务。中断执行报告 execution_interrupted，不重复执行。若 Relay 临时记录已丢失
或过期，现有协议无法证明结果已接收，恢复保持未就绪，不擅自丢弃未 ACK 工作。

日志同时记录本地会话的确认末尾响应，执行前先标记上下文不确定。
只支持从确认末尾继续；非末尾分支、丢失或被回收的检查点、不确定上下文均明确返回
relay_branch_unsupported，须新建任务/会话，不能静默使用最新历史。会话末尾缓存最多 1024 条。

旧版已 ACK 记录可保守迁移，但旧会话缺少确认检查点，需新建会话。
旧版未 ACK 记录缺少原实例身份，启动时明确报错并保留原文件；应先确认旧请求结果再处置，
不能删除状态后直接重发相同工作。

直连诊断 CLI 仅发送 Bearer Token，由 Relay 的 token 配置提供身份，不发送部分转发身份头。
通过社区代理的正式发布链路继续携带持久化 X-CSG-Client-ID，由社区注入用户身份。
