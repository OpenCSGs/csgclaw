# Agentfile v1 规范草案

本文定义 `Agentfile.toml` v1 规范草案。它的目标是把 agent template 标准化为一个清晰、可验证、可分发、可演进的协议。

一句话定义：

```text
Agent template = runnable OCI image + runtime declaration + image env contract + optional workspace overlay.
```

`Agentfile.toml` v1 描述的是**可直接创建并运行 agent 的标准模板**。它不描述 image 如何构建，不保存真实 secret，也不负责 marketplace 排名、签名、审计、版本历史或回滚策略。

---

## 1. 概述

### 1.1 设计目标

标准 Agent Template 应满足以下目标：

- 有明确的模板身份信息，例如稳定 ID、模板版本、名称、描述和更新时间。
- 有明确的创建角色，例如 `manager` 或 `worker`。
- 有明确的 runtime 类型，例如 `picoclaw_sandbox` 或 `openclaw_sandbox`。
- 可以声明 runtime 版本或 runtime contract 版本，用于兼容性提示和 workspace layout 预期。
- 有明确的 OCI image 引用，agent platform 可以基于该 image 创建运行环境。
- 可以声明 image 运行所需或支持的环境变量契约。
- 可以携带可选 workspace overlay，例如 `AGENT.md`、`AGENTS.md`、skills、memory、项目文件或初始化资料。
- 可以通过 `.gitignore` 或其他 ignore file 控制 workspace overlay 的源文件过滤。
- 可以被 CLI、Web UI、template registry、CI 或导入流程进行静态校验。
- 不包含真实 secret。
- 不承担 image 构建系统职责。

### 1.2 非目标

`Agentfile.toml` v1 不解决以下问题：

- 不描述如何构建 image，例如 Dockerfile、Containerfile、Buildpacks、Nix 或 Bazel。
- 不自动执行本地 build。
- 不发布或保存真实环境变量 secret。
- 不标准化依赖用户本机已安装工具、认证状态或本地文件路径的纯本地 runtime。
- 不描述 marketplace 排名、签名、审计、扫描、版本历史或回滚策略。
- 不保证第三方 image 可信。
- 不定义 agent 内部协议、LLM 调用协议或工具调用协议。

如果后续需要本地 image 构建能力，应使用独立文件或命令，例如 `image-build.toml` 或某个具体实现提供的 build command，而不是把构建系统放入 `Agentfile.toml` 主协议。

---

## 2. 标准 Agent Template

标准 agent template 必须包含有效的 `Agentfile.toml`，并且必须声明可用的 `[image].ref`。

它可以被 agent platform 直接用于创建 agent。

```text
standard agent template = valid Agentfile.toml + runnable image.ref + optional workspace overlay
```

标准 Agentfile v1 的核心边界是：

```text
描述如何运行一个标准 agent template，
不描述如何构建 image，
不保存真实 secret，
不把 workspace-only/local preset/source template 混入标准 runnable template。
```

缺少可用 `[image].ref` 的目录不是标准 Agentfile v1 template。source template、build-required template、workspace template 和 local preset 等非标准形态见附录 A。

---

## 3. 文件布局与格式

### 3.1 文件布局

标准 template 目录使用以下布局：

```text
<template-root>/
  Agentfile.toml
  workspace/          # optional
    .gitignore        # optional, default workspace ignore file
```

`workspace/` 是可选目录。存在时，agent platform 可以在基于 template 创建 agent 时，把该目录作为 workspace overlay 复制到 agent 的运行时 workspace。

`workspace` 表示 agent 的工作区间。不同 runtime 可以有不同的 workspace 内部结构。Agentfile v1 不要求不同 runtime kind 共享完全一致的 workspace 文件布局。

例如：

```text
picoclaw-worker/
  Agentfile.toml
  workspace/
    .gitignore
    AGENT.md

openclaw-worker/
  Agentfile.toml
  workspace/
    .gitignore
    AGENTS.md
    SOUL.md
```

### 3.2 文件格式

`Agentfile.toml` 使用 TOML 格式。

选择 TOML 的原因：

- TOML 适合手写配置，比 JSON 更易读。
- TOML 的隐式类型和缩进风险比 YAML 更少。
- `Agentfile.toml` 带后缀，编辑器、GitHub 和 Web UI 更容易识别格式。

---

## 4. 最小合法 Agentfile

```toml
schema_version = "agentfile/v1"

id = "opencsg.openclaw-worker"
version = "0.1.0"
name = "OpenClaw Worker"
role = "worker"

[runtime]
kind = "openclaw_sandbox"

[image]
ref = "registry.example.com/team/openclaw-worker:1.0.0"
```

这是标准 Agentfile v1 的最小合法格式。

其中：

- `schema_version` 标识 Agentfile 协议版本。
- `id` 是稳定模板 ID。
- `version` 是模板自身版本。
- `name` 是展示名称。
- `role` 是创建角色。
- `[runtime].kind` 声明 runtime 类型。
- `[image].ref` 声明可运行 OCI image。

