# 云端智能体只读档案与聊天资源

平台创建的 CSGClaw 实例复用本地资源选择器，支持技能、连接器和知识库。档案来自当前运行实例，包含介绍、运行时、模型、自定义指令和资源摘要，全部只读；不提供修改、安装、启停或凭据配置操作。外部发布、没有平台 sandbox endpoint 的智能体保留原来的展示。

## 查询与执行路径

- 档案和资源查询：浏览器 → 本地 CSGClaw 的 `/api/v1/relay/agents/{id}/profile` 或 `/resources` → 平台实例记录中的 endpoint → sandbox CSGClaw 的 GET API。
- 聊天执行：浏览器 → 本地消息持久化 → CSGRelay `/v1/responses` → 云端 CSGClaw executor → sandbox 内部 Session API → Agent Runtime。

endpoint 仅由服务端从当前登录身份可访问的平台实例记录解析。它必须匹配当前账号的 AI Gateway 和 sandbox 路径，不接受浏览器传入 URL，不跟随重定向。档案响应不包含环境变量、模型凭据或 MCP 配置。资源选择器没有本地资源管理链接。

## 资源协议

Relay 请求和 executor `request` 帧增加可选 `resource_refs`：

```json
{
  "resource_refs": [
    {"source": "skill", "id": "weather-forecast"},
    {"source": "mcp", "id": "weather-service"}
  ]
}
```

资源来源支持 `skill`、`mcp` 和 `app`，单条消息最多 16 个引用。仅选择一个技能时允许正文为空；连接器、知识库或多个资源仍需要正文或附件。

执行端通过 hello/heartbeat 的 `resource_refs` capability 声明支持。Relay 将引用纳入请求幂等校验、持久化记录和断线恢复帧；同一个幂等键更改资源引用会被拒绝。旧执行端接收纯文本的行为不变，资源请求会返回 `executor_resources_unsupported`，不会悄悄忽略选择。

本地发送前从云端资源列表重建展示快照，持久化在消息的资源元数据中。云端 Session API 在执行前再次检查资源是否仍挂载、启用且可用，并使用现有资源指令格式构造本轮输入。资源选择不自动继承到下一轮。

## 部署

资源聊天需要同步更新 CSGRelay、本地 CSGClaw 客户端以及云端 CSGClaw 镜像。已有实例应重建或重启到新镜像以更新 executor 的能力声明。仅更新客户端界面无法让旧执行端支持资源引用。平台还必须为当前用户提供可访问的实例 endpoint；无访问权限时显示错误和重试入口，不展示为成功的空列表。
