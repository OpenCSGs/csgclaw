<p align="center">
  <img src="assets/logo.png" alt="CSGClaw logo" width="600" />
</p>

<p align="center">
  English | <a href="./README.zh.md">中文</a>
</p>

# CSGClaw

> Your Personal AI Team

CSGClaw is a multi-agent collaboration platform built by OpenCSG — designed around one practical question: **once work becomes non-trivial, how do you get a group of AI agents to operate like a team, without the system becoming heavy or painful to set up?**

## Install

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/OpenCSGs/csgclaw/main/scripts/install.sh | bash
```

The installer downloads a prebuilt release binary and places it on your `PATH`. Prebuilt binaries are available for macOS arm64 and Linux amd64.

**Build from source:**

```bash
export CGO_ENABLED=1
go mod download
(cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 go run ./cmd/setup)
go build ./cmd/csgclaw
```

## Quick Start

```bash
csgclaw onboard
csgclaw serve
```

Open the printed URL (e.g. `http://127.0.0.1:18080/`) in your browser to enter the IM workspace.
For a fresh interactive setup, `onboard` defaults to `csghub-lite` at `http://127.0.0.1:11435/v1`, lets you override that URL, and imports models from its OpenAI-compatible `/v1/models` endpoint. Start CSGHub-lite first, for example:

```bash
csghub-lite run Qwen/Qwen3-0.6B-GGUF
```

For scripts or non-interactive environments, pass model flags explicitly:

```bash
csgclaw onboard --provider csghub-lite --models Qwen/Qwen3-0.6B-GGUF
csgclaw onboard --base-url <url> --api-key <key> --models <model[,model...]> [--reasoning-effort <effort>]
```

## Configuration

`csgclaw onboard` writes a local config with server, model, bootstrap, sandbox, and channel settings. See [docs/config.md](docs/config.md) for model provider examples, sandbox provider options, and Worker override examples.

## Features

- **Multi-agent coordination** — work with a team of specialized agents through a single coordination point, not a pile of chat windows
- **One-click install** — prebuilt binaries for macOS arm64 and Linux amd64; up and running in minutes
- **WebUI out of the box** — browser-based workspace available immediately after `csgclaw serve`
- **Multi-channel support** — connect Feishu, WeChat, Matrix, or other channels when needed
- **Isolated execution** — each Worker runs in a secure sandbox with security boundaries enabled by default
- **Role-based Workers** — specialize Workers for frontend, backend, testing, docs, research, and more

## What CSGClaw Is

CSGClaw gives you one **Manager** and a set of specialized **Workers**, so instead of juggling isolated agents, you work through a single coordination point for defining goals, breaking down work, assigning roles, tracking progress, and collecting results.

```text
┌────────────────────────────────────────────────────────────┐
│                         CSGClaw                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Manager — understands goals, plans, coordinates      │  │
│  └──────────────────────────────────────────────────────┘  │
│               ↓                      ↓                     │
│        Worker Alice            Worker Bob                  │
│          frontend                 backend                  │
│                                                            │
│   WebUI / Feishu / WeChat / Matrix / other channels        │
└────────────────────────────────────────────────────────────┘
                      ↑ you make decisions
```

**Manager** — receives your goals, decomposes tasks, selects Workers, tracks progress, and consolidates results.

**Workers** — role-specific executors (frontend, backend, testing, docs, research…). Specialization keeps context clean and reduces role confusion.

**Sandbox** — Worker execution is isolated by the configured sandbox provider. The default is **BoxLite**, with support for `boxlite-cli` and custom providers.

**Interface** — WebUI out of the box; Feishu, WeChat, Matrix, and other channels available as integrations.

## A Typical Workflow

```text
You: Build a web app prototype — landing page, login, and basic admin view.

Manager: Splitting into tasks.
  · Alice → landing page & login UI
  · Bob   → backend APIs & data model
  · Carol → integration checks

You: Add GitHub login to the login flow.

Manager: Updating Alice and Bob.

Carol: Login response is missing the user avatar field.

Manager: Bob updates the API first; Alice updates the UI once the field contract is confirmed.
```

The key isn't that multiple agents exist — it's that **their collaboration is organized**.

## Design Principles

**PicoClaw by default, extensible by design.**
CSGClaw uses PicoClaw as its lightweight default Agent Runtime, keeping the Manager fast to start and cheap to run. The runtime remains pluggable, so deployments can integrate alternatives such as OpenClaw when needed.

**BoxLite by default, sandbox provider as an extension point.**
Isolation is non-negotiable, but Docker is often overkill for local-first use. BoxLite gives Workers meaningful security boundaries without a container runtime. Teams that need a different isolation model can add a custom sandbox provider.

**WebUI first, channel-agnostic by design.**
Many multi-agent systems are tightly coupled to one messaging protocol. CSGClaw ships with a built-in WebUI so you can start immediately, while keeping other channels (Feishu, WeChat, Matrix) as optional integrations — not assumptions.

## Who It Is For

- Independent developers who want an AI team, not just a single assistant
- Small teams that want lower-friction multi-agent collaboration
- Users who value fast startup, lighter runtime, and sensible defaults

## Acknowledgement

CSGClaw is informed by ideas explored in HiClaw around multi-agent usability, while placing stronger emphasis on lightweight runtime, easier local startup, and a platform model not bound to a single communication channel.

## License

CSGClaw is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.
