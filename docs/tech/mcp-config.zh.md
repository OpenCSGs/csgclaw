# MCP 配置规范

模板中的 MCP 配置位于 `mcps/mcp.json`。推荐使用顶层 `mcpServers`，其值是 MCP server 名称到配置对象的映射：

```json
{
  "mcpServers": {
    "example": {
      "url": "https://mcp.example.com/mcp",
      "transport": "streamable-http",
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 60
    }
  }
}
```

为了兼容早期模板，CSGClaw 也接受省略 `mcpServers`、直接以 server 名称作为顶层键的形式；新模板应使用上述标准结构。

## 通用字段

每个 server 必须提供非空的 `command` 或 `url`，并支持以下通用字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `command` | string | 在目标 Runtime 中启动 stdio MCP server 的命令。 |
| `args` | string[] | `command` 的参数。 |
| `env` | object | stdio MCP server 的字符串环境变量。不要在社区模板中写入密钥。 |
| `url` | string | 远程 MCP endpoint。 |
| `transport` | string | 传输类型，例如 `stdio`、`sse` 或 `streamable-http`。 |
| `headers` | object | 远程 MCP 的字符串 HTTP headers。不要在社区模板中写入 Token。 |
| `startup_timeout_sec` | integer | MCP 初始化超时秒数。 |
| `tool_timeout_sec` | integer | 单次工具调用超时秒数。 |
| `description` | string | 展示给用户的说明。 |
| `enabled` | boolean | 是否启用该 server。 |
| `_meta` | object | CSGClaw 管理的来源和运行时声明。 |

普通远程 MCP 会直接使用配置中的 `url` 和 `headers`，CSGClaw 不会因为配置了 `auth_type` 字符串就向任意 URL 注入 OpenCSG Token。需要 OpenCSG 用户身份的 MCP 必须使用下面定义的受管类型。

## OpenCSG Gateway MCP

通过 OpenCSG MCP Gateway 访问的 server 使用以下完整配置：

```json
{
  "mcpServers": {
    "file-parser": {
      "description": "Parse documents through the trusted OpenCSG MCP Gateway.",
      "url": "https://aigateway.example.com/gateway/mcp",
      "transport": "streamable-http",
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 180,
      "_meta": {
        "com.opencsg/mcp": {
          "type": "opencsg_mcp_gateway",
          "auth_type": "csghub_access_token",
          "file_bindings": ["parse_file_content"]
        }
      }
    }
  }
}
```

受管 Gateway 配置规则：

- `type` 必须是 `opencsg_mcp_gateway`。
- `auth_type` 必须是 `csghub_access_token`。只有这两个值同时匹配时，CSGClaw 才会启用受管 Gateway 链路。
- `url` 必须是非空字符串以兼容通用 MCP schema，但运行时不会信任或直接连接模板中的地址，而是替换为 CSGClaw 本地可信代理。
- `transport` 建议填写 `streamable-http`，运行时也会统一设置为该值。
- 模板中的 `Authorization` 不会作为 OpenCSG 用户凭证使用。代理在运行时读取当前登录用户的 OpenCSG Token，并且只转发到当前环境配置的可信 AIGateway。
- `transport` 在运行时统一设置为 `streamable-http`。
- 不需要处理本地文件时可以省略 `file_bindings`，此时模型通过标准 MCP Gateway 直接调用工具。

## `file_bindings`

遵循 OpenCSG 标准文件参数约定的 MCP 只需要列出工具名：

```json
"file_bindings": ["parse_file_content"]
```

CSGClaw 默认映射到 `content_base64`、`filename` 和 `content_type`，文件上限为 10 MiB。只有上游工具不遵循该约定时，才需要使用以下高级对象形式：

```json
"file_bindings": {
  "custom_parser": {
    "encoding": "base64",
    "content_argument": "data",
    "filename_argument": "name",
    "content_type_argument": "mime_type",
    "max_bytes": 5242880
  }
}
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `encoding` | 是 | 当前必须为 `base64`。 |
| `content_argument` | 是 | 上游工具接收 Base64 内容的参数名。 |
| `filename_argument` | 是 | 上游工具接收文件名的参数名。 |
| `content_type_argument` | 否 | 上游工具接收 MIME 类型的参数名；省略时不发送 MIME 参数。 |
| `max_bytes` | 否 | 允许读取的最大文件字节数；实际值不会超过 CSGClaw 的 10 MiB 全局上限。 |

配置有效后，模型看到的仍是标准 MCP 工具，但输入参数被转换为：

```json
{
  "path": "relative/path/to/file.pdf",
  "content_type": "application/pdf"
}
```

其中 `path` 必须是 Runtime 工作区内的相对路径。CSGClaw 会验证路径和普通文件类型、读取文件，并在模型上下文之外完成 Base64 编码。File Bridge 以当前用户从可信 Gateway 实际获得的 `tools/list` 为准：未绑定工具保持原始定义并正常转发，绑定工具只有在 Gateway 确实提供时才接受文件。文件调用不跟随 HTTP 重定向。

## 知识库 MCP

`llm_wiki` 是另一种受管类型，由知识库安装流程生成：

```json
{
  "_meta": {
    "com.opencsg/mcp": {
      "type": "llm_wiki",
      "auth_type": "csghub_access_token",
      "resource_id": "42",
      "content_id": "knowledge-content-id"
    }
  }
}
```

CSGClaw 使用 `resource_id` 和 `content_id` 校验知识库身份、从 OpenCSG 获取当前 endpoint，并注入当前用户凭证。不要把 `llm_wiki` 改成 `opencsg_mcp_gateway`；两种类型使用不同的解析和安全策略。

## 模板安全规则

- 不要在 `mcps/mcp.json` 中发布 Token、API Key、Cookie 或其他用户凭证。
- 社区模板发布流程会清理通用 `headers` 和 `env` 中的凭证，但模板作者仍应保持源文件无密钥。
- 对于 `opencsg_mcp_gateway`，OpenCSG 用户 Token 只在运行时代理中注入，不进入模板、Agent 持久化配置或模型上下文。
- 普通 MCP URL 不会获得 OpenCSG Token；只有已识别的受管类型会进入对应的可信解析链路。
- 当前 Gateway 的安全边界是“当前用户经过认证后可见的工具目录”。不要把未经审核的 MCP 注册到该目录；若未来需要同一 Gateway 内的强资源隔离，Gateway 协议还需要提供可验证的 resource/server 标识。