完整推荐发布示例见附录 F。

---

## 5. 字段规范

### 5.1 顶层字段

#### 字段列表

| 字段             | 类型   | 必填 | 说明                                         |
| ---------------- | ------ | ---- | -------------------------------------------- |
| `schema_version` | string | 是   | Agentfile 协议版本，当前为 `agentfile/v1`    |
| `id`             | string | 是   | 稳定模板 ID                                  |
| `version`        | string | 是   | 模板自身版本                                 |
| `name`           | string | 是   | 模板显示名称，也可作为缺省 agent name 的来源 |
| `description`    | string | 否   | 模板说明文字                                 |
| `role`           | string | 是   | 创建角色，例如 `manager` 或 `worker`         |
| `updated_at`     | string | 否   | RFC3339 更新时间，用于展示和排序             |

#### `schema_version`

当前版本固定为：

```toml
schema_version = "agentfile/v1"
```

实现必须拒绝未知 major version。

对于同一 major version 内新增的可选字段，旧实现可以忽略，或在严格校验场景中给出 warning 或 error。

#### `id`

`id` 用于标识模板的稳定 ID。

示例：

```toml
id = "opencsg.openclaw-worker"
```

约束：

- 必填。
- trim 后不能为空。
- 应在发布源或 template registry namespace 内保持稳定。
- 不应随显示名称改变而改变。
- 不应包含真实 secret。
- 建议只使用小写字母、数字、点号、短横线和下划线。

设计理由：

- `name` 更适合作为展示名称，不适合作为稳定身份。
- 同步、升级、去重、收藏、权限控制、来源追踪和引用都需要稳定 ID。
- `updated_at` 不能替代模板 ID。

#### `version`

`version` 用于描述模板自身版本，不等同于 `schema_version`。

示例：

```toml
version = "0.1.0"
```

约束：

- 必填。
- trim 后不能为空。
- 推荐使用 SemVer。
- 同一 `id` 下的不同模板修订应更新 `version`。

设计理由：

- `schema_version` 描述 Agentfile 协议版本。
- `version` 描述模板内容版本。
- `updated_at` 只适合排序和展示，不适合作为唯一升级依据。

#### `name`

`name` 是模板的显示名称，也可作为缺省 agent name 的来源。

示例：

```toml
name = "OpenClaw Worker"
```

约束：

- 必填。
- trim 后不能为空。
- 不应包含真实 secret。
- 不应作为稳定模板 ID 使用。

#### `description`

`description` 是模板的说明文字。

示例：

```toml
description = "OpenClaw worker template"
```

#### `role`

`role` 描述该 template 适用的创建角色。

允许值：

```text
manager
worker
```

`role` 用于判断 template 适用于 manager 还是 worker 创建流程。默认 manager template 应声明 `manager`，默认 worker template 应声明 `worker`。

注意：

```text
role describes the creation role this template is intended for,
not the full semantic role or capability model of the agent.
```

也就是说，`role` 不是通用 agent taxonomy。后续如果引入 notification bot、A2A bot、reviewer、tool agent 等概念，不应直接复用该字段表达所有语义角色。

#### `updated_at`

`updated_at` 是 RFC3339 时间戳，用于排序、展示和同步提示。

示例：

```toml
updated_at = "2026-05-18T00:00:00Z"
```

约束：

- 如果存在，必须是 RFC3339 时间戳。
- 推荐使用 UTC 时间并以 `Z` 结尾。

注意：

```text
updated_at is presentation metadata only.
Implementations must not use updated_at as the only identity or upgrade key.
```

模板身份和升级判断应优先基于 `id`、`version`、源信息和 registry metadata，而不是只依赖 `updated_at`。

---

### 5.2 `[runtime]`

`[runtime]` 是必填段，用于声明 agent platform 应使用哪类 runtime 创建 agent。

#### 字段列表

| 字段      | 类型   | 必填 | 说明                                     |
| --------- | ------ | ---- | ---------------------------------------- |
| `kind`    | string | 是   | runtime 类型，例如 `openclaw_sandbox`    |
| `version` | string | 否   | runtime 实现版本或 runtime contract 版本 |

#### `runtime.kind`

示例：

```toml
[runtime]
kind = "openclaw_sandbox"
```

v1 标准 Agentfile 面向可复现的 sandbox/container template。推荐允许值：

```text
picoclaw_sandbox
openclaw_sandbox
```

实现可以支持更多 runtime kind，但标准公开分发的 template 应优先使用可复现的 sandbox/container runtime。

不建议把依赖用户本机工具链的纯本地 runtime 放入标准 agent template。此类配置更适合作为 local preset 或 workspace template。

#### `runtime.version`

`runtime.version` 是可选字段，用于描述该模板期望的 runtime 实现版本或 runtime contract 版本。

示例：

```toml
[runtime]
kind = "openclaw_sandbox"
version = "0.3.0"
```

它不是 Agentfile 协议版本，不是模板自身版本，也不能替代 `image.ref` 或 `image.digest`。

