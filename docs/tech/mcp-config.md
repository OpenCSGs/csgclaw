# MCP Configuration

Template MCP configuration lives in `mcps/mcp.json`. The recommended top-level `mcpServers` object maps server names to server definitions:

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

For compatibility with older templates, CSGClaw also accepts a direct top-level server map without the `mcpServers` wrapper. New templates should use the standard structure above.

## Common fields

Each server must provide a non-empty `command` or `url`.

| Field | Type | Description |
| --- | --- | --- |
| `command` | string | Command that starts a stdio MCP server inside the target Runtime. |
| `args` | string[] | Arguments passed to `command`. |
| `env` | object | String environment variables for a stdio server. Do not publish secrets here. |
| `url` | string | Remote MCP endpoint. |
| `transport` | string | Transport such as `stdio`, `sse`, or `streamable-http`. |
| `headers` | object | String HTTP headers for a remote server. Do not publish tokens here. |
| `startup_timeout_sec` | integer | MCP initialization timeout in seconds. |
| `tool_timeout_sec` | integer | Per-tool invocation timeout in seconds. |
| `description` | string | User-facing server description. |
| `display_name` | string | Editable display label, independent of the fixed MCP map key. |
| `enabled` | boolean | Whether the server is enabled. |
| `_meta` | object | CSGClaw-managed source and runtime declarations. |

A regular remote MCP uses its configured `url` and `headers` directly. CSGClaw does not inject an OpenCSG token into an arbitrary URL merely because an `auth_type` string is present. MCP servers that require the current OpenCSG identity must use a managed type described below.

## OpenCSG Gateway MCP

Use the following shape for a server accessed through the OpenCSG MCP Gateway:

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

Managed Gateway rules:

- `type` must be `opencsg_mcp_gateway`.
- `auth_type` must be `csghub_access_token`. Both values must match before CSGClaw enables the managed Gateway path.
- `url` must be non-empty for compatibility with the common MCP schema, but runtime materialization does not trust or connect directly to the template value; it replaces it with the local trusted CSGClaw proxy.
- `transport` should be `streamable-http`, which is also enforced at runtime.
- A template `Authorization` header is not used as the OpenCSG user credential. The proxy loads the current user's token at runtime and forwards it only to the trusted AIGateway configured for the current environment.
- `transport` is materialized as `streamable-http` at runtime.
- Omit `file_bindings` when no local file transfer is required; tools are then called directly through the standard MCP Gateway path.

## `file_bindings`

For an MCP that follows the standard OpenCSG file argument convention, list only the tool names:

```json
"file_bindings": ["parse_file_content"]
```

CSGClaw maps these tools to `content_base64`, `filename`, and `content_type`, with a 10 MiB limit. Use the following advanced object form only when an upstream tool does not follow that convention.

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

| Field | Required | Description |
| --- | --- | --- |
| `encoding` | Yes | Must currently be `base64`. |
| `content_argument` | Yes | Upstream argument that receives Base64 content. |
| `filename_argument` | Yes | Upstream argument that receives the filename. |
| `content_type_argument` | No | Upstream argument for the MIME type. No MIME argument is sent when omitted. |
| `max_bytes` | No | Maximum file size; the effective value cannot exceed CSGClaw's global 10 MiB limit. |

The model still sees a standard MCP tool, with its input transformed to:

```json
{
  "path": "relative/path/to/file.pdf",
  "content_type": "application/pdf"
}
```

`path` must be relative to the Runtime workspace. CSGClaw validates the path and regular-file type, reads the file, and performs Base64 encoding outside the model context. The File Bridge uses the authenticated `tools/list` returned by the trusted Gateway: unbound tools retain their original definitions and are forwarded normally, while bound tools accept files only when the Gateway actually advertises them. File transfers never follow HTTP redirects.

## Knowledge-base MCP

`llm_wiki` is a separate managed type generated by the knowledge-base installation flow:

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

CSGClaw uses `resource_id` and `content_id` to verify the knowledge-base identity, resolve the current endpoint from OpenCSG, and inject the current user's credential. Do not substitute `opencsg_mcp_gateway` for `llm_wiki`; the two types use different resolution and security policies.

## Template security rules

- Never publish tokens, API keys, cookies, or user credentials in `mcps/mcp.json`.
- Community template publishing sanitizes credentials from common `headers` and `env`, but template source files should still contain no secrets.
- For `opencsg_mcp_gateway`, OpenCSG user tokens are injected only by the runtime proxy and do not enter templates, persisted Agent configuration, or model context.
- Regular MCP URLs never receive an OpenCSG token. Only recognized managed types enter their corresponding trusted resolution path.
- The current Gateway trust boundary is the authenticated tool catalog visible to the current user. Do not register unreviewed MCP services in that catalog. Strong isolation between resources behind one Gateway will require the Gateway protocol to expose a verifiable resource/server identity.

## Stable identity and display names

Managed MCP maps use immutable runtime IDs as keys and store the editable label in `display_name`.
Chinese characters, spaces, and punctuation are supported in display names.
Legal existing keys are retained; a new name that cannot be a runtime ID, or whose key is already occupied, receives a generated `mcp_` ID.
Rename `display_name` rather than the map key to preserve runtime identity.
Catalog PUT and DELETE paths, Agent batch `names`, and file-bridge paths identify servers by their fixed map keys.
The catalog PUT request's `name` field is the desired display name.

```json
{
  "mcpServers": {
    "mcp_8f53d812e79c4b76ab537915a83250bc": {
      "display_name": "必应搜索中文",
      "url": "https://mcp.example.com/mcp",
      "transport": "streamable-http"
    }
  }
}
```

Display names and marketplace source metadata are removed before runtime materialization.
Changing only a display name does not require a runtime restart.
Agent configurations remain independent snapshots of the catalog; applying the same catalog ID again updates its snapshot without adding a second server.
Marketplace installations retain their source ID so reinstalling after a local or upstream rename preserves the local ID and display name.

Startup migrates the root catalog and all Agent MCP snapshots together, before loading services.
The migration preserves legal keys, assigns shared replacement IDs for the same invalid old key, and copies old names into `display_name`.
It preserves per-Agent connection settings, credentials, unknown fields, and the distinction between absent, null, and empty MCP maps.
Agents whose runtime keys change are marked as requiring a runtime refresh.
The migration writes and syncs a temporary file with owner-only permissions, then atomically replaces `state.json` without keeping backup files.
The temporary file is cleaned up when the operation returns.
Repeating startup preserves IDs and leaves unchanged data untouched.
