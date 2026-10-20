# Model connections

The model settings page groups official services by brand, then separates API, Coding Plan, Agent Plan, and Token Plan connections.
Choose a service and region, follow **Get Key** to its official console, paste the Key, and select **Connect**.
The service address and API protocol are filled automatically.
The advanced section supports a custom connection name, endpoint, protocol, request headers, and model IDs.

Each connection owns its credentials, service, region, and model directory.
You can configure several accounts for the same service.
API and subscription credentials are separate even when their endpoints are identical.
A failed connection never switches to a different region or billing channel.

## Supported services

The authoritative directory is `internal/modelpreset/catalog.go` and is available at `GET /api/v1/model-provider-presets`.
It includes the official documentation and Key console links for each service and region.
The Web UI reads this directory rather than maintaining another set of endpoints.

| Brand | Services | Regions |
| --- | --- | --- |
| OpenAI | API | Global |
| Claude | API | Global |
| Kimi | API, Coding | China, Global |
| Alibaba Cloud | API, Token Plan | China, Singapore |
| Volcengine Ark | API, Coding Plan, Agent Plan | Beijing |
| GLM | API, Coding Plan | China, Z.ai |
| Xiaomi MiMo | API; Token Plan | Global API; China, Singapore, Europe Plan |
| MiniMax | API, M Plan | China, Global |
| DeepSeek | API | Global |

Claude API uses a Console API Key and the native Messages protocol.
It is a separate connection from the existing Claude Code account login.
Subscription access and model permissions remain subject to the provider's terms and account entitlements.
CSGClaw identifies itself truthfully and does not impersonate another client to bypass access restrictions.

## Connection verification and storage

`POST /api/v1/model-providers` accepts `preset`, `region`, `api_protocol`, and `connect: true` in addition to the existing connection fields.
With `connect: true`, the server discovers models, verifies a minimal conversation inference when conversation models are available, and saves only after success.
Image-only and video-only services are verified through their authenticated model directory; checking a connection does not create a paid image or video generation task.
An omitted display name is generated from the service name, with a numeric suffix when necessary.
Explicit duplicate display names are rejected.

`api_protocol` is `responses`, `chat_completions`, or `anthropic_messages`.
The provider config loader, external model storage, onboarding, Agent profiles, and session profile resolution preserve it.
Agents continue to use the local CSGClaw LLM bridge, which translates requests and streamed tool events to the configured upstream protocol.

Model discovery uses the provider's authenticated model endpoint.
If discovery is unsupported or empty, a documented preset or manually supplied model list can be used only after a successful inference.
The response identifies `model_source` as `upstream`, `preset`, or `manual`.
A documented model list describes candidates, not a guarantee of access to every model.
Authentication, quota, permission, and network failures are shown separately and cannot be hidden by a preset model list.

Credentials are persisted by the backend and are omitted from connection-directory responses.
Provider errors shown during connection do not include raw upstream bodies or echoed secrets.
Official logos ship locally, with provenance recorded in `web/app/public/model-providers/SOURCES.md`.

## Agent creation defaults

The model settings page provides conversation, image-generation, and video-generation defaults for new Agents.
Only connected services contribute selectable models, and image and video choices use the corresponding advertised capabilities.
Without an explicit choice, conversation defaults prefer OpenCSG's `auto` model; image and video defaults prefer the first advertised OpenCSG model for that capability.
If OpenCSG is unavailable, the first available model is used; categories without available models display **Unspecified**.
Explicit choices, including **Unspecified**, persist across refreshes, server restarts, and later Agent creations.
An explicit model that becomes unavailable falls back to the recommendation for that category without overwriting the saved reference.

`GET /api/v1/agent-profile-defaults` returns effective defaults and `PATCH` updates one category at a time, rejecting disconnected providers and mismatched capabilities.
Explicit selections are stored in `agents.creation_defaults` in the local state file, without provider credentials.
Unchanged categories continue to use recommendations, and changing one category does not overwrite another category’s saved preference.
The existing remembered profile remains separate from these explicit selections.
Template creation, custom creation, and Agent settings share the filtered model selectors.
When a default is **Unspecified**, creation requires a manual conversation-model choice and leaves optional image and video models empty.
Previous creation choices are not restored, including when loading defaults fails.

## Changing models

Agent settings define the default model for new sessions.
Changing the model in a conversation affects that conversation only, retains its history, and applies to the next message.
**Follow Agent default** removes the conversation override.
Changing models is disabled while a reply is running, and a failed save retains the previous selection.
The same searchable model selector is used by Agent settings and conversation controls, with service and connection names to distinguish identical model IDs.

## Verification

Contract tests cover service presets, authentication, discovery, unavailable discovery, failed inference, persistence, and all three wire protocols.
Protocol tests include tool definitions, tool IDs, tool-result continuation, streaming completion, and cancellation.
Headless UI verification covers onboarding, connection errors, regions, keyboard navigation, conversation persistence, themes, narrow screens, and deployment under a URL prefix.
Real-provider availability requires the corresponding account's authorized test Key; contract fixtures do not establish a live vendor connection.