版本语义区分如下：

```text
schema_version   = Agentfile 协议版本
version          = template 自身版本
runtime.version  = runtime 实现版本或 runtime contract 版本
image.ref        = 实际可运行 image 引用
image.digest     = 实际可运行 image digest
```

实现可以使用 `runtime.version`：

- 选择兼容的 runtime adapter。
- 校验 workspace layout 预期。
- 给出 runtime 兼容性提示。
- 在 registry 或 UI 中展示 runtime contract 信息。

`runtime.version` 是兼容性元数据。真正可运行的 artifact 仍然由 `[image]` 定义。

---

### 5.3 `[image]`

`[image]` 是必填段，用于声明 template 的可运行 OCI image。

#### 字段列表

| 字段        | 类型            | 必填 | 说明                 |
| ----------- | --------------- | ---- | -------------------- |
| `ref`       | string          | 是   | OCI image 引用       |
| `digest`    | string          | 否   | image digest         |
| `platforms` | array of string | 否   | image 支持的平台列表 |

#### `image.ref`

示例：

```toml
[image]
ref = "registry.example.com/team/agent:1.0.0"
```

约束：

- 必须是 runtime adapter 能消费的 image 引用。
- trim 后不能为空。
- 不应包含 secret，例如带明文 token 的 registry URL。
- 在标准 Agentfile v1 中总是必填。

设计理由：

- 标准 agent template 的核心目标是可复现创建运行环境。
- 如果没有 image，template 只能描述 workspace 或本地工具约定，无法保证跨机器一致创建 agent。
- 构建 image 的方式属于发布或开发流程，不属于运行协议。

#### `image.digest`

示例：

```toml
digest = "sha256:..."
```

建议：

- 正式分发的模板应优先使用 digest-pinned image。
- 本地或私有模板可以只使用 tag。
- 如果同时提供 `ref` 和 `digest`，实现可以在拉取或校验时确认最终 image digest 是否匹配。

`tag` 可能被覆盖，不能完全保证可复现；`digest` 更适合作为正式分发、审计和回滚依据。

#### `image.platforms`

示例：

```toml
platforms = ["linux/amd64", "linux/arm64"]
```

用途：

- Web UI 可以提前提示当前机器是否兼容。
- CLI 可以在创建前给出更明确的错误。
- Registry 可以展示平台支持情况。

---

### 5.4 `[[image.env]]`

`[[image.env]]` 是可选数组，用于声明 image 的环境变量契约。

它描述 image 期望或支持哪些环境变量，但不保存用户实际值。创建 agent 时，agent platform 可以根据该契约生成 UI 表单、CLI 提示、校验规则，并把用户提供的值写入 agent profile、secret storage 或 runtime create spec。

#### 字段列表

| 字段          | 类型            | 必填 | 默认值  | 说明               |
| ------------- | --------------- | ---- | ------- | ------------------ |
| `name`        | string          | 是   | 无      | 环境变量名         |
| `required`    | bool            | 否   | `false` | 是否必填           |
| `secret`      | bool            | 否   | `false` | 是否按 secret 处理 |
| `default`     | string          | 否   | 无      | 非 secret 默认值   |
| `description` | string          | 否   | 无      | 说明文字           |
| `choices`     | array of string | 否   | 无      | 允许值列表         |
| `pattern`     | string          | 否   | 无      | 格式校验正则       |
| `example`     | string          | 否   | 无      | 示例值             |
| `placeholder` | string          | 否   | 无      | UI 占位提示        |

#### 示例

```toml
[[image.env]]
name = "OPENCLAW_API_KEY"
required = true
secret = true
description = "API key used by the runtime image"

[[image.env]]
name = "OPENCLAW_LOG_LEVEL"
required = false
secret = false
default = "info"
choices = ["debug", "info", "warn", "error"]
description = "Runtime log level"
```

#### 核心规则

- `name` trim 后不能为空。
- 同一 Agentfile 内的 env `name` 不得重复。
- `name` 建议只使用大写字母、数字和下划线。
- `name` 不得和实现保留环境变量冲突。
- `required` 默认值为 `false`。
- `secret` 默认值为 `false`。
- `secret = true` 时不得设置 `default`。
- `default` 只表示缺省运行值，不表示该变量必填。
- `choices` 如果存在，不得为空数组。
- 如果设置了 `default`，且同时设置了 `choices`，`default` 必须出现在 `choices` 中。
- `pattern` 如果存在，必须是实现支持的合法正则表达式。
- 如果 `choices` 足够表达约束，优先使用 `choices`。
- `secret = true` 时，`example` 不应是真实 secret。

#### Required 与 default

对于 `required = true` 的 env：

- 如果 `secret = true`，不能通过 `default` 满足 required 约束。
- 如果 `secret = false` 且设置了 `default`，实现可以认为默认值满足 required 约束。
- 推荐避免同时设置 `required = true` 和非 secret `default`，除非确实希望展示“有默认值但仍是关键配置”。

#### 空值语义

实现应明确区分以下状态：

```text
missing: 用户没有提供该变量
empty string: 用户显式提供空字符串
non-empty string: 用户提供非空字符串
```

对于 `required = true` 的 env，默认规则是：

```text
missing 或 empty string 均视为未满足 required
```

如果未来需要允许空字符串，应增加显式字段，而不是让实现自行猜测。

---

### 5.5 `[workspace]`

`[workspace]` 是可选段，用于声明 template 是否携带 workspace overlay。

在 v1 中，`workspace` 表示 template 提供的一组 agent 初始工作区文件。对于 Claw-family runtime，它通常会映射到 agent 的工作目录。其他 runtime 可以由 runtime adapter 映射到等价的默认工作目录、项目目录或 agent home。

`[workspace]` 保持为顶层段，因为它描述的是 template 提供的初始文件 overlay，而不是 runtime 内部配置的一部分。

workspace 的内部结构是 runtime-specific 的。Agentfile v1 不要求不同 runtime kind 共享完全一致的 workspace 文件布局。例如，PicoClaw template 可以提供 `AGENT.md`，而 OpenClaw template 可以提供 `AGENTS.md` 和 `SOUL.md`。

实现应根据 `runtime.kind`、`runtime.version` 和对应 runtime adapter 解释 `workspace.path`，并把它映射到该 runtime 的默认工作目录。

#### 字段列表

| 字段           | 类型   | 必填 | 默认值             | 说明                                             |
| -------------- | ------ | ---- | ------------------ | ------------------------------------------------ |
| `path`         | string | 否   | `workspace`        | workspace overlay 源目录                         |
| `optional`     | bool   | 否   | `true`             | 源目录缺失时是否允许继续                         |
| `merge_policy` | string | 否   | `fail_on_conflict` | overlay 冲突处理策略                             |
| `ignore_file`  | string | 否   | `.gitignore`       | overlay ignore 文件，路径相对于 `workspace.path` |

#### 示例

```toml
[workspace]
path = "workspace"
optional = true
merge_policy = "fail_on_conflict"
ignore_file = ".gitignore"
```

#### `workspace.path`

`workspace.path` 是 template root 下的相对路径。

约束：

- 不得是绝对路径。
- 不得为空。
- 不得包含 `..` 跳出 template root。
- 不得通过 symlink 指向 template root 外部。
- 不得指向普通文件。

#### `workspace.optional`

当为 `true` 时，`workspace.path` 不存在也可以加载 template。

当为 `false` 时，路径不存在或不是目录应视为 template 无效。

#### `workspace.merge_policy`

允许值：

```text
fail_on_conflict
overwrite
skip_existing
```

语义：

- `fail_on_conflict`：如果目标 workspace 中已存在同名文件或目录冲突，则创建失败。
- `overwrite`：template overlay 覆盖目标 workspace 中的同名文件。
- `skip_existing`：保留目标 workspace 中已有文件，跳过 template 中冲突文件。

v1 推荐默认使用 `fail_on_conflict`，因为它最安全、最可解释。

#### `workspace.ignore_file`

`workspace.ignore_file` 是可选字段，用于指定 `workspace.path` 下的 ignore 文件。

默认值为：

```toml
ignore_file = ".gitignore"
```

如果该文件存在，实现应使用 Git ignore pattern semantics 来决定哪些源文件不参与 workspace overlay。

规则：

- `ignore_file` 路径相对于 `workspace.path`。
- `ignore_file` 不得是绝对路径。
- `ignore_file` 不得包含 `..` 跳出 `workspace.path`。
- 如果 `ignore_file` 指向的文件不存在，则不应用 ignore 规则。
- ignore 文件本身属于 overlay 控制元数据，默认不复制到目标 workspace。
- 如果实现不能完整支持 Git ignore 语义，必须明确记录其限制。

示例：

```gitignore
# local/editor noise
.DS_Store
.idea/
.vscode/

# temporary files
tmp/
*.log

# local secrets
.env
*.key
```

说明：

- `.gitignore` 是默认值，因为它符合大多数用户对项目文件过滤的既有心智。
- 如果 Git ignore 规则与 overlay 规则不一致，模板作者可以显式指定其他文件，例如 `.workspaceignore` 或 `.agentignore`。
- Agentfile v1 只要求实现处理 `workspace.ignore_file` 指向的文件；是否额外支持嵌套 `.gitignore`、全局 Git excludes 或 Git index 状态，由具体实现决定并应在文档中说明。

#### Runtime 映射

`workspace` 只描述 template 源文件集合，不强制规定 runtime 内的目标目录。

推荐映射：

- Claw-family runtime：复制到 agent workspace。
- 其他 sandbox runtime：复制到 runtime adapter 定义的默认工作目录或 agent home。
- 不支持 workspace overlay 的 runtime：可以拒绝该 template，或在创建前给出明确错误。

更详细的 workspace 安全处理建议见附录 D。

---

### 5.6 Vendor extension

Agentfile v1 主协议不包含特定实现的版本约束、Hub policy 或产品配置。

如果某个实现需要随模板携带私有扩展字段，推荐使用 `x.<vendor>` namespace。

示例：

```toml
[x.csgclaw]
min_version = "0.3.2"
```

规则：

- 标准实现可以忽略未知的 `x.*` 段。
- 具体实现可以读取自己的 `x.<vendor>` 段。
- `x.*` 不得改变标准字段的基础语义。
- 标准字段与 vendor extension 冲突时，应以标准字段为准。

---

## 6. 校验规则

实现加载 `Agentfile.toml` 时必须执行以下校验。

### 6.1 基础校验

- `schema_version` 必须存在且为支持的版本。
- `id` 必须存在且 trim 后非空。
- `version` 必须存在且 trim 后非空。
- `name` 必须存在且 trim 后非空。
- `role` 必须是 `manager` 或 `worker`。
- `[runtime]` 必须存在。
- `runtime.kind` 必须存在且为实现支持的 runtime kind。
- `runtime.version` 如果存在，trim 后必须非空。
- `[image]` 必须存在。
- `image.ref` 必须存在且 trim 后非空。
- `updated_at` 存在时必须是 RFC3339 时间戳。

### 6.2 Image 校验

- `image.ref` 不得包含明文 secret。
- `image.digest` 如果存在，应符合 digest 格式。
- `image.platforms` 如果存在，不得为空数组。
- `image.platforms` 中的值应符合 `os/arch` 或 `os/arch/variant` 格式。

### 6.3 Env 校验

- `[[image.env]]` 中每个 `name` 必须非空。
- `[[image.env]]` 中的 `name` 在同一 Agentfile 内必须唯一。
- `[[image.env]]` 中的 `name` 不得和实现保留环境变量冲突。
- `secret = true` 的 env 不得设置 `default`。
- `choices` 如果存在，不得为空数组。
- `default` 如果存在且 `choices` 存在，`default` 必须出现在 `choices` 中。
- `pattern` 如果存在，必须是实现支持的合法正则表达式。

### 6.4 Workspace 校验

- `[workspace]` 存在时，`workspace.path` 必须是安全的 template-relative path。
- `workspace.optional = false` 时，`workspace.path` 必须存在且为目录。
- `workspace.merge_policy` 必须是允许值之一。
- `workspace.ignore_file` 如果存在，必须是安全的 workspace-relative path。
- `workspace.ignore_file` 如果存在，不得是绝对路径，不得包含 `..`，不得跳出 `workspace.path`。
- workspace overlay 不得通过路径穿越或 symlink 写出 template root 或目标 workspace。

### 6.5 Registry 或发布流程额外校验

Template registry、官方模板发布流程或 CI 可以比基础协议更严格，建议根据需要增加以下要求：

- 对未知字段给出 warning 或 error。
- 要求正式模板使用 digest-pinned image。
- 要求声明 `image.platforms`。
- 执行保留 env 列表校验。
- 执行 workspace overlay 大小限制。
- 检查 `workspace.ignore_file` 是否存在以及语义是否可解析。
- 执行 registry policy，例如 image allowlist、签名校验、漏洞扫描或人工审核。

---

## 7. 创建 agent 时的解析行为

当用户从 template 创建 agent 时，agent platform 应按以下方式解析：

1. 读取 `Agentfile.toml`。
2. 校验 manifest。
3. 读取 `name`、`description`、`role`、`runtime.kind`、`runtime.version` 和 `image.ref` 作为创建请求的默认值或兼容性元数据。
4. 如果创建请求显式传入字段，则显式字段优先。
5. 根据 `[[image.env]]` 生成需要用户确认或填写的环境变量。
6. 合并来自用户输入、agent profile、workspace profile、secret storage 或其他配置来源的 env 值。
7. 校验 required env、choices、pattern 和 secret 规则。
8. 把最终 env 值写入 agent profile、secret storage 或 runtime create spec，不写回 `Agentfile.toml`。
9. 如果 workspace 存在，则读取 `workspace.ignore_file` 并计算需要应用的 workspace overlay 文件集合。
10. 按照 `[workspace]` 配置把过滤后的 workspace overlay 应用到 agent runtime workspace。
11. 创建 runtime environment 时只消费最终解析出的 `image.ref` 和 runtime create spec。

字段优先级建议：

```text
explicit create request
> user-provided values from UI/CLI
> selected profile or saved config
> Agentfile non-secret defaults
> implementation defaults
```

secret 处理建议：

```text
Agentfile never stores secret values.
Resolved secret values may be stored only in approved secret storage or agent profile mechanism.
Logs and UI must redact secret values.
```

---

## 8. 向后兼容与版本演进

### 8.1 兼容策略

标准 Agentfile v1 只定义 `Agentfile.toml`。

具体实现可以为了迁移目的读取旧格式或私有格式，但这些兼容读取行为不改变标准 Agentfile v1 的必填字段和语义。

例如，某个实现可以把旧的 `agent.toml` 映射为 Agentfile v1 内部结构，但被映射后的结果必须满足标准 Agentfile v1 校验规则，才能被视为标准 Agentfile v1 template。

### 8.2 版本演进

版本演进建议：

- `agentfile/v1` 内只能新增可选字段。
- 删除字段、改变必填性或改变字段语义时，应进入 `agentfile/v2`。
- 新字段必须有合理默认行为。
- 旧实现可以忽略未知可选字段，或者在严格校验场景中拒绝未知字段。
- Template registry 和发布流程可以比本地读取流程更严格。

---

## 9. 字段汇总

### 9.1 顶层字段

| 字段             | 类型   | 必填 | 说明                                      |
| ---------------- | ------ | ---- | ----------------------------------------- |
| `schema_version` | string | 是   | Agentfile 协议版本，当前为 `agentfile/v1` |
| `id`             | string | 是   | 稳定模板 ID                               |
| `version`        | string | 是   | 模板自身版本                              |
| `name`           | string | 是   | 模板显示名称                              |
| `description`    | string | 否   | 模板说明                                  |
| `role`           | string | 是   | 创建角色：`manager` 或 `worker`           |
| `updated_at`     | string | 否   | RFC3339 更新时间，用于展示和排序          |

### 9.2 `[runtime]`

| 字段      | 类型   | 必填 | 说明                                     |
| --------- | ------ | ---- | ---------------------------------------- |
| `kind`    | string | 是   | runtime 类型，例如 `openclaw_sandbox`    |
| `version` | string | 否   | runtime 实现版本或 runtime contract 版本 |

### 9.3 `[image]`

| 字段        | 类型            | 必填 | 说明           |
| ----------- | --------------- | ---- | -------------- |
| `ref`       | string          | 是   | OCI image 引用 |
| `digest`    | string          | 否   | image digest   |
| `platforms` | array of string | 否   | 支持的平台列表 |

### 9.4 `[[image.env]]`

| 字段          | 类型            | 必填 | 默认值  | 说明               |
| ------------- | --------------- | ---- | ------- | ------------------ |
| `name`        | string          | 是   | 无      | 环境变量名         |
| `required`    | bool            | 否   | `false` | 是否必填           |
| `secret`      | bool            | 否   | `false` | 是否按 secret 处理 |
| `default`     | string          | 否   | 无      | 非 secret 默认值   |
| `description` | string          | 否   | 无      | 说明文字           |
| `choices`     | array of string | 否   | 无      | 允许值列表         |
| `pattern`     | string          | 否   | 无      | 格式校验正则       |
| `example`     | string          | 否   | 无      | 示例值             |
| `placeholder` | string          | 否   | 无      | UI 占位提示        |

### 9.5 `[workspace]`

| 字段           | 类型   | 必填 | 默认值             | 说明                                             |
| -------------- | ------ | ---- | ------------------ | ------------------------------------------------ |
| `path`         | string | 否   | `workspace`        | workspace overlay 源目录                         |
| `optional`     | bool   | 否   | `true`             | 源目录缺失时是否允许继续                         |
| `merge_policy` | string | 否   | `fail_on_conflict` | overlay 冲突处理策略                             |
| `ignore_file`  | string | 否   | `.gitignore`       | overlay ignore 文件，路径相对于 `workspace.path` |

### 9.6 `[x.<vendor>]`

| 字段     | 类型                   | 必填 | 说明                                                  |
| -------- | ---------------------- | ---- | ----------------------------------------------------- |
| 任意字段 | implementation-defined | 否   | vendor-specific extension，不属于 Agentfile v1 主协议 |

---

# 附录 A：非标准模板形态

v1 明确区分标准 agent template 与以下非标准形态。

## A.1 Source template / build-required template

source template 可以分发 Dockerfile、Containerfile、README、workspace 和其他构建上下文，但它**不是标准 Agentfile v1 模板的一种变体**。

如果一个 template 需要用户先本地构建 image，它在构建完成并补全 `[image].ref` 之前，不应被实现当作标准 agent template 接受。

推荐边界：

```text
source template / build-required template:
  - 可以作为 template registry 的参与路径或展示类型
  - 可以包含 Dockerfile、Containerfile、README、workspace 等文件
  - 不能在缺少可用 image.ref 时直接创建 agent
  - 不改变 Agentfile.toml v1 的必填规则
```

## A.2 Workspace template

如果一个模板只有 workspace，没有 image，它不应称为标准 agent template。

它可以另行定义为 workspace template：

```text
workspace template = workspace files only, no promise of runnable agent environment
```

workspace template 可以用于初始化文件、项目骨架或 prompt/material 分发，但它不承诺 runtime 可启动，也不能替代标准 Agentfile v1 template。

## A.3 Local preset

如果一个模板依赖用户本机工具、认证状态、本地 CLI 或本地文件路径，它更适合作为 local preset，而不是标准 agent template。

```text
local preset = local convenience configuration, not a portable runtime template
```

local preset 可以提升本地使用便利性，但不应被当作可跨机器、可复现、可直接分发的标准 runtime template。

## A.4 Registry 展示类型

Template registry 展示时应明确区分：

```text
standard agent template: 已有 image.ref，可以直接创建 agent
source template / build-required template: 需要用户先构建 image，不能直接创建 agent
workspace template: 只分发 workspace，不承诺 runtime 可启动
local preset: 面向本机便利，允许依赖用户本机工具或认证状态
```

---

# 附录 B：本地 image 构建

## B.1 为什么 Agentfile v1 不包含 `[build]`

`Agentfile.toml` v1 不包含 `[build]`。

原因：

- build 依赖本机是否安装 Docker、Podman、Buildah、Nix、Bazel 或其他工具。
- build 依赖平台架构、buildx、缓存、网络、私有依赖和 registry 认证。
- 远程 template 如果自动触发 build，会引入安全确认、日志审计和失败恢复问题。
- template 运行协议应描述可运行 artifact，而不是 artifact 生产过程。

## B.2 推荐构建流程

如果需要保留本地构建能力，建议独立为发布流程：

```text
开发者使用任意工具构建 OCI image
开发者把 image 推送到 registry 或打成本地 tag
Agentfile.toml 的 [image].ref 指向该 image
agent platform 创建 agent 时只运行该 image
```

## B.3 Source template 参与流程

有些用户希望共享 agent template，但无法提供公开可信的 image registry，或者使用方不愿直接信任第三方发布的 image。v1 可以允许 source template 作为 registry 的参与路径，但应保持协议边界清晰。

source template 可以包含：

```text
<template-root>/
  Dockerfile or Containerfile
  image-build.toml        # optional, future helper file
  workspace/              # optional
  README.md               # optional
```

如果包含 `Agentfile.toml`，则该文件仍必须满足标准 v1 规则。也就是说，标准 `Agentfile.toml` 仍然需要 `[image].ref`。

如果还没有可用 image.ref，应使用其他文件描述 source template 元数据，或者在 registry 中以 build-required 类型展示，而不是把不完整的 Agentfile 当作标准 v1 接受。

推荐流程：

```text
作者发布 source template
使用方下载并审查 Dockerfile、workspace 和 README
使用方通过 Docker、Podman、Buildah、Nix 或某个实现提供的 build command 构建本地 image
构建结果得到本地 tag 或可信 registry ref
生成或更新 Agentfile.toml，让 [image].ref 指向该 image
agent platform 按标准 Agentfile v1 创建 agent
```

## B.4 未来扩展

未来可以增加独立辅助文件，例如：

```text
image-build.toml
```

但该文件不属于 Agentfile v1 的运行协议。

---

# 附录 C：Legacy 映射建议

标准 Agentfile v1 不定义 legacy manifest。

具体实现可以为了迁移目的读取旧格式或私有格式。例如，某个实现历史上可能使用 `agent.toml`：

```toml
name = "openclaw-worker"
description = "OpenClaw worker template"
role = "worker"
runtime_kind = "openclaw_sandbox"
image = "registry.example.com/team/agent:1.0.0"
updated_at = "2026-05-18T00:00:00Z"
```

该实现可以把它映射为 Agentfile v1 内部结构：

```toml
schema_version = "agentfile/v1"

id = "example.openclaw-worker"
version = "0.1.0"
name = "openclaw-worker"
description = "OpenClaw worker template"
role = "worker"
updated_at = "2026-05-18T00:00:00Z"

[runtime]
kind = "openclaw_sandbox"
version = "0.3.0"

[image]
ref = "registry.example.com/team/agent:1.0.0"

[workspace]
path = "workspace"
optional = true
merge_policy = "fail_on_conflict"
ignore_file = ".gitignore"
```

注意：

- legacy 映射是实现自己的迁移行为，不属于 Agentfile v1 主协议。
- 映射后的结构必须满足标准 Agentfile v1 校验规则，才能被视为标准 Agentfile v1 template。
- 如果 legacy manifest 缺少 `id` 或 `version`，实现可以在迁移时要求用户补全，或根据明确规则生成临时值，但不应把缺失 `id` / `version` 的 manifest 当作标准 Agentfile v1。

---

# 附录 D：安全模型

`Agentfile.toml` 是模板元数据，不是可信背书。

## D.1 Image 安全

- 第三方 image 默认应视为不可信。
- 是否允许拉取和运行第三方 image，应由用户确认、管理员策略或 registry policy 决定。
- Registry 可以在 Agentfile v1 之外增加 allowlist、digest pinning、签名校验、漏洞扫描和审计策略。
- `image.ref` 不得包含明文 token、用户名密码或其他 secret。

## D.2 Env 安全

- Agentfile 只声明 env contract，不保存真实 env value。
- `secret = true` 的 env 不得设置 `default`。
- UI、CLI、日志和导出流程必须避免明文显示 secret。
- 实现应维护保留环境变量列表，避免 template 覆盖系统注入变量。

保留环境变量通常包括 agent platform 在启动 runtime 时自动注入的 callback、token、agent id、LLM bridge 等运行时变量。实现应维护一个保留列表，并在校验时拒绝或忽略冲突项。

## D.3 Workspace 安全

workspace overlay 是不可信输入。

实现应用 workspace overlay 时，建议遵循以下默认行为：

- 复制普通文件和目录。
- 保留相对路径结构。
- 保留可执行权限位。
- 复制 dotfiles，除非被 `workspace.ignore_file` 排除。
- 允许空目录，但实现可以选择忽略空目录并给出说明。
- 不允许任何文件写出目标 workspace 之外。
- 不跟随指向 template root 外部的 symlink。
- 对 symlink 的处理应明确：要么拒绝 symlink，要么只允许安全的相对 symlink。
- 应限制单文件大小和总 overlay 大小，具体限制由实现或 registry policy 决定。

建议默认策略：

```text
symlink_policy = reject_unsafe_symlink
```

如果实现暂时无法安全处理 symlink，应直接拒绝 workspace overlay 中的 symlink。

## D.4 Ignore 文件安全

`workspace.ignore_file` 用于过滤 workspace overlay 源文件，但它不能替代安全校验。

实现仍然必须在 ignore 过滤之后执行路径安全、symlink 安全和大小限制校验。

建议：

- 默认读取 `workspace/.gitignore`。
- ignore 文件本身默认不复制到目标 workspace。
- ignore 文件不得使路径穿越、外部 symlink 或超大文件绕过安全策略。
- 即使某个文件没有被 ignore，也不代表它一定安全。
- Registry 或 CI 可以额外检查常见 secret 文件，例如 `.env`、`*.key`、credential files 等。

## D.5 Build 安全

- Agentfile v1 不自动执行 build。
- source template 的构建步骤必须由用户或显式命令触发。
- 自动 build、构建日志审计、registry 登录和私有依赖访问不属于 Agentfile v1 主协议。

---

# 附录 E：推荐实现清单

实现 Agentfile v1 时，建议至少完成：

- `Agentfile.toml` 解析。
- schema version 校验。
- `id`、`version`、`name`、`role`、`runtime.kind`、`image.ref` 必填校验。
- `runtime.version` 读取和展示。
- `updated_at` RFC3339 校验。
- env name 去重。
- secret env 禁止 default。
- choices/default 一致性校验。
- 保留 env 冲突检测。
- workspace path 安全校验。
- workspace ignore file 读取。
- Git ignore pattern semantics 支持或限制说明。
- workspace symlink 安全处理。
- workspace merge policy。
- 创建 agent 时的字段优先级合并。
- secret 脱敏输出。
- vendor extension 的忽略或读取机制。

---

# 附录 F：完整推荐发布示例

```toml
schema_version = "agentfile/v1"

id = "opencsg.openclaw-worker"
version = "0.1.0"
name = "OpenClaw Worker"
description = "OpenClaw worker template"
role = "worker"
updated_at = "2026-05-18T00:00:00Z"

[runtime]
kind = "openclaw_sandbox"
version = "0.3.0"

[image]
ref = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260518.1"
platforms = ["linux/amd64", "linux/arm64"]

[[image.env]]
name = "OPENCLAW_LOG_LEVEL"
required = false
secret = false
default = "info"
choices = ["debug", "info", "warn", "error"]
description = "Log level used by OpenClaw runtime"

[workspace]
path = "workspace"
optional = true
merge_policy = "fail_on_conflict"
ignore_file = ".gitignore"

[x.csgclaw]
min_version = "0.3.2"
```

其中 `[x.csgclaw]` 是 vendor extension 示例，不属于 Agentfile v1 主协议。

对应 workspace 示例：

```text
workspace/
  .gitignore
  AGENTS.md
  SOUL.md
  TOOLS.md
  HEARTBEAT.md
  skills/
  memory/
```

`.gitignore` 示例：

```gitignore
.DS_Store
.env
*.key
*.log
tmp/
```

---

## 总结

`Agentfile.toml` v1 应保持主协议收敛：

```text
标准模板必须可直接创建 agent；
标准模板必须声明 runnable image.ref；
标准模板必须声明稳定 id 和 version；
runtime.version 描述 runtime 实现版本或 runtime contract 版本；
workspace.ignore_file 默认使用 .gitignore 过滤 overlay 源文件；
构建、源码分发、workspace-only、本地 preset 都是协议外形态；
具体平台版本约束、Hub policy、legacy 迁移和安全策略可以通过扩展或附录补充。
```

这能让 Agentfile v1 同时具备：

- 清晰的运行入口；
- 可验证的 manifest；
- 可复现的 sandbox/container 创建路径；
- 可被 Web UI 和 CLI 消费的 env contract；
- 可选但有安全边界的 workspace overlay；
- 更明确的 runtime contract 兼容性表达；
- 更符合用户直觉的 `.gitignore` overlay 过滤机制；
- 面向未来 template registry、CI、版本演进和安全策略的扩展空间。
